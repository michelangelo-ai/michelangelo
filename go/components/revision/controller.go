package revision

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const _requestTimeoutSec = 30

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

// Reconcile implements reconcile.Reconciler. Status is persisted when the
// handler mutates any field in rev.Status, even if the handler returns an error.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, _requestTimeoutSec*time.Second)
	defer cancel()

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

	if apiutils.IsImmutable(rev) {
		return ctrl.Result{}, nil
	}

	if rev.Status.State == v2pb.REVISION_STATE_READY || rev.Status.State == v2pb.REVISION_STATE_ERROR {
		apiutils.MarkImmutable(rev)
		if err := r.Update(ctx, rev, &metav1.UpdateOptions{}); err != nil {
			return ctrl.Result{}, fmt.Errorf("mark revision immutable %s/%s: %w", req.Namespace, req.Name, err)
		}
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

	result, handlerErr := h.Reconcile(ctx, rev)
	if handlerErr != nil {
		handlerErr = fmt.Errorf("handler reconcile for %s/%s: %w", key.APIVersion, key.Kind, handlerErr)
	}

	var updateErr error
	if !reflect.DeepEqual(original.Status, rev.Status) {
		if err := r.UpdateStatus(ctx, rev, &metav1.UpdateOptions{}); err != nil {
			updateErr = fmt.Errorf("update revision status %s/%s: %w", req.Namespace, req.Name, err)
		}
	}

	return result, errors.Join(handlerErr, updateErr)
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
		WithEventFilter(
			predicate.NewPredicateFuncs(func(object client.Object) bool {
				rev, ok := object.(*v2pb.Revision)
				if !ok {
					return false
				}
				baseType := rev.Spec.GetBaseType()
				if baseType == nil {
					return false
				}
				_, exists := r.handlers[*baseType]
				return exists
			}),
		).
		Complete(r)
}
