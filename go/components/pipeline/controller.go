// Package pipeline implements the Pipeline controller.
package pipeline

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"go.uber.org/zap"

	pbtypes "github.com/gogo/protobuf/types"
	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/revision"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	reconcileInterval  = 10 * time.Second
	pipelineAPIVersion = "michelangelo.api/v2"
	pipelineKind       = "Pipeline"
)

// Reconciler reconciles Pipeline resources.
type Reconciler struct {
	api.Handler
	env               env.Context
	logger            *zap.Logger
	apiHandlerFactory apiHandler.Factory
	revisionManager   revision.Manager
}

// Reconcile processes a Pipeline, updating its status and snapshotting a Revision.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger.With(zap.String("namespace-name", req.NamespacedName.String()))
	logger.Info("Reconciling pipeline starts")
	pipeline := &v2pb.Pipeline{}
	if err := r.Get(ctx, req.Namespace, req.Name, &metav1.GetOptions{}, pipeline); err != nil {
		// The API handler surfaces not-found as a gRPC status error, so use
		// utils.IsNotFoundError (handles both gRPC and k8s-typed errors) rather
		// than client.IgnoreNotFound (k8s-typed only). A deleted Pipeline is a
		// clean no-op.
		if utils.IsNotFoundError(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	// When the Pipeline is being deleted (a deletionTimestamp is set), stop
	// reconciling so we do not keep stamping status and requeueing while the
	// Kubernetes garbage collector and finalizers tear the object down.
	if !pipeline.GetDeletionTimestamp().IsZero() {
		logger.Info("Pipeline is being deleted; skipping reconcile")
		return ctrl.Result{}, nil
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

	// Snapshot the pipeline as a Revision on every READY reconcile.
	// UpsertRevision deduplicates immutable revisions, so this is safe to call
	// repeatedly. Status is already persisted above, so returning an error here
	// requeues only for the snapshot retry without affecting the pipeline's
	// READY state.
	if err == nil && pipeline.Status.State == v2pb.PIPELINE_STATE_READY {
		if snapshotErr := r.snapshotRevision(ctx, pipeline); snapshotErr != nil {
			logger.Error("failed to snapshot pipeline revision", zap.Error(snapshotErr))
			return result, snapshotErr
		}
	}

	return result, err
}

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

func formatRevisionName(pipeline *v2pb.Pipeline) string {
	if pipeline.Spec.Commit != nil {
		return fmt.Sprintf("%s-%s-%s", "pipeline", strings.ToLower(pipeline.Name), pipeline.Spec.Commit.GitRef[:min(len(pipeline.Spec.Commit.GitRef), 12)])
	}
	return ""
}

func isTerminatedState(state v2pb.PipelineState) bool {
	return state == v2pb.PIPELINE_STATE_READY ||
		state == v2pb.PIPELINE_STATE_ERROR
}

func (r *Reconciler) snapshotRevision(ctx context.Context, pipeline *v2pb.Pipeline) error {
	if pipeline.Spec.Commit == nil {
		r.logger.Info("skipping revision snapshot: pipeline has no commit info",
			zap.String("namespace", pipeline.Namespace),
			zap.String("name", pipeline.Name))
		return nil
	}

	content, err := pbtypes.MarshalAny(pipeline)
	if err != nil {
		return fmt.Errorf("marshal pipeline content: %w", err)
	}

	rev := &v2pb.Revision{
		TypeMeta: metav1.TypeMeta{
			APIVersion: pipelineAPIVersion,
			Kind:       "Revision",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        formatRevisionName(pipeline),
			Namespace:   pipeline.Namespace,
			Annotations: pipeline.Annotations,
		},
		Spec: v2pb.RevisionSpec{
			BaseType: &metav1.TypeMeta{
				Kind:       pipelineKind,
				APIVersion: pipelineAPIVersion,
			},
			BaseResource: &apipb.ResourceIdentifier{
				Name:      pipeline.Name,
				Namespace: pipeline.Namespace,
			},
			Content:    content,
			Owner:      pipeline.Spec.GetOwner(),
			RevisionId: pipeline.Spec.Commit.GitRef,
			Source:     revision.SourceGit,
			GitCommit:  pipeline.Spec.Commit,
		},
	}

	_, err = r.revisionManager.UpsertRevision(ctx, rev, revision.UpsertOpts{})
	return err
}

func (r *Reconciler) Register(mgr ctrl.Manager) error {
	handler, err := r.apiHandlerFactory.GetAPIHandler(mgr.GetClient())
	if err != nil {
		return err
	}
	r.Handler = handler
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.Pipeline{}).
		Complete(r)
}
