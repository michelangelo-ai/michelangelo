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
// The controller now uses RunnerFactory for provider-aware runner selection instead
// of hardcoded Runner implementations.
type Params struct {
	fx.In

	Logger            logr.Logger
	WorkflowClient    clientInterface.WorkflowClient
	APIHandlerFactory apiHandler.Factory
}

// Reconciler reconciles TriggerRun resources through a state machine.
//
// The reconciler manages the complete lifecycle of trigger runs, from initial workflow
// start through terminal states (SUCCEEDED, FAILED, or KILLED). It uses RunnerFactory
// for provider-aware runner selection.
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
	WorkflowClient    clientInterface.WorkflowClient

	// Removed RunnerFactory - using WorkflowClient unified scheduling directly
}

// NewReconciler creates a new TriggerRun Reconciler with the provided dependencies.
//
// The reconciler uses WorkflowClient's unified scheduling methods for provider-aware
// trigger management. The API handler is configured during registration through the Register method.
func NewReconciler(p Params) *Reconciler {
	return &Reconciler{
		apiHandlerFactory: p.APIHandlerFactory,
		WorkflowClient:    p.WorkflowClient,
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
			log.Info("trigger_run resource has been deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	return r.reconcile(ctx, log, triggerRun)
}

// reconcile processes a TriggerRun through its state machine.
//
// State Machine Logic:
//
//   - Terminal states (SUCCEEDED/FAILED/KILLED): Mark resource immutable and stop reconciliation
//   - INVALID: Start scheduled workflow using WorkflowClient unified scheduling
//   - RUNNING: Check schedule status, handle kill requests if Spec.Kill is true
//
// The method performs the following operations:
//  1. Check if resource is in terminal state and mark immutable if needed
//  2. Create deep copy of resource to detect changes
//  3. Execute state transitions using WorkflowClient's unified scheduling methods
//  4. Persist status updates if resource changed
//  5. Requeue after 60 seconds for continued monitoring
//
// Kill requests are processed by setting Spec.Kill=true, which causes the reconciler
// to invoke WorkflowClient.StopScheduledWorkflow during the next reconciliation.
func (r *Reconciler) reconcile(
	ctx context.Context, log logr.Logger, triggerRun *v2pb.TriggerRun,
) (ctrl.Result, error) {
	if isTerminateState(triggerRun) {
		if !apiutils.IsImmutable(triggerRun) {
			apiutils.MarkImmutable(triggerRun)
			err := r.Update(ctx, triggerRun, &metav1.UpdateOptions{})
			if err != nil {
				log.Error(err, "Fail to update trigger run status")
				return ctrl.Result{}, err
			}
			log.Info("trigger_run resource marked as immutable")
		}
		log.Info(fmt.Sprintf("reached terminal state: %s", triggerRun.Status.State.String()))
		// do not requeue
		return ctrl.Result{}, nil
	}
	originalTriggerRun := triggerRun.DeepCopy()

	// Generate schedule ID for this trigger
	scheduleID := r.generateScheduleID(triggerRun)
StateMachine:
	switch triggerRun.Status.State {
	case v2pb.TRIGGER_RUN_STATE_INVALID:
		log.Info("TRIGGER_RUN_STATE_INVALID")
		if triggerRun.Spec.Kill {
			triggerRun.Status = v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED}
			break StateMachine
		}

		// Create scheduled workflow using unified scheduling
		execution, err := r.WorkflowClient.StartScheduledWorkflow(ctx, clientInterface.ScheduledWorkflowOptions{
			TriggerRun:   triggerRun,
			WorkflowType: "trigger.CronTrigger",
			TaskQueue:    "trigger_run",
			Args:         []interface{}{CreateTriggerRequest{TriggerRun: triggerRun}},
		})

		if err != nil {
			log.Error(err, "failed to start scheduled workflow",
				"operation", "start_workflow",
				"namespace", triggerRun.Namespace,
				"name", triggerRun.Name,
				"triggerType", fmt.Sprintf("%T", triggerRun.Spec.Trigger.TriggerType),
				"provider", r.WorkflowClient.GetProvider(),
				"supportsSchedules", r.WorkflowClient.SupportsSchedules())
			triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_FAILED
			triggerRun.Status.ErrorMessage = err.Error()
			break StateMachine
		}

		log.Info("scheduled workflow started",
			"operation", "workflow_started",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"scheduleId", scheduleID,
			"executionId", execution.ID)
		triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_RUNNING
		triggerRun.Status.ExecutionWorkflowId = execution.ID
		triggerRun.Status.LogUrl = r.getWorkflowURL(execution.ID)
	case v2pb.TRIGGER_RUN_STATE_RUNNING:
		log.Info("TRIGGER_RUN_STATE_RUNNING")
		// disable the trigger
		if triggerRun.Spec.Kill {
			err := r.WorkflowClient.StopScheduledWorkflow(ctx, scheduleID)
			if err != nil {
				log.Error(err, "failed to stop scheduled workflow",
					"operation", "stop_workflow",
					"namespace", triggerRun.Namespace,
					"name", triggerRun.Name,
					"scheduleId", scheduleID)
				triggerRun.Status.ErrorMessage = err.Error()
				triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_FAILED
				break StateMachine
			}
			log.Info("trigger run killed",
				"operation", "workflow_killed",
				"namespace", triggerRun.Namespace,
				"name", triggerRun.Name,
				"scheduleId", scheduleID)
			triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_KILLED
			break StateMachine
		}

		// Check schedule status
		scheduleStatus, err := r.WorkflowClient.GetScheduleStatus(ctx, scheduleID)
		if err != nil {
			log.Error(err, "failed to get schedule status",
				"operation", "get_status",
				"namespace", triggerRun.Namespace,
				"name", triggerRun.Name,
				"scheduleId", scheduleID)
			triggerRun.Status.ErrorMessage = err.Error()
			triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_FAILED
			break StateMachine
		}

		// Map schedule status to trigger run state
		switch scheduleStatus.State {
		case "RUNNING":
			triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_RUNNING
		case "PAUSED":
			triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_RUNNING // Still considered running, just paused
		case "KILLED":
			triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_KILLED
		case "FAILED":
			triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_FAILED
			triggerRun.Status.ErrorMessage = scheduleStatus.ErrorMessage
		}
	}

	if !reflect.DeepEqual(originalTriggerRun, triggerRun) {
		err := r.UpdateStatus(ctx, triggerRun, &metav1.UpdateOptions{})
		if err != nil {
			log.Error(err, "Fail to update trigger run status")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

// Register registers the TriggerRun controller with the controller manager.
//
// This method configures the controller with:
//   - API handler for Kubernetes operations
//   - Structured logger with "triggerRun" prefix
//   - TriggerRun resource watch
//   - Maximum concurrent reconciles setting
//
// Returns an error if API handler creation or controller registration fails.
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

// generateScheduleID creates a deterministic schedule ID from TriggerRun metadata.
func (r *Reconciler) generateScheduleID(triggerRun *v2pb.TriggerRun) string {
	return fmt.Sprintf("%s-%s-schedule", triggerRun.Namespace, triggerRun.Name)
}

// getWorkflowURL constructs a URL for viewing the workflow/schedule in the provider's UI.
func (r *Reconciler) getWorkflowURL(workflowID string) string {
	// This would need to be configured based on the deployment
	// For now, return a placeholder that includes the workflow ID
	return fmt.Sprintf("%s://workflow/%s", r.WorkflowClient.GetProvider(), workflowID)
}
