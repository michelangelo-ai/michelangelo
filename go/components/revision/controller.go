package revision

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
	logger.Info("Reconciling revision starts")
	
	revision := &v2pb.Revision{}
	if err := r.Get(ctx, req.Namespace, req.Name, &metav1.GetOptions{}, revision); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	
	originalRevision := revision.DeepCopy()
	state := revision.Status.State
	logger.Info("Reconciling revision", zap.Any("RevisionStatusState", state.String()))
	
	// Process revision state machine
	switch state {
	case v2pb.REVISION_STATE_INVALID, v2pb.REVISION_STATE_CREATED:
		logger.Info("Revision transitioning to BUILDING state")
		revision.Status.State = v2pb.REVISION_STATE_BUILDING
		
	case v2pb.REVISION_STATE_BUILDING:
		// Simulate build process - in real implementation, this would check actual build status
		logger.Info("Revision build completed, transitioning to READY state")
		revision.Status.State = v2pb.REVISION_STATE_READY
		
	case v2pb.REVISION_STATE_READY, v2pb.REVISION_STATE_ERROR:
		// Terminal states - no further processing needed
		logger.Info("Revision in terminal state", zap.Any("state", state.String()))
	}
	
	return r.updateRevisionStatus(ctx, revision, originalRevision, logger)
}

func (r *Reconciler) updateRevisionStatus(ctx context.Context, revision *v2pb.Revision, originalRevision *v2pb.Revision, logger *zap.Logger) (ctrl.Result, error) {
	result := ctrl.Result{}
	if !isTerminatedState(revision.Status.State) {
		result = ctrl.Result{RequeueAfter: reconcileInterval}
	}
	
	if !reflect.DeepEqual(originalRevision.Status, revision.Status) {
		logger.Info("Revision status updated", zap.Any("RevisionStatusState", revision.Status.State.String()))
		err := r.UpdateStatus(ctx, revision, &metav1.UpdateOptions{})
		if err != nil {
			logger.Error("Failed to update revision status", zap.Error(err))
			return result, err
		}
	}

	return result, nil
}

func isTerminatedState(state v2pb.RevisionState) bool {
	return state == v2pb.REVISION_STATE_READY ||
		state == v2pb.REVISION_STATE_ERROR
}

// Register is used to register the controller with the manager.
func (r *Reconciler) Register(mgr ctrl.Manager) error {
	handler, err := r.apiHandlerFactory.GetAPIHandler(mgr.GetClient())
	if err != nil {
		return err
	}
	r.Handler = handler
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.Revision{}).
		Complete(r)
}