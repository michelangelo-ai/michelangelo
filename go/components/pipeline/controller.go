// Package pipeline implements a Kubernetes controller for managing Pipeline resources.
//
// The controller watches Pipeline custom resources and reconciles their state by:
//   - Updating the latest revision reference
//   - Managing pipeline state transitions
//   - Scheduling periodic reconciliation for non-terminal states
//
// The controller integrates with the Michelangelo API handler to perform CRUD
// operations on Pipeline resources and updates their status accordingly.
package pipeline

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun"
	"github.com/michelangelo-ai/michelangelo/go/components/triggerrun"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// reconcileInterval defines how frequently non-terminal pipelines are reconciled.
	reconcileInterval = 10 * time.Second
)

// Reconciler implements the controller-runtime Reconciler interface for Pipeline resources.
//
// It manages the reconciliation loop for Pipeline custom resources, handling state
// updates and revision tracking. The reconciler uses an API handler for Kubernetes
// operations and maintains environment context and logging capabilities.
type Reconciler struct {
	api.Handler
	env                env.Context
	logger             *zap.Logger
	apiHandlerFactory  apiHandler.Factory
	triggerRunManager  triggerrun.Manager
	pipelineRunManager pipelinerun.Manager
}

// Reconcile is the main reconciliation loop entry point for Pipeline resources.
//
// It processes reconciliation requests for Pipeline objects by:
//   - Retrieving the Pipeline resource from Kubernetes
//   - Updating the latest revision reference based on the pipeline's git commit
//   - Transitioning the pipeline state to READY
//   - Persisting status updates back to Kubernetes
//
// The reconcile loop will requeue non-terminal pipelines at regular intervals
// to ensure continuous monitoring. Terminal states (READY, ERROR) do not requeue.
//
// Returns a Result indicating whether to requeue and an error if reconciliation failed.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger.With(zap.String("namespace-name", req.NamespacedName.String()))
	logger.Info("Reconciling pipeline starts")
	pipeline := &v2pb.Pipeline{}
	if err := r.Get(ctx, req.Namespace, req.Name, &metav1.GetOptions{}, pipeline); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Manage finalizer lifecycle for cascade delete
	if pipeline.GetDeletionTimestamp().IsZero() {
		if !controllerutil.ContainsFinalizer(pipeline, api.PipelineFinalizer) {
			controllerutil.AddFinalizer(pipeline, api.PipelineFinalizer)
			if err := r.Update(ctx, pipeline, &metav1.UpdateOptions{}); err != nil {
				return ctrl.Result{}, fmt.Errorf("add pipeline finalizer: %w", err)
			}
		}
	} else {
		return r.handleDeletion(ctx, pipeline, logger)
	}

	originalPipeline := pipeline.DeepCopy()
	state := pipeline.Status.State
	logger.Info("Reconciling pipeline", zap.Any("PipelineStatusState", state.String()))
	pipeline.Status.LatestRevision = &apipb.ResourceIdentifier{
		Name:      formatRevisionName(pipeline),
		Namespace: pipeline.Namespace,
	}
	pipeline.Status.State = v2pb.PIPELINE_STATE_READY

	// Emit metrics for pipeline becoming ready
	if originalPipeline.Status.State != v2pb.PIPELINE_STATE_READY && pipeline.Status.State == v2pb.PIPELINE_STATE_READY {
		IncPipelineReady(pipeline.Namespace, pipeline.Name, pipeline.Spec.Type.String())
	}

	result, err := r.updatePipelineStatus(ctx, pipeline, originalPipeline, logger)

	// Emit reconciliation metrics
	if err != nil {
		IncPipelineReconcileError(pipeline.Namespace, pipeline.Name)
	} else if pipeline.Status.State == v2pb.PIPELINE_STATE_READY {
		IncPipelineReconcileSuccess(pipeline.Namespace, pipeline.Name)
	}

	return result, err
}

// updatePipelineStatus persists pipeline status changes to Kubernetes.
//
// It compares the original and updated pipeline status and writes changes
// to the API server if they differ. For non-terminal states, it schedules
// requeue after the reconcileInterval to ensure continued reconciliation.
//
// Returns a Result with requeue information and an error if the update fails.
func (r *Reconciler) updatePipelineStatus(ctx context.Context, pipeline *v2pb.Pipeline, originalPipeline *v2pb.Pipeline, logger *zap.Logger) (ctrl.Result, error) {
	result := ctrl.Result{}
	if !isTerminatedState(pipeline.Status.State) {
		result = ctrl.Result{RequeueAfter: reconcileInterval}
	}
	if !reflect.DeepEqual(originalPipeline.Status, pipeline.Status) {
		logger.Info("Pipeline status updated", zap.Any("PipelineStatusState", pipeline.Status.State.String()))
		err := r.UpdateStatus(ctx, pipeline, &metav1.UpdateOptions{})
		if err != nil {
			logger.Error("Failed to update pipeline status",
				zap.Error(err),
				zap.String("operation", "update_status"),
				zap.String("namespace", pipeline.Namespace),
				zap.String("name", pipeline.Name))
			return result, fmt.Errorf("update pipeline status for %s/%s: %w", pipeline.Namespace, pipeline.Name, err)
		}
	}

	return result, nil
}

func (r *Reconciler) handleDeletion(ctx context.Context, pipeline *v2pb.Pipeline, logger *zap.Logger) (ctrl.Result, error) {
	// If the finalizer is not present we don't own this deletion; nothing to cascade.
	// Avoids wasted list/kill/delete work on pipelines that pre-date the finalizer rollout.
	if !controllerutil.ContainsFinalizer(pipeline, api.PipelineFinalizer) {
		return ctrl.Result{}, nil
	}
	logger.Info("Pipeline is being deleted, starting cascade delete")

	triggerRuns, err := r.triggerRunManager.ListTriggerRunsForPipeline(ctx, pipeline.Namespace, pipeline.Name)
	if err != nil {
		logger.Error("Failed to list trigger runs for cascade delete",
			zap.Error(err),
			zap.String("operation", "list_trigger_runs"),
			zap.String("namespace", pipeline.Namespace),
			zap.String("name", pipeline.Name))
		return ctrl.Result{}, fmt.Errorf("list trigger runs for pipeline %s/%s: %w", pipeline.Namespace, pipeline.Name, err)
	}

	pipelineRuns, err := r.pipelineRunManager.ListPipelineRunsForPipeline(ctx, pipeline.Namespace, pipeline.Name)
	if err != nil {
		logger.Error("Failed to list pipeline runs for cascade delete",
			zap.Error(err),
			zap.String("operation", "list_pipeline_runs"),
			zap.String("namespace", pipeline.Namespace),
			zap.String("name", pipeline.Name))
		return ctrl.Result{}, fmt.Errorf("list pipeline runs for pipeline %s/%s: %w", pipeline.Namespace, pipeline.Name, err)
	}

	if len(triggerRuns) == 0 && len(pipelineRuns) == 0 {
		logger.Info("No children found, removing finalizer")
		controllerutil.RemoveFinalizer(pipeline, api.PipelineFinalizer)
		if updateErr := r.Update(ctx, pipeline, &metav1.UpdateOptions{}); updateErr != nil {
			logger.Error("Failed to remove finalizer after cascade delete",
				zap.Error(updateErr),
				zap.String("operation", "remove_finalizer"),
				zap.String("namespace", pipeline.Namespace),
				zap.String("name", pipeline.Name))
			return ctrl.Result{}, fmt.Errorf("remove finalizer on pipeline %s/%s: %w", pipeline.Namespace, pipeline.Name, updateErr)
		}
		return ctrl.Result{}, nil
	}

	// Kill active TriggerRuns (best-effort)
	activeTRs, err := r.triggerRunManager.ListActiveTriggerRunsForPipeline(ctx, pipeline.Namespace, pipeline.Name)
	if err != nil {
		logger.Error("Failed to list active trigger runs for cascade delete",
			zap.Error(err),
			zap.String("operation", "list_active_trigger_runs"),
			zap.String("namespace", pipeline.Namespace),
			zap.String("name", pipeline.Name))
		return ctrl.Result{}, fmt.Errorf("list active trigger runs for pipeline %s/%s: %w", pipeline.Namespace, pipeline.Name, err)
	}
	if len(activeTRs) > 0 {
		for _, tr := range activeTRs {
			if killErr := r.triggerRunManager.KillTriggerRun(ctx, tr); killErr != nil {
				logger.Error("Failed to kill trigger run during cascade delete",
					zap.Error(killErr),
					zap.String("operation", "kill_trigger_run"),
					zap.String("namespace", tr.Namespace),
					zap.String("name", tr.Name))
			}
		}
		return ctrl.Result{RequeueAfter: reconcileInterval}, nil
	}

	// Kill active PipelineRuns (best-effort)
	activePRs, err := r.pipelineRunManager.ListActivePipelineRunsForPipeline(ctx, pipeline.Namespace, pipeline.Name)
	if err != nil {
		logger.Error("Failed to list active pipeline runs for cascade delete",
			zap.Error(err),
			zap.String("operation", "list_active_pipeline_runs"),
			zap.String("namespace", pipeline.Namespace),
			zap.String("name", pipeline.Name))
		return ctrl.Result{}, fmt.Errorf("list active pipeline runs for pipeline %s/%s: %w", pipeline.Namespace, pipeline.Name, err)
	}
	if len(activePRs) > 0 {
		for _, pr := range activePRs {
			if err := r.pipelineRunManager.KillPipelineRun(ctx, pr); err != nil {
				logger.Error("Failed to kill pipeline run during cascade delete",
					zap.Error(err),
					zap.String("operation", "kill_pipeline_run"),
					zap.String("namespace", pr.Namespace),
					zap.String("name", pr.Name))
			}
		}
		return ctrl.Result{RequeueAfter: reconcileInterval}, nil
	}

	// Delete steps will be added in subsequent PRs
	logger.Info("Children found, requeueing for cascade delete",
		zap.Int("triggerRuns", len(triggerRuns)),
		zap.Int("pipelineRuns", len(pipelineRuns)))
	return ctrl.Result{RequeueAfter: reconcileInterval}, nil
}

// formatRevisionName generates a standardized revision name for a pipeline.
//
// The name format is: "pipeline-{lowercase-pipeline-name}-{git-ref-prefix}"
// where git-ref-prefix is the first 12 characters (or less) of the git reference.
//
// For example: "pipeline-my-model-a1b2c3d4e5f6"
func formatRevisionName(pipeline *v2pb.Pipeline) string {
	if pipeline.Spec.Commit != nil {
		return fmt.Sprintf("%s-%s-%s", "pipeline", strings.ToLower(pipeline.Name), pipeline.Spec.Commit.GitRef[:min(len(pipeline.Spec.Commit.GitRef), 12)])
	}
	return ""
}

// isTerminatedState checks if a pipeline state is terminal.
//
// Terminal states (READY, ERROR) indicate the pipeline has reached a final
// state and does not require further reconciliation. Non-terminal states
// will continue to be reconciled at regular intervals.
func isTerminatedState(state v2pb.PipelineState) bool {
	return state == v2pb.PIPELINE_STATE_READY ||
		state == v2pb.PIPELINE_STATE_ERROR
}

// Register sets up the Pipeline controller with the controller-runtime manager.
//
// It initializes the API handler from the factory and configures the controller
// to watch Pipeline resources. The controller will reconcile all Pipeline objects
// in the cluster whenever they are created, updated, or deleted.
//
// Returns an error if the API handler cannot be created or the controller
// registration fails.
func (r *Reconciler) Register(mgr ctrl.Manager) error {
	handler, err := r.apiHandlerFactory.GetAPIHandler(mgr.GetClient())
	if err != nil {
		return err
	}
	r.Handler = handler
	r.triggerRunManager = triggerrun.NewManager(mgr.GetClient(), r.logger)
	r.pipelineRunManager = pipelinerun.NewManager(mgr.GetClient(), r.logger)
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.Pipeline{}).
		Complete(r)
}
