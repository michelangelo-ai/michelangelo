package revision

import (
	"context"
	"fmt"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Reconciler watches Revision CRs and dispatches to the Handler registered
// for each revision's Spec.BaseType.
type Reconciler struct {
	api.Handler
	apiHandlerFactory apiHandler.Factory
	logger            *zap.Logger
	handlers          map[metav1.TypeMeta]Handler
}

// NewReconciler constructs a Reconciler with the given handlers.
func NewReconciler(
	apiHandlerFactory apiHandler.Factory,
	logger *zap.Logger,
	handlers []Handler,
) *Reconciler {
	m := make(map[metav1.TypeMeta]Handler, len(handlers))
	for _, h := range handlers {
		m[h.TypeMeta()] = h
	}
	return &Reconciler{
		apiHandlerFactory: apiHandlerFactory,
		logger:            logger,
		handlers:          m,
	}
}

// Reconcile implements reconcile.Reconciler. Status is persisted only when
// the handler changes rev.Status.State, to avoid unnecessary writes.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger.With(zap.String("namespace-name", req.NamespacedName.String()))

	rev := &v2pb.Revision{}
	if err := r.Get(ctx, req.Namespace, req.Name, &metav1.GetOptions{}, rev); err != nil {
		if apiutils.IsNotFoundError(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !rev.GetDeletionTimestamp().IsZero() {
		logger.Info("Revision is being deleted; skipping reconcile")
		return ctrl.Result{}, nil
	}

	if rev.Spec.BaseType == nil {
		logger.Info("Revision has no BaseType; skipping reconcile")
		return ctrl.Result{}, nil
	}

	key := metav1.TypeMeta{
		APIVersion: rev.Spec.BaseType.APIVersion,
		Kind:       rev.Spec.BaseType.Kind,
	}
	h, ok := r.handlers[key]
	if !ok {
		logger.Info("no handler registered for BaseType; skipping reconcile",
			zap.String("apiVersion", key.APIVersion),
			zap.String("kind", key.Kind),
		)
		return ctrl.Result{}, nil
	}

	original := rev.DeepCopy()

	if err := h.Reconcile(ctx, rev); err != nil {
		return ctrl.Result{}, fmt.Errorf("handler reconcile for %s/%s: %w", key.APIVersion, key.Kind, err)
	}

	if rev.Status.State != original.Status.State {
		if err := r.UpdateStatus(ctx, rev, &metav1.UpdateOptions{}); err != nil {
			return ctrl.Result{}, fmt.Errorf("update revision status %s/%s: %w", req.Namespace, req.Name, err)
		}
	}

	return ctrl.Result{}, nil
}

// Register sets up the Revision controller with the controller-runtime manager.
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
