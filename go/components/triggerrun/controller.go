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
//
// Cascade delete: when the owning Pipeline is deleted, the controller drains the
// run's recurring cron/schedule before GC removes it. The drain finalizer is
// installed before the ownerReference so the run is never GC-eligible unprotected,
// and a deleting run can never re-arm its cron; a 24h timeout is the backstop.
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
	"github.com/michelangelo-ai/michelangelo/go/cascadedelete"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.uber.org/fx"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// maximumConcurrentReconciles defines the maximum number of concurrent reconcile loops
	// for the TriggerRun controller. This value can be tuned based on cluster capacity.
	maximumConcurrentReconciles = 10

	// drainFinalizer blocks garbage collection of a run until its workflow has
	// been drained. Keep this string stable across releases for rollout safety —
	// do not change it.
	drainFinalizer = "triggerruns.michelangelo.uber.com/drain"

	// metricKind is this kind's cascade metric label. It is a documented dashboard
	// contract (see the cascade-delete plan §8) — do not change this value.
	metricKind = "trigger_run"

	// drainRequeueInterval paces re-checks of a TriggerRun being drained (cascade delete).
	// The kill path is synchronous, so this mainly lets the terminal-state branch remove the
	// finalizer on the next loop.
	drainRequeueInterval = 10 * time.Second
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

	CronTrigger       Runner `name:"cron-trigger"`        // Handles cron-based recurring workflows
	IntervalTrigger   Runner `name:"interval-trigger"`    // Handles interval-based workflows
	BackfillTrigger   Runner `name:"backfill-trigger"`    // Handles backfill workflows
	BatchRerunTrigger Runner `name:"batch-rerun-trigger"` // Handles batch rerun workflows
}

// Reconciler reconciles TriggerRun resources through a state machine.
//
// All fields are unexported. Exported struct fields become permanent public API surface —
// external packages can depend on them directly, making future removal a breaking change.
// All dependencies are injected via NewReconciler() or Register() instead.
// This follows the same pattern used by controllers in kubernetes/kubernetes.
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
	log    logr.Logger
	scheme *runtime.Scheme

	apiHandlerFactory apiHandler.Factory
	workflowClient    clientInterface.WorkflowClient

	cronTrigger       Runner // Executes cron-scheduled workflows
	intervalTrigger   Runner // Executes interval-based workflows
	backfillTrigger   Runner // Executes backfill workflows
	batchRerunTrigger Runner // Executes batch rerun workflows
}

// NewReconciler creates a new TriggerRun Reconciler with the provided dependencies.
//
// The reconciler is initialized with Runner implementations for each supported trigger type.
// The API handler is configured during registration through the Register method.
func NewReconciler(p Params) *Reconciler {
	return &Reconciler{
		apiHandlerFactory: p.APIHandlerFactory,
		workflowClient:    p.WorkflowClient,
		cronTrigger:       p.CronTrigger,
		intervalTrigger:   p.IntervalTrigger,
		backfillTrigger:   p.BackfillTrigger,
		batchRerunTrigger: p.BatchRerunTrigger,
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
	log := r.log.WithValues("triggerRun", req.NamespacedName)
	triggerRun := &v2pb.TriggerRun{}
	if err := r.Get(ctx, req.NamespacedName.Namespace, req.NamespacedName.Name, &metav1.GetOptions{},
		triggerRun); err != nil {
		if apiutils.IsNotFoundError(err) {
			log.Info("trigger_run resource has been deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// On cascade delete (deletionTimestamp set), hand off to the drain driver
	// before the state machine so a deleting run can't re-arm its cron.
	if !triggerRun.GetDeletionTimestamp().IsZero() {
		st := cascadedelete.DrainState{
			Object:      triggerRun,
			Kind:        metricKind,
			Finalizer:   drainFinalizer,
			IsTerminal:  isTerminateState(triggerRun),
			WorkStarted: triggerRunWorkStarted(triggerRun),
		}
		return cascadedelete.RunDrainStep(ctx, st, &triggerRunDrainTarget{r: r, log: log, run: triggerRun}, drainRequeueInterval)
	}

	// Finalizer before ownerRef: the ownerRef makes the run GC-eligible, so the
	// finalizer must be present first or GC could remove the run and orphan the cron.
	if err := r.ensureDrainFinalizer(ctx, log, triggerRun); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure the ownerReference; runs before the state machine so terminal runs are covered.
	if err := r.ensureOwnerRef(ctx, log, triggerRun); err != nil {
		return ctrl.Result{}, err
	}

	return r.reconcile(ctx, log, triggerRun)
}

// reconcile processes a TriggerRun through its state machine.
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

		// Handle actions using the new action field (preferred) or deprecated boolean fields (backward compatibility)
		actionToPerform := triggerRun.Spec.Action

		// For backward compatibility, check deprecated boolean fields if no action is set
		if actionToPerform == v2pb.TRIGGER_RUN_ACTION_NO_ACTION {
			if triggerRun.Spec.Kill {
				actionToPerform = v2pb.TRIGGER_RUN_ACTION_KILL
			}
		}

		// Sync TriggerRun spec changes to workflow engine, passing the action so cron
		// update and pause/resume can be applied atomically in a single API call.
		status, actionHandled, err := runner.Update(ctx, triggerRun, actionToPerform)
		if err != nil {
			log.Error(err, "failed to sync trigger spec to workflow engine")
			triggerRun.Status.ErrorMessage = err.Error()
			triggerRun.Status.State = status.State
			break StateMachine
		}
		triggerRun.Status = status

		// If Update already applied the action atomically, clear and skip
		if actionHandled {
			triggerRun.Spec.Action = v2pb.TRIGGER_RUN_ACTION_NO_ACTION
			triggerRun.Spec.Kill = false
			break StateMachine
		}

		switch actionToPerform {
		case v2pb.TRIGGER_RUN_ACTION_KILL:
			status, err = runner.Kill(ctx, triggerRun)
			if err != nil {
				log.Error(err, "failed to kill scheduled workflow")
				triggerRun.Status.ErrorMessage = err.Error()
				triggerRun.Status.State = status.State
				break StateMachine
			}
			log.Info("trigger run killed")
			triggerRun.Status = status
			triggerRun.Spec.Action = v2pb.TRIGGER_RUN_ACTION_NO_ACTION
			triggerRun.Spec.Kill = false
			break StateMachine

		case v2pb.TRIGGER_RUN_ACTION_PAUSE:
			status, err = runner.Pause(ctx, triggerRun)
			if err != nil {
				log.Error(err, "failed to pause scheduled workflow")
				triggerRun.Status.ErrorMessage = err.Error()
				triggerRun.Status.State = status.State
				break StateMachine
			}
			log.Info("trigger run paused")
			triggerRun.Status = status
			triggerRun.Spec.Action = v2pb.TRIGGER_RUN_ACTION_NO_ACTION
			break StateMachine
		}

		status2, err := runner.GetStatus(ctx, triggerRun)
		if err != nil {
			log.Error(err, "TriggerRun GetStatus failed")
			triggerRun.Status.ErrorMessage = err.Error()
			triggerRun.Status.State = status2.State
			break StateMachine
		}
		triggerRun.Status.State = status2.State

	case v2pb.TRIGGER_RUN_STATE_PAUSED:
		log.Info("TRIGGER_RUN_STATE_PAUSED")

		// Handle actions using the new action field
		actionToPerform := triggerRun.Spec.Action

		// Backward compat: if Spec.Kill is set and no explicit action, treat as KILL
		if actionToPerform == v2pb.TRIGGER_RUN_ACTION_NO_ACTION && triggerRun.Spec.Kill {
			actionToPerform = v2pb.TRIGGER_RUN_ACTION_KILL
		}

		// Sync TriggerRun spec changes to workflow engine, passing the action so cron
		// update and resume can be applied atomically in a single API call.
		status, actionHandled, err := runner.Update(ctx, triggerRun, actionToPerform)
		if err != nil {
			log.Error(err, "failed to sync trigger spec to workflow engine")
			// Even on error, merge the returned status to preserve fields like
			// ActualNotifications that were synced despite the update failure.
			// This prevents infinite retry loops when workflows hit signal limits.
			triggerRun.Status.ErrorMessage = err.Error()
			triggerRun.Status.State = status.State
			if status.ActualNotifications != nil {
				triggerRun.Status.ActualNotifications = status.ActualNotifications
			}
			break StateMachine
		}
		triggerRun.Status = status

		// If Update already applied the action atomically, clear and skip
		if actionHandled {
			triggerRun.Spec.Action = v2pb.TRIGGER_RUN_ACTION_NO_ACTION
			triggerRun.Spec.Kill = false
			break StateMachine
		}

		switch actionToPerform {
		case v2pb.TRIGGER_RUN_ACTION_KILL:
			status, err = runner.Kill(ctx, triggerRun)
			if err != nil {
				log.Error(err, "failed to kill paused workflow")
				triggerRun.Status.ErrorMessage = err.Error()
				triggerRun.Status.State = status.State
				break StateMachine
			}
			log.Info("paused trigger run killed")
			triggerRun.Status = status
			triggerRun.Spec.Action = v2pb.TRIGGER_RUN_ACTION_NO_ACTION
			break StateMachine

		case v2pb.TRIGGER_RUN_ACTION_RESUME:
			status, err = runner.Resume(ctx, triggerRun)
			if err != nil {
				log.Error(err, "failed to resume scheduled workflow")
				triggerRun.Status.ErrorMessage = err.Error()
				triggerRun.Status.State = status.State
				break StateMachine
			}
			log.Info("trigger run resumed")
			triggerRun.Status = status
			triggerRun.Spec.Action = v2pb.TRIGGER_RUN_ACTION_NO_ACTION
			break StateMachine
		}
		// Stay paused if no action requested (no status change needed)
	}

	// Actions are one-time commands. Persist their reset through the main
	// resource endpoint before updating status so a status failure cannot cause
	// a successfully handled action to be replayed on the next reconciliation.
	specChanged := !reflect.DeepEqual(originalTriggerRun.Spec, triggerRun.Spec)
	statusChanged := !reflect.DeepEqual(originalTriggerRun.Status, triggerRun.Status)
	if specChanged {
		// The main-resource update returns the status currently stored by the
		// status subresource, so retain the state-machine result for the separate
		// status update below.
		desiredStatus := triggerRun.Status
		if err := r.Update(ctx, triggerRun, &metav1.UpdateOptions{}); err != nil {
			log.Error(err, "Fail to update trigger run spec")
			return ctrl.Result{}, err
		}
		triggerRun.Status = desiredStatus
	}
	if statusChanged {
		if err := r.UpdateStatus(ctx, triggerRun, &metav1.UpdateOptions{}); err != nil {
			log.Error(err, "Fail to update trigger run status")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

// ensureOwnerRef is a transitional MIGRATION: it stamps the owning Pipeline as
// the run's controller ownerReference on CRs that predate the apiserver
// BeforeCreate hook, which is the canonical place ownerRefs are set. Idempotent;
// a no-op once stamped or when the Pipeline/ref is absent.
//
// TODO(#1337): remove after the migration completes. New runs get their ownerRef
// from the BeforeCreate apihook — all supported creates (CLI + triggers) route
// through ma-apiserver; runs created outside it are the creator's responsibility.
func (r *Reconciler) ensureOwnerRef(ctx context.Context, log logr.Logger, triggerRun *v2pb.TriggerRun) error {
	pipelineRef := triggerRun.Spec.GetPipeline()
	if pipelineRef == nil || pipelineRef.GetName() == "" {
		return nil
	}
	namespace := pipelineRef.GetNamespace()
	if namespace == "" {
		// ownerReferences are namespace-local; default to the run's own
		// namespace when the reference omits one.
		namespace = triggerRun.GetNamespace()
	}

	pipeline := &v2pb.Pipeline{}
	if err := r.Get(ctx, namespace, pipelineRef.GetName(), &metav1.GetOptions{}, pipeline); err != nil {
		// The owning Pipeline may not exist (yet/anymore); skip quietly.
		if apiutils.IsNotFoundError(err) {
			return nil
		}
		return err
	}

	changed, err := cascadedelete.EnsureControllerRef(triggerRun, pipeline, r.scheme)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if err := r.Update(ctx, triggerRun, &metav1.UpdateOptions{}); err != nil {
		return err
	}
	cascadedelete.IncOwnerRefBackfill(metricKind)
	log.Info("Ensured Pipeline ownerReference on trigger run", "pipeline", pipelineRef.GetName())
	return nil
}

// ensureDrainFinalizer adds the drain finalizer to an active run so a Pipeline
// delete blocks on stopping its recurring cron/schedule before GC. No-op for
// terminal runs or once already present.
func (r *Reconciler) ensureDrainFinalizer(ctx context.Context, log logr.Logger, triggerRun *v2pb.TriggerRun) error {
	if isTerminateState(triggerRun) {
		return nil
	}
	if ctrlutil.ContainsFinalizer(triggerRun, drainFinalizer) {
		return nil
	}
	ctrlutil.AddFinalizer(triggerRun, drainFinalizer)
	if err := r.Update(ctx, triggerRun, &metav1.UpdateOptions{}); err != nil {
		return err
	}
	log.Info("Added cascade drain finalizer to trigger run")
	return nil
}

// triggerRunDrainTarget adapts a single TriggerRun to cascadedelete.DrainTarget. Each
// mutating method persists via the controller's api.Handler; the driver
// (cascadedelete.RunDrainStep) holds no client and writes only through these methods.
type triggerRunDrainTarget struct {
	r   *Reconciler
	log logr.Logger
	run *v2pb.TriggerRun
}

// triggerRunWorkStarted reports whether a schedule/workflow was ever created, from
// the persisted state — a live "open run" probe would mis-read an idle recurring
// schedule as never-started and leave it armed.
func triggerRunWorkStarted(run *v2pb.TriggerRun) bool {
	return run.Status.State != v2pb.TRIGGER_RUN_STATE_INVALID
}

// RequestCancel stops the recurring cron (idempotent kill) and stamps the
// drain-counted token in one persisted update.
func (t *triggerRunDrainTarget) RequestCancel(ctx context.Context) error {
	runner := t.r.getRunner(t.run)
	status, err := runner.Kill(ctx, t.run)
	if err != nil {
		return fmt.Errorf("kill trigger run %s/%s during drain: %w", t.run.Namespace, t.run.Name, err)
	}
	t.run.Status = status
	cascadedelete.MarkDrainCounted(t.run)
	if err := t.r.Update(ctx, t.run, &metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("begin drain for trigger run %s/%s: %w", t.run.Namespace, t.run.Name, err)
	}
	t.log.Info("killed trigger run during drain", "state", t.run.Status.State.String())
	return nil
}

// Progress re-issues the idempotent kill and re-checks terminal, so a failed prior
// status-update cannot wedge the drain. It persists status and reports terminal.
func (t *triggerRunDrainTarget) Progress(ctx context.Context) (bool, error) {
	runner := t.r.getRunner(t.run)
	status, err := runner.Kill(ctx, t.run)
	if err != nil {
		return false, fmt.Errorf("re-issue kill for trigger run %s/%s during drain: %w", t.run.Namespace, t.run.Name, err)
	}
	t.run.Status = status
	if err := t.r.UpdateStatus(ctx, t.run, &metav1.UpdateOptions{}); err != nil {
		return false, err
	}
	return isTerminateState(t.run), nil
}

// MarkKilled drives a never-started run straight to terminal KILLED without engine
// work, persisting status. It must NOT stamp the drain-counted token.
func (t *triggerRunDrainTarget) MarkKilled(ctx context.Context) error {
	t.run.Status.State = v2pb.TRIGGER_RUN_STATE_KILLED
	if err := t.r.UpdateStatus(ctx, t.run, &metav1.UpdateOptions{}); err != nil {
		return err
	}
	t.log.Info("draining trigger run that never started; marked KILLED")
	return nil
}

// ForceKill performs best-effort engine teardown on the safety timeout, deleting
// the trigger so no schedule stays armed to spawn orphaned workflows. Errors are
// logged and swallowed by the driver.
func (t *triggerRunDrainTarget) ForceKill(ctx context.Context) error {
	if err := ForceKillWorkflow(ctx, t.run, t.log, t.r.workflowClient); err != nil {
		t.log.Error(err, "force-kill workflow during drain timeout failed")
		return err
	}
	return nil
}

// CompleteDrain finalizes in ONE persisted metadata update: it marks the run
// immutable (a TriggerRun always retains — no TTL, no storage gate), clears the
// drain-counted token, and removes the drain finalizer.
func (t *triggerRunDrainTarget) CompleteDrain(ctx context.Context) error {
	if !apiutils.IsImmutable(t.run) {
		apiutils.MarkImmutable(t.run)
	}
	cascadedelete.ClearDrainCounted(t.run)
	ctrlutil.RemoveFinalizer(t.run, drainFinalizer)
	if err := t.r.Update(ctx, t.run, &metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("complete drain for trigger %s/%s: %w", t.run.Namespace, t.run.Name, err)
	}
	t.log.Info("removed cascade drain finalizer from trigger run")
	return nil
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
	r.scheme = mgr.GetScheme()
	handler, err := r.apiHandlerFactory.GetAPIHandler(mgr.GetClient())
	if err != nil {
		return err
	}
	r.Handler = handler
	r.log = mgr.GetLogger().
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
		return r.intervalTrigger
	case TriggerTypeBackfill:
		return r.backfillTrigger
	case TriggerTypeBatchRerun:
		return r.batchRerunTrigger
	default:
		return r.cronTrigger
	}
}
