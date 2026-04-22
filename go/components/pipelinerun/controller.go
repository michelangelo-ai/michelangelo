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
package pipelinerun

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	defaultEngine "github.com/michelangelo-ai/michelangelo/go/base/conditions/engine"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/notification"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/plugin"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Reconciler implements the controller-runtime Reconciler interface for PipelineRun resources.
//
// It manages the execution lifecycle of pipeline runs through a condition-based engine,
// coordinating multiple actors (source pipeline, image build, workflow execution) to
// progress pipeline runs through their various states. The reconciler tracks execution
// status and updates the PipelineRun resource accordingly.
type Reconciler struct {
	api.Handler
	logger            *zap.Logger
	plugin            *plugin.Plugin
	engine            *defaultEngine.DefaultEngine[*v2pb.PipelineRun]
	apiHandlerFactory apiHandler.Factory
	notifier          *notification.PipelineRunNotifier
}

// NewReconciler creates a new PipelineRun controller reconciler.
//
// The reconciler is initialized with a condition-based engine that orchestrates
// pipeline execution through the provided plugin's actors. The logger is enhanced
// with component-specific fields for better observability.
//
// Parameters:
//   - plugin: Contains the ConditionActors for pipeline execution stages
//   - logger: Structured logger for the controller
//   - apiHandlerFactory: Factory for creating API handlers to interact with Kubernetes
//   - notifier: Handles pipeline run notifications for state changes
//
// Returns a configured Reconciler ready to be registered with a controller manager.
func NewReconciler(
	plugin *plugin.Plugin,
	logger *zap.Logger,
	apiHandlerFactory apiHandler.Factory,
	notifier *notification.PipelineRunNotifier,
) *Reconciler {
	logger = logger.With(zap.String("component", "pipelinerun"))
	return &Reconciler{
		plugin:            plugin,
		logger:            logger,
		engine:            defaultEngine.NewDefaultEngine[*v2pb.PipelineRun](logger),
		apiHandlerFactory: apiHandlerFactory,
		notifier:          notifier,
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

	// Send notifications for state changes (non-blocking)
	if notificationErr := r.notifier.NotifyOnStateChange(ctx, originalPipelineRun, pipelineRun); notificationErr != nil {
		logger.Warn("Failed to send notifications",
			zap.Error(notificationErr),
			zap.String("pipeline_run", req.NamespacedName.String()))
		// Don't fail reconciliation due to notification errors
	}

	// Check if state changed to terminal state and emit metrics
	originalIsTerminal := IsTerminalState(originalPipelineRun.Status.State)
	currentIsTerminal := IsTerminalState(pipelineRun.Status.State)
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

	return result, returnErr
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.PipelineRun{}).
		Complete(r)
}

// IsTerminalState checks if a pipeline run state is terminal.
//
// Terminal states (SUCCEEDED, FAILED, KILLED, SKIPPED) indicate the pipeline
// run has reached a final state and will not transition further. SKIPPED is
// included per the proto contract (proto/api/v2/pipeline_run.proto); omitting
// it would cause cascade delete to treat SKIPPED runs as active forever.
func IsTerminalState(state v2pb.PipelineRunState) bool {
	return state == v2pb.PIPELINE_RUN_STATE_SUCCEEDED ||
		state == v2pb.PIPELINE_RUN_STATE_FAILED ||
		state == v2pb.PIPELINE_RUN_STATE_KILLED ||
		state == v2pb.PIPELINE_RUN_STATE_SKIPPED
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
