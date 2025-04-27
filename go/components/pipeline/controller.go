package pipeline

import (
	"context"
	"reflect"
	"time"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	reconcileInterval = 10 * time.Second
)

// Reconciler is the output of NewReconciler.
type Reconciler struct {
	api.Handler
	env               env.Context
	logger            *zap.Logger
	apiHandlerFactory apiHandler.Factory
}

// Reconcile is the main entrypoint for the controller.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger.With(zap.String("namespace-name", req.NamespacedName.String()))
	logger.Info("Reconciling pipeline starts")
	pipeline := &v2pb.Pipeline{}
	if err := r.Get(ctx, req.Namespace, req.Name, &metav1.GetOptions{}, pipeline); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	originalPipeline := pipeline.DeepCopy()
	state := pipeline.Status.State
	logger.Info("Reconciling pipeline", zap.Any("PipelineStatusState", state.String()))
	// TODO: add logic to handle different states
	switch state {
	case v2pb.PIPELINE_STATE_INVALID:
		pipeline.Status.State = v2pb.PIPELINE_STATE_CREATED
	case v2pb.PIPELINE_STATE_CREATED:
		pipeline.Status.State = v2pb.PIPELINE_STATE_READY
		pipeline.Status.Commit = &v2pb.CommitInfo{
			GitRef: pipeline.Spec.Commit.GitRef,
			Branch: pipeline.Spec.Commit.Branch,
		}
	case v2pb.PIPELINE_STATE_READY, v2pb.PIPELINE_STATE_ERROR:
		if shouldUpdateStatus(pipeline) {
			pipeline.Status.State = v2pb.PIPELINE_STATE_INVALID
		}
	}
	return r.updatePipelineStatus(ctx, pipeline, originalPipeline, logger)
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
			logger.Error("Failed to update pipeline status", zap.Error(err))
			return result, err
		}
	}

	return result, nil
}

func isTerminatedState(state v2pb.PipelineState) bool {
	return state == v2pb.PIPELINE_STATE_READY ||
		state == v2pb.PIPELINE_STATE_ERROR
}

func shouldUpdateStatus(pipeline *v2pb.Pipeline) bool {
	return !reflect.DeepEqual(pipeline.Spec.Commit, pipeline.Status.Commit)
}

// Register is used to register the controller with the manager.
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
