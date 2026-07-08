// Package pipelinerun implements a Kubernetes controller for managing PipelineRun resources.
//
// The controller orchestrates the execution of machine learning pipelines by coordinating
// multiple stages through a condition-based engine:
//   - Source pipeline retrieval and validation
//   - Image building and management
//   - Workflow execution via Cadence/Temporal
//
// Each stage is implemented as a ConditionActor that checks prerequisites and executes
// actions. The controller manages state transitions and ensures consistent status updates
// for long-running pipeline executions.
//
// For cascade delete, the loop stamps the owning Pipeline ownerReference and a
// drain finalizer so that when the Pipeline is deleted, the run's in-flight
// workflow is drained before GC removes it.
package pipelinerun

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	defaultEngine "github.com/michelangelo-ai/michelangelo/go/base/conditions/engine"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"github.com/michelangelo-ai/michelangelo/go/cascadedelete"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/notification"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/plugin"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	// drainFinalizer blocks garbage collection of a run until its workflow has
	// been drained. It is BYTE-IDENTICAL across releases for rollout safety
	// (see the cascade-delete plan §8) — do not change this string.
	drainFinalizer = "pipelineruns.michelangelo.uber.com/drain"

	// metricKind is this kind's cascade metric label. It is a documented dashboard
	// contract (see the cascade-delete plan §8) — do not change this value.
	metricKind = "pipeline_run"

	// drainRequeueInterval is how often a PipelineRun being drained for
	// cascade-delete is re-reconciled while its workflow is still terminating. It
	// matches the engine's default inactive requeue period.
	drainRequeueInterval = 10 * time.Second
)

// Config holds configuration for the PipelineRun controller.
type Config struct {
	// TTLDays is how long after last update a terminal PipelineRun is kept in
	// ETCD before being marked immutable and evicted to MySQL-only storage.
	// Zero means TTL eviction is disabled.
	// Note: TTL eviction only works when metadata storage is enabled.
	TTLDays int `yaml:"ttlDays"`
}

// Reconciler implements the controller-runtime Reconciler interface for PipelineRun resources.
//
// It manages the execution lifecycle of pipeline runs through a condition-based engine,
// coordinating multiple actors (source pipeline, image build, workflow execution) to
// progress pipeline runs through their various states. The reconciler tracks execution
// status and updates the PipelineRun resource accordingly.
type Reconciler struct {
	api.Handler
	logger                 *zap.Logger
	config                 Config
	metadataStorageEnabled bool // Cached at initialization - doesn't change at runtime
	plugin                 *plugin.Plugin
	engine                 *defaultEngine.DefaultEngine[*v2pb.PipelineRun]
	// workflowClient drives direct workflow teardown on the cascade safety-timeout
	// path (ForceKill); graceful cancellation goes through the engine via Spec.Kill.
	workflowClient    clientInterface.WorkflowClient
	apiHandlerFactory apiHandler.Factory
	notifier          *notification.PipelineRunNotifier
	scheme            *runtime.Scheme
}

// NewReconciler creates a new PipelineRun controller reconciler.
//
// The reconciler is initialized with a condition-based engine that orchestrates
// pipeline execution through the provided plugin's actors. The logger is enhanced
// with component-specific fields for better observability.
//
// Parameters:
//   - plugin: Contains the ConditionActors for pipeline execution stages
//   - workflowClient: Client used to tear down a run's workflow on the cascade safety-timeout path
//   - logger: Structured logger for the controller
//   - apiHandlerFactory: Factory for creating API handlers to interact with Kubernetes
//   - notifier: Handles pipeline run notifications for state changes
//   - config: PipelineRun controller configuration including TTL settings
//   - metadataStorageConfig: Metadata storage configuration to determine if MySQL backup exists
//
// Returns a configured Reconciler ready to be registered with a controller manager.
func NewReconciler(
	plugin *plugin.Plugin,
	workflowClient clientInterface.WorkflowClient,
	logger *zap.Logger,
	apiHandlerFactory apiHandler.Factory,
	notifier *notification.PipelineRunNotifier,
	config Config,
	metadataStorageConfig storage.MetadataStorageConfig,
) *Reconciler {
	logger = logger.With(zap.String("component", "pipelinerun"))
	return &Reconciler{
		plugin:                 plugin,
		workflowClient:         workflowClient,
		logger:                 logger,
		config:                 config,
		metadataStorageEnabled: storage.EnableMetadataStorage(&metadataStorageConfig),
		engine:                 defaultEngine.NewDefaultEngine[*v2pb.PipelineRun](logger),
		apiHandlerFactory:      apiHandlerFactory,
		notifier:               notifier,
	}
}

// Reconcile is the main reconciliation loop entry point for PipelineRun resources.
//
// It processes reconciliation requests by running the pipeline through the condition
// engine, which executes registered actors in sequence. Based on the engine's results,
// it updates the PipelineRun state:
//   - RUNNING: Pipeline execution is in progress
//   - SUCCEEDED: All conditions satisfied successfully
//   - FAILED: One or more conditions failed
//   - KILLED: Pipeline was explicitly terminated
//
// The method ensures that status changes are persisted to Kubernetes and returns
// appropriate requeue results for ongoing executions.
//
// Returns a Result indicating requeue behavior and an error if reconciliation fails.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pipelineRun := &v2pb.PipelineRun{}
	logger := r.logger.With(zap.String("namespace-name", req.NamespacedName.String()))
	logger.Info("Reconciling pipeline run starts")
	if err := r.Get(ctx, req.Namespace, req.Name, &metav1.GetOptions{}, pipelineRun); err != nil {
		return ctrl.Result{}, fmt.Errorf("get pipeline run %q: %w", req.NamespacedName, err)
	}

	// Cascade-delete bookkeeping. GC stamps a deletionTimestamp when the owning
	// Pipeline is foreground-deleted: drain the in-flight workflow before letting
	// GC remove the run.
	if !pipelineRun.GetDeletionTimestamp().IsZero() {
		st := cascadedelete.DrainState{
			Object:      pipelineRun,
			Kind:        metricKind,
			Finalizer:   drainFinalizer,
			IsTerminal:  isTerminalState(pipelineRun.Status.State),
			WorkStarted: pipelineRunWorkStarted(pipelineRun),
		}
		return cascadedelete.RunDrainStep(ctx, st, &pipelineRunDrainTarget{r: r, logger: logger, run: pipelineRun}, drainRequeueInterval)
	}
	// Finalizer before ownerRef: the ownerRef makes the run GC-eligible, so the
	// finalizer must be present first or GC could remove the run with its workflow
	// still live.
	if err := r.ensureDrainFinalizer(ctx, logger, pipelineRun); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureOwnerRef(ctx, logger, pipelineRun); err != nil {
		return ctrl.Result{}, err
	}

	originalPipelineRun := pipelineRun.DeepCopy()
	conditionResult, err := r.engine.Run(ctx, r.plugin, pipelineRun)
	result := conditionResult.Result
	var returnErr error
	if err != nil {
		logger.Error("Failed to run engine",
			zap.Error(err),
			zap.String("operation", "run_engine"),
			zap.String("namespace", req.Namespace),
			zap.String("name", req.Name))
		returnErr = fmt.Errorf("run engine for pipeline run %q: %w", req.NamespacedName, err)
		IncPipelineRunReconcileError(req.Namespace, req.Name)
	} else {
		if conditionResult.IsKilled {
			pipelineRun.Status.State = v2pb.PIPELINE_RUN_STATE_KILLED
		} else if !conditionResult.IsTerminal {
			pipelineRun.Status.State = v2pb.PIPELINE_RUN_STATE_RUNNING
		} else if conditionResult.AreSatisfied {
			pipelineRun.Status.State = v2pb.PIPELINE_RUN_STATE_SUCCEEDED
		} else {
			pipelineRun.Status.State = v2pb.PIPELINE_RUN_STATE_FAILED
		}
	}

	// Check if state changed to terminal state and emit metrics
	originalIsTerminal := isTerminalState(originalPipelineRun.Status.State)
	currentIsTerminal := isTerminalState(pipelineRun.Status.State)
	if !originalIsTerminal && currentIsTerminal {
		r.emitPipelineRunMetrics(pipelineRun)
	}

	if err = r.updatePipelineRunStatus(ctx, pipelineRun, originalPipelineRun); err != nil {
		if returnErr != nil {
			logger.Error("Failed to update pipeline run status", zap.Error(err))
			return result, fmt.Errorf("update pipeline run status for %q: %w (previous error: %w)", req.NamespacedName, err, returnErr)
		}
		logger.Error("Failed to update pipeline run status",
			zap.Error(err),
			zap.String("operation", "update_status"),
			zap.String("namespace", req.Namespace),
			zap.String("name", req.Name))
		returnErr = fmt.Errorf("update pipeline run status for %q: %w", req.NamespacedName, err)
		IncPipelineRunReconcileError(req.Namespace, req.Name)
	} else if currentIsTerminal {
		// Only count as successful reconciliation when reaching terminal state
		IncPipelineRunReconcileSuccess(req.Namespace, req.Name)
	}

	// Send notifications after status is persisted. Dispatching before the
	// status write would re-fire the notification on every reconcile until the
	// write succeeds (controller restart or conflict between dispatch and persist).
	if returnErr == nil && r.notifier != nil {
		if notificationErr := r.notifier.NotifyOnStateChange(ctx, originalPipelineRun, pipelineRun); notificationErr != nil {
			logger.Warn("Failed to send notifications",
				zap.Error(notificationErr),
				zap.String("pipeline_run", req.NamespacedName.String()))
			// Don't fail reconciliation due to notification errors
		}
	}

	// For terminal runs, check if TTL has elapsed and mark immutable if so.
	// This evicts the run from ETCD once it's old enough, keeping ETCD lean.
	// CRITICAL: Only do this when metadata storage is enabled (MySQL backup exists).
	// Otherwise, evicting from ETCD would permanently delete the records.
	if returnErr == nil && currentIsTerminal && r.metadataStorageEnabled {
		if requeueAt, done := r.markImmutableIfExpired(ctx, logger, pipelineRun); !done {
			return ctrl.Result{RequeueAfter: requeueAt}, nil
		}
	}

	return result, returnErr
}

// ensureOwnerRef is a transitional MIGRATION: it stamps the owning Pipeline as
// the run's controller ownerReference on CRs that predate the apiserver
// BeforeCreate hook, which is the canonical place ownerRefs are set. Idempotent;
// a no-op once stamped or when the Pipeline/ref is absent.
//
// TODO(#1337): remove after the migration completes. New runs get their ownerRef
// from the BeforeCreate apihook — all supported creates (CLI + triggers) route
// through ma-apiserver; runs created outside it are the creator's responsibility.
func (r *Reconciler) ensureOwnerRef(ctx context.Context, logger *zap.Logger, pipelineRun *v2pb.PipelineRun) error {
	pipelineRef := pipelineRun.Spec.GetPipeline()
	if pipelineRef == nil || pipelineRef.GetName() == "" {
		return nil
	}
	namespace := pipelineRef.GetNamespace()
	if namespace == "" {
		// ownerReferences are namespace-local; default to the run's own
		// namespace when the reference omits one.
		namespace = pipelineRun.GetNamespace()
	}

	pipeline := &v2pb.Pipeline{}
	if err := r.Get(ctx, namespace, pipelineRef.GetName(), &metav1.GetOptions{}, pipeline); err != nil {
		// The owning Pipeline may not exist (yet/anymore); skip quietly.
		if utils.IsNotFoundError(err) {
			return nil
		}
		return err
	}

	changed, err := cascadedelete.EnsureControllerRef(pipelineRun, pipeline, r.scheme)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if err := r.Update(ctx, pipelineRun, &metav1.UpdateOptions{}); err != nil {
		return err
	}
	cascadedelete.IncOwnerRefBackfill(metricKind)
	logger.Info("Ensured Pipeline ownerReference on pipeline run", zap.String("pipeline", pipelineRef.GetName()))
	return nil
}

// ensureDrainFinalizer adds the drain finalizer to active runs so a Pipeline
// delete blocks on draining their workflow before GC, rather than orphaning live
// Spark/Ray compute. No-op for terminal runs or once already present.
func (r *Reconciler) ensureDrainFinalizer(ctx context.Context, logger *zap.Logger, pipelineRun *v2pb.PipelineRun) error {
	if isTerminalState(pipelineRun.Status.State) {
		return nil
	}
	if ctrlutil.ContainsFinalizer(pipelineRun, drainFinalizer) {
		return nil
	}
	ctrlutil.AddFinalizer(pipelineRun, drainFinalizer)
	if err := r.Update(ctx, pipelineRun, &metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("add drain finalizer to pipeline run %q: %w", pipelineRun.Name, err)
	}
	logger.Info("Added drain finalizer to active pipeline run")
	return nil
}

// pipelineRunDrainTarget adapts a single PipelineRun to cascadedelete.DrainTarget. Each
// mutating method persists via the controller's api.Handler; the driver
// (cascadedelete.RunDrainStep) holds no client and writes only through these methods.
type pipelineRunDrainTarget struct {
	r      *Reconciler
	logger *zap.Logger
	run    *v2pb.PipelineRun
}

// pipelineRunWorkStarted reports whether the workflow actually started; an empty
// WorkflowId/WorkflowRunId means the ExecuteWorkflow actor never launched one, so
// there is nothing to cancel.
func pipelineRunWorkStarted(run *v2pb.PipelineRun) bool {
	return run.Status.WorkflowId != "" && run.Status.WorkflowRunId != ""
}

// RequestCancel sets Spec.Kill (which the ExecuteWorkflow actor reads to cancel
// the workflow and tear down Spark/Ray) and stamps the drain-counted token in one
// persisted update.
func (t *pipelineRunDrainTarget) RequestCancel(ctx context.Context) error {
	t.run.Spec.Kill = true
	cascadedelete.MarkDrainCounted(t.run)
	if err := t.r.Update(ctx, t.run, &metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("request kill for draining pipeline run %q: %w", t.run.Name, err)
	}
	t.logger.Info("PipelineRun drain started; requested workflow cancellation")
	return nil
}

// Progress drives the engine to advance the cancellation (reusing the exact path a
// user-initiated kill takes), maps the engine result to a state, persists status,
// and reports whether the run is now terminal.
func (t *pipelineRunDrainTarget) Progress(ctx context.Context) (bool, error) {
	originalPipelineRun := t.run.DeepCopy()
	conditionResult, err := t.r.engine.Run(ctx, t.r.plugin, t.run)
	if err != nil {
		return false, fmt.Errorf("run engine while draining pipeline run %q: %w", t.run.Name, err)
	}
	if conditionResult.IsKilled {
		t.run.Status.State = v2pb.PIPELINE_RUN_STATE_KILLED
	} else if !conditionResult.IsTerminal {
		t.run.Status.State = v2pb.PIPELINE_RUN_STATE_RUNNING
	} else if conditionResult.AreSatisfied {
		t.run.Status.State = v2pb.PIPELINE_RUN_STATE_SUCCEEDED
	} else {
		t.run.Status.State = v2pb.PIPELINE_RUN_STATE_FAILED
	}
	if err := t.r.updatePipelineRunStatus(ctx, t.run, originalPipelineRun); err != nil {
		return false, err
	}
	return isTerminalState(t.run.Status.State), nil
}

// MarkKilled drives the run straight to terminal KILLED without engine work (the
// workflow never started), persisting status. It must NOT stamp the drain-counted
// token.
func (t *pipelineRunDrainTarget) MarkKilled(ctx context.Context) error {
	originalPipelineRun := t.run.DeepCopy()
	t.run.Status.State = v2pb.PIPELINE_RUN_STATE_KILLED
	if err := t.r.updatePipelineRunStatus(ctx, t.run, originalPipelineRun); err != nil {
		return err
	}
	t.logger.Info("PipelineRun drain: workflow never started, marking killed without starting one")
	return nil
}

// ForceKill is the cascade safety-timeout teardown: it directly cancels the run's
// workflow via the workflow client using the recorded WorkflowId/WorkflowRunId, so
// a run that never drained gracefully (e.g. first reconciled past the timeout, or
// whose cancellation never succeeded) still has its Spark/Ray workflow torn down
// before GC removes it. Best-effort — the driver swallows the error. No-op when no
// workflow was ever started.
func (t *pipelineRunDrainTarget) ForceKill(ctx context.Context) error {
	wid, rid := t.run.Status.WorkflowId, t.run.Status.WorkflowRunId
	if wid == "" || rid == "" {
		return nil
	}
	return t.r.workflowClient.CancelWorkflow(ctx, wid, rid, defaultEngine.KillReason)
}

// CompleteDrain finalizes in ONE persisted metadata update: when metadata storage
// is enabled it marks the run immutable (so the ingester evicts it to MySQL-only
// storage); always it clears the drain-counted token and removes the drain
// finalizer.
func (t *pipelineRunDrainTarget) CompleteDrain(ctx context.Context) error {
	if t.r.metadataStorageEnabled {
		utils.MarkImmutable(t.run)
	}
	cascadedelete.ClearDrainCounted(t.run)
	ctrlutil.RemoveFinalizer(t.run, drainFinalizer)
	if err := t.r.Update(ctx, t.run, &metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("remove drain finalizer from pipeline run %q: %w", t.run.Name, err)
	}
	t.logger.Info("PipelineRun drained; removed drain finalizer")
	return nil
}

// markImmutableIfExpired checks whether a terminal PipelineRun has been idle
// longer than the configured TTL. If so, it sets the michelangelo/Immutable
// annotation so the ingester will evict it from ETCD to MySQL-only storage.
//
// Returns (requeueAfter, done):
//   - done=true: TTL has elapsed; annotation was set (or was already set).
//   - done=false: TTL has not elapsed yet; caller should requeue after requeueAfter.
func (r *Reconciler) markImmutableIfExpired(ctx context.Context, logger *zap.Logger, pipelineRun *v2pb.PipelineRun) (time.Duration, bool) {
	var lastUpdate time.Time
	if endTime := pipelineRun.Status.GetEndTime(); endTime != nil {
		lastUpdate = time.Unix(endTime.Seconds, int64(endTime.Nanos))
	} else {
		// EndTime not set yet — fall back to creation timestamp.
		lastUpdate = pipelineRun.GetCreationTimestamp().Time
	}
	if lastUpdate.IsZero() {
		return 0, true
	}

	expireAt := lastUpdate.Add(time.Duration(r.config.TTLDays) * 24 * time.Hour)
	remaining := time.Until(expireAt)

	if remaining > 0 {
		logger.Info("PipelineRun TTL not yet elapsed, requeueing",
			zap.String("name", pipelineRun.Name),
			zap.Duration("requeueAfter", remaining))
		return remaining, false
	}

	// TTL elapsed — set immutable annotation if not already set.
	annotations := pipelineRun.GetAnnotations()
	if annotations[api.ImmutableAnnotation] == "true" {
		return 0, true
	}
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[api.ImmutableAnnotation] = "true"
	pipelineRun.SetAnnotations(annotations)
	if err := r.Update(ctx, pipelineRun, &metav1.UpdateOptions{}); err != nil {
		logger.Error("Failed to mark PipelineRun immutable", zap.Error(err))
	}
	return 0, true
}

// updatePipelineRunStatus persists PipelineRun status changes to Kubernetes.
//
// It performs a deep comparison between the original and updated status to avoid
// unnecessary API calls. Only when changes are detected is the status updated via
// the Kubernetes API.
//
// Returns an error if the status update fails.
func (r *Reconciler) updatePipelineRunStatus(ctx context.Context, pipelineRun *v2pb.PipelineRun, originalPipelineRun *v2pb.PipelineRun) error {
	if !reflect.DeepEqual(pipelineRun.Status, originalPipelineRun.Status) {
		if err := r.UpdateStatus(ctx, pipelineRun, &metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update status for pipeline run %q: %w", pipelineRun.Name, err)
		}
	}
	return nil
}

// Register sets up the PipelineRun controller with the controller-runtime manager.
//
// It initializes the API handler from the factory and configures the controller
// to watch PipelineRun resources. The controller will reconcile all PipelineRun
// objects whenever they are created, updated, or when reconciliation is triggered.
//
// Returns an error if the API handler cannot be created or controller registration fails.
func (r *Reconciler) Register(mgr ctrl.Manager) error {
	handler, err := r.apiHandlerFactory.GetAPIHandler(mgr.GetClient())
	if err != nil {
		return err
	}
	r.Handler = handler
	r.scheme = mgr.GetScheme()
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.PipelineRun{}).
		Complete(r)
}

// isTerminalState checks if a pipeline run state is terminal
func isTerminalState(state v2pb.PipelineRunState) bool {
	return state == v2pb.PIPELINE_RUN_STATE_SUCCEEDED ||
		state == v2pb.PIPELINE_RUN_STATE_FAILED ||
		state == v2pb.PIPELINE_RUN_STATE_KILLED
}

// emitPipelineRunMetrics emits metrics for completed pipeline runs
func (r *Reconciler) emitPipelineRunMetrics(pipelineRun *v2pb.PipelineRun) {
	labels := extractMetricLabels(pipelineRun)

	// Calculate duration if we have creation timestamp
	objMeta := pipelineRun.GetObjectMeta()
	if objMeta != nil && !objMeta.GetCreationTimestamp().Time.IsZero() {
		duration := time.Since(objMeta.GetCreationTimestamp().Time)
		ObservePipelineRunDuration(labels, duration)
	}

	// Emit result counter with state
	IncPipelineRunResult(labels)

	// Emit specific success/failure counters and gauge
	switch pipelineRun.Status.State {
	case v2pb.PIPELINE_RUN_STATE_SUCCEEDED:
		IncPipelineRunResultSuccess(labels)
		SetPipelineRunFailed(labels, false)
	case v2pb.PIPELINE_RUN_STATE_FAILED:
		IncPipelineRunResultFailure(labels)
		SetPipelineRunFailed(labels, true)
	case v2pb.PIPELINE_RUN_STATE_KILLED:
		SetPipelineRunFailed(labels, true)
	}

	// Emit step-level success metrics
	emitStepSuccessMetrics(pipelineRun, labels.PipelineType)
}

// emitStepSuccessMetrics recursively emits metrics for successful pipeline steps
func emitStepSuccessMetrics(pipelineRun *v2pb.PipelineRun, pipelineType string) {
	if len(pipelineRun.Status.Steps) == 0 {
		return
	}

	// Helper function to recursively process steps and sub-steps
	var processSteps func(steps []*v2pb.PipelineRunStepInfo)
	processSteps = func(steps []*v2pb.PipelineRunStepInfo) {
		for _, step := range steps {
			if step.State == v2pb.PIPELINE_RUN_STEP_STATE_SUCCEEDED {
				IncPipelineRunStepSuccess(
					pipelineRun.Namespace,
					pipelineRun.Name,
					step.Name,
					pipelineType,
				)
			}
			// Recursively process sub-steps
			if len(step.SubSteps) > 0 {
				processSteps(step.SubSteps)
			}
		}
	}

	processSteps(pipelineRun.Status.Steps)
}

// extractMetricLabels extracts all metric labels from a pipeline run
func extractMetricLabels(pipelineRun *v2pb.PipelineRun) PipelineRunMetricLabels {
	labels := PipelineRunMetricLabels{
		Namespace:     pipelineRun.Namespace,
		PipelineRun:   pipelineRun.Name,
		State:         pipelineRun.Status.State.String(),
		PipelineType:  getPipelineType(pipelineRun),
		Environment:   getEnvironment(pipelineRun),
		Tier:          getTier(pipelineRun),
		Region:        getRegion(pipelineRun),
		Zone:          getZone(pipelineRun),
		FailureReason: getFailureReason(pipelineRun),
	}
	return labels
}

// getPipelineType extracts the pipeline type from the pipeline run
// Returns "unknown" if the type cannot be determined
func getPipelineType(pipelineRun *v2pb.PipelineRun) string {
	// The pipeline type would need to be extracted from the source pipeline
	// or from labels/annotations. For now, return unknown.
	// In a full implementation, you would fetch the Pipeline resource
	// and extract the type from pipeline.Spec.Type
	if pipelineRun.Labels != nil {
		if pipelineType, ok := pipelineRun.Labels["pipelinerun.michelangelo/pipeline-type"]; ok {
			return pipelineType
		}
	}
	return "unknown"
}

// getEnvironment extracts the environment from pipeline run labels
// Returns "unknown" if not set
func getEnvironment(pipelineRun *v2pb.PipelineRun) string {
	if pipelineRun.Labels != nil {
		if env, ok := pipelineRun.Labels["pipelinerun.michelangelo/environment"]; ok {
			return env
		}
	}
	return "unknown"
}

// getTier extracts the project tier
// Returns "unknown" if not available
func getTier(pipelineRun *v2pb.PipelineRun) string {
	// Note: In a full implementation, you would fetch the Project resource
	// and extract the tier from project.Spec.Tier
	// For now, return unknown to avoid additional API calls
	return "unknown"
}

// getRegion extracts the region from annotations or labels
// Returns "unknown" if not set
func getRegion(pipelineRun *v2pb.PipelineRun) string {
	// Check annotations first
	if pipelineRun.Annotations != nil {
		if region, ok := pipelineRun.Annotations["pipelinerun.michelangelo/region"]; ok {
			return region
		}
	}
	// Check labels
	if pipelineRun.Labels != nil {
		if region, ok := pipelineRun.Labels["pipelinerun.michelangelo/region"]; ok {
			return region
		}
	}
	return "unknown"
}

// getZone extracts the zone from annotations or labels
// Returns "unknown" if not set
func getZone(pipelineRun *v2pb.PipelineRun) string {
	// Check annotations first
	if pipelineRun.Annotations != nil {
		if zone, ok := pipelineRun.Annotations["pipelinerun.michelangelo/zone"]; ok {
			return zone
		}
	}
	// Check labels
	if pipelineRun.Labels != nil {
		if zone, ok := pipelineRun.Labels["pipelinerun.michelangelo/zone"]; ok {
			return zone
		}
	}
	return "unknown"
}

// getFailureReason extracts the failure reason from the pipeline run
// Returns "none" if not failed or reason not available
func getFailureReason(pipelineRun *v2pb.PipelineRun) string {
	if pipelineRun.Status.State == v2pb.PIPELINE_RUN_STATE_FAILED {
		if pipelineRun.Status.ErrorMessage != "" {
			// Truncate and sanitize error message for use as a metric label
			reason := pipelineRun.Status.ErrorMessage
			if len(reason) > 50 {
				reason = reason[:50]
			}
			return reason
		}
		return "unknown_failure"
	}
	return "none"
}
