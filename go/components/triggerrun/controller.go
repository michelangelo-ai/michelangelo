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
	workflow "github.com/michelangelo-ai/michelangelo/go/components/triggerrun/workflow"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/fx"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

const (
	// this is the concurrency reconcile loops for trigger run, it can be tuned if needed.
	_maximumConcurrentReconciles = 10
	_noCronScheduleErrorMessage  = "no cron schedule found, please check the trigger in pipeline spec to make sure it has cron schedule"
	_triggerNotFoundErrorMessage = "source trigger name or trigger spec not found in current pipeline revision, please manually create a new trigger run"
)

// Params are the params for instantiating the reconciler.
type Params struct {
	fx.In

	Logger            logr.Logger
	WorkflowClient    clientInterface.WorkflowClient
	APIHandlerFactory apiHandler.Factory

	CronTrigger       Runner `name:"cron-trigger"`
	IntervalTrigger   Runner `name:"interval-trigger"`
	BackfillTrigger   Runner `name:"backfill-trigger"`
	BatchRerunTrigger Runner `name:"batch-rerun-trigger"`
}

// Reconciler reconciles a TriggerRun object
type Reconciler struct {
	api.Handler
	Log    logr.Logger
	Scheme *runtime.Scheme

	apiHandlerFactory apiHandler.Factory

	CronTrigger       Runner
	IntervalTrigger   Runner
	BackfillTrigger   Runner
	BatchRerunTrigger Runner
}

// NewReconciler returns a new TriggerRun Reconciler.
func NewReconciler(p Params) *Reconciler {
	return &Reconciler{
		apiHandlerFactory: p.APIHandlerFactory,
		CronTrigger:       p.CronTrigger,
		IntervalTrigger:   p.IntervalTrigger,
		BackfillTrigger:   p.BackfillTrigger,
		BatchRerunTrigger: p.BatchRerunTrigger,
	}
}

// Reconcile reconciles the TriggerRun object
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

	runner := r.getRunner(triggerRun)
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
			log.Error(err, "failed to start scheduled cadence workflow")
			triggerRun.Status.State = v2pb.TRIGGER_RUN_STATE_FAILED
			triggerRun.Status.ErrorMessage = status.ErrorMessage
			break StateMachine
		}
		log.Info("cadence scheduled workflow started", zap.Any("status", status))
		triggerRun.Status.State = status.State
		triggerRun.Status.LogUrl = status.LogUrl
		triggerRun.Status.ExecutionWorkflowId = status.ExecutionWorkflowId
	case v2pb.TRIGGER_RUN_STATE_RUNNING:
		log.Info("TRIGGER_RUN_STATE_RUNNING")
		// disable the trigger
		if triggerRun.Spec.Kill {
			status, err := runner.Kill(ctx, triggerRun)
			if err != nil {
				log.Error(err, "failed to kill scheduled cadence workflow")
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
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

// Register is used to register the controller with the manager.
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
		WithOptions(controller.Options{MaxConcurrentReconciles: _maximumConcurrentReconciles}).
		Complete(r)
}

func (r *Reconciler) getRunner(tr *v2pb.TriggerRun) Runner {
	triggerType := workflow.GetTriggerType(tr)
	switch triggerType {
	case workflow.TriggerTypeInterval:
		return r.IntervalTrigger
	case workflow.TriggerTypeBackfill:
		return r.BackfillTrigger
	case workflow.TriggerTypeBatchRerun:
		return r.BatchRerunTrigger
	default:
		return r.CronTrigger
	}
}
