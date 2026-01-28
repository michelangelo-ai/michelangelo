// Package triggerrun implements a Kubernetes controller for managing TriggerRun resources.
//
// This package provides scheduled and event-driven workflow execution through a state machine
// that manages the lifecycle of trigger runs. It supports multiple trigger types including
// cron schedules, backfill operations, interval-based triggers, and batch reruns.
//
// Architecture:
//
// The controller uses a Runner interface abstraction to support different trigger types:
//   - CronTrigger: Recurring scheduled workflows using cron expressions
//   - BackfillTrigger: One-time workflows for historical data backfilling
//   - IntervalTrigger: Workflows triggered at fixed intervals
//   - BatchRerunTrigger: Bulk reprocessing of previously executed workflows
//
// State Machine:
//
// TriggerRun resources transition through the following states:
//   - INVALID → RUNNING: Initial workflow start
//   - RUNNING → SUCCEEDED/FAILED/KILLED: Terminal states based on execution outcome
//
// The controller reconciles resources every 60 seconds to check workflow status and handle
// kill requests. Terminal states are marked immutable to prevent further modifications.
//
// Workflow Integration:
//
// The controller integrates with Cadence or Temporal workflow engines to execute
// scheduled workflows. Each Runner implementation manages workflow lifecycle operations
// including starting, monitoring, and terminating workflow executions.
package triggerrun

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/fx"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

const (
	// maximumConcurrentReconciles defines the maximum number of concurrent reconcile loops
	// for the TriggerRun controller. This value can be tuned based on cluster capacity.
	maximumConcurrentReconciles = 10
)

// Params contains the dependencies required to instantiate the TriggerRun Reconciler.
//
// This struct uses Uber FX dependency injection to wire controller dependencies.
// The Runner implementations are tagged by name to inject the correct trigger type.
type Params struct {
	fx.In

	Logger            logr.Logger
	WorkflowClient    clientInterface.WorkflowClient
	APIHandlerFactory apiHandler.Factory

	CronTrigger       Runner // Handles cron-based recurring workflows
	IntervalTrigger   Runner // Handles interval-based workflows
	BackfillTrigger   Runner // Handles backfill workflows
	BatchRerunTrigger Runner // Handles batch rerun workflows
}

// Reconciler reconciles TriggerRun resources through a state machine.
//
// The reconciler manages the complete lifecycle of trigger runs, from initial workflow
// start through terminal states (SUCCEEDED, FAILED, or KILLED). It delegates execution
// to the appropriate Runner based on the trigger type.
//
// State transitions are handled through a labeled switch statement that allows
// breaking out of the state machine once a terminal state is reached. The reconciler
// persists status updates to Kubernetes and requeues resources every 60 seconds for
// ongoing status checks.
//
// The reconciler supports concurrent processing of multiple TriggerRun resources
// based on the maximumConcurrentReconciles setting.
type Reconciler struct {
	api.Handler
	Log    logr.Logger
	Scheme *runtime.Scheme

	apiHandlerFactory apiHandler.Factory

	CronTrigger       Runner // Executes cron-scheduled workflows
	IntervalTrigger   Runner // Executes interval-based workflows
	BackfillTrigger   Runner // Executes backfill workflows
	BatchRerunTrigger Runner // Executes batch rerun workflows
}

// NewReconciler creates a new TriggerRun Reconciler with the provided dependencies.
//
// The reconciler is initialized with Runner implementations for each supported trigger type.
// The API handler is configured during registration through the Register method.
func NewReconciler(p Params) *Reconciler {
	return &Reconciler{
		apiHandlerFactory: p.APIHandlerFactory,
		CronTrigger:       p.CronTrigger,
		IntervalTrigger:   p.IntervalTrigger,
		BackfillTrigger:   p.BackfillTrigger,
		BatchRerunTrigger: p.BatchRerunTrigger,
	}
}

// Reconcile implements the controller-runtime Reconciler interface for TriggerRun resources.
//
// This method is invoked by the controller framework whenever a TriggerRun resource is
// created, updated, or periodically requeued. It fetches the resource from Kubernetes
// and delegates to the reconcile helper method for state machine processing.
//
// If the resource has been deleted, reconciliation completes without error. Other fetch
// errors are returned to be retried by the controller framework.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("triggerRun", req.NamespacedName)
	triggerRun := &v2pb.TriggerRun{}
	if err := r.Get(ctx, req.NamespacedName.Namespace, req.NamespacedName.Name, &metav1.GetOptions{},
		triggerRun); err != nil {
		if apiutils.IsNotFoundError(err) {
			log.Info("TriggerRun not found, likely deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get TriggerRun")
		return ctrl.Result{}, err
	}

	return r.reconcile(ctx, log, triggerRun)
}

// reconcile implements the core state machine logic for managing TriggerRun lifecycle.
//
// This method performs state transitions based on the current TriggerRun status and handles
// workflow execution through the appropriate Runner implementation. The state machine logic
// is designed to be idempotent and handles concurrent access through deep copying.
//
// State Machine Logic:
//
//   - Terminal states (SUCCEEDED/FAILED/KILLED): Mark resource immutable and stop reconciliation
//   - INVALID: Start workflow execution using appropriate Runner, transition to RUNNING or FAILED
//   - RUNNING: Check workflow status, handle kill requests if Spec.Kill is true
//
// The method performs the following operations:
//  1. Check if resource is in terminal state and mark immutable if needed
//  2. Create deep copy of resource to detect changes
//  3. Execute state transitions through labeled StateMachine switch
//  4. Persist status updates if resource changed
//  5. Requeue after 60 seconds for continued monitoring
//
// Kill requests are processed by setting Spec.Kill=true, which causes the reconciler
// to invoke the Runner's Kill method during the next reconciliation.
func (r *Reconciler) reconcile(
	ctx context.Context, log logr.Logger, triggerRun *v2pb.TriggerRun,
) (ctrl.Result, error) {
	if isTerminateState(triggerRun) {
		err := r.markImmutable(ctx, triggerRun)
		if err != nil {
			log.Error(err, "Failed to mark TriggerRun as immutable")
			return ctrl.Result{}, err
		}
		log.Info("TriggerRun marked immutable")
		return ctrl.Result{}, nil
	}

	// Requeue after 60 seconds for monitoring workflow status
	requeueAfter := time.Minute
	originalTriggerRun := triggerRun.DeepCopy()

	runner := r.getRunner(triggerRun)
	if runner == nil {
		triggerType := GetTriggerType(triggerRun)
		log.Error(nil, "trigger type not implemented",
			"triggerType", triggerType,
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name)
		triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_FAILED
		triggerRun.Status.ErrorMessage = fmt.Sprintf("trigger type %s is not yet implemented", triggerType)
		if !reflect.DeepEqual(originalTriggerRun, triggerRun) {
			err := r.UpdateStatus(ctx, triggerRun, &metav1.UpdateOptions{})
			if err != nil {
				log.Error(err, "Failed to update trigger run status")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

StateMachine:
	switch triggerRun.Status.State {
	case v2pb.TRIGGER_RUN_STATE_INVALID:
		log.Info("TRIGGER_RUN_STATE_INVALID")
		if triggerRun.Spec.Kill {
			triggerRun.Status = v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED}
			break StateMachine
		}
		status, err := runner.Run(ctx, triggerRun)
		if err != nil {
			log.Error(err, "failed to start scheduled workflow",
				"operation", "start_workflow",
				"namespace", triggerRun.Namespace,
				"name", triggerRun.Name)
			triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_FAILED
			triggerRun.Status.ErrorMessage = status.ErrorMessage
			break StateMachine
		}
		log.Info("scheduled workflow started",
			"operation", "workflow_started",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"state", status.State,
			"execution_workflow_id", status.ExecutionWorkflowId)
		triggerRun.Status.State = status.State
		triggerRun.Status.LogUrl = status.LogUrl
		triggerRun.Status.ExecutionWorkflowId = status.ExecutionWorkflowId
	case v2pb.TRIGGER_RUN_STATE_RUNNING:
		log.Info("TRIGGER_RUN_STATE_RUNNING")
		// disable the trigger
		if triggerRun.Spec.Kill {
			status, err := runner.Kill(ctx, triggerRun)
			if err != nil {
				log.Error(err, "failed to kill scheduled workflow")
				triggerRun.Status.ErrorMessage = err.Error()
				triggerRun.Status.State = status.State
				break StateMachine
			}
			log.Info("trigger run killed")
			triggerRun.Status = status
			break StateMachine
		}
		status, err := runner.GetStatus(ctx, triggerRun)
		if err != nil {
			log.Error(err, "TriggerRun GetStatus failed")
			triggerRun.Status.ErrorMessage = err.Error()
			triggerRun.Status.State = status.State
			break StateMachine
		}
		triggerRun.Status.State = status.State
	}

	if !reflect.DeepEqual(originalTriggerRun, triggerRun) {
		err := r.UpdateStatus(ctx, triggerRun, &metav1.UpdateOptions{})
		if err != nil {
			log.Error(err, "Fail to update trigger run status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// markImmutable marks a TriggerRun resource as immutable when it reaches a terminal state.
//
// This prevents further modifications to the resource once workflow execution completes.
// The method updates the resource finalizers to indicate the immutable status.
func (r *Reconciler) markImmutable(ctx context.Context, triggerRun *v2pb.TriggerRun) error {
	triggerRun.ObjectMeta.Finalizers = []string{"immutable"}
	return r.Update(ctx, triggerRun, &metav1.UpdateOptions{})
}

// Register configures the TriggerRun controller with the controller manager.
//
// This method sets up the controller to watch TriggerRun resources and configures
// the maximum number of concurrent reconciliation workers. The API handler is also
// registered during this phase to enable REST API access to TriggerRun resources.
func (r *Reconciler) Register(mgr ctrl.Manager) error {
	r.Scheme = mgr.GetScheme()
	handler, err := r.apiHandlerFactory.GetAPIHandler(mgr.GetClient())
	if err != nil {
		return err
	}
	r.Handler = handler
	r.Log = mgr.GetLogger().
		WithName("triggerRun")

	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.TriggerRun{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: maximumConcurrentReconciles}).
		Complete(r)
}

// getRunner selects the appropriate Runner implementation based on the TriggerRun's trigger type.
//
// The selection is made using GetTriggerType which examines the TriggerRun spec to determine
// whether it's a batch rerun, backfill, interval, or cron trigger. The default is CronTrigger
// if the type cannot be determined.
func (r *Reconciler) getRunner(tr *v2pb.TriggerRun) Runner {
	triggerType := GetTriggerType(tr)
	switch triggerType {
	case TriggerTypeInterval:
		return r.IntervalTrigger
	case TriggerTypeBackfill:
		return r.BackfillTrigger
	case TriggerTypeBatchRerun:
		return r.BatchRerunTrigger
	default:
		return r.CronTrigger
	}
}
