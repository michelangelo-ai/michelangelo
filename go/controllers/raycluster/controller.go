package raycluster

import (
	"context"
	"time"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type (
	Controller struct {
		client.Client
		Scheme          *runtime.Scheme
	}
)

func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconcile")

	var rayCluster v2pb.RayCluster
	if err := r.Get(ctx, req.NamespacedName, &rayCluster); err == nil {
		// Resource exists.
		// Reconcile: Create cadence workflow execution (if not exists)
		return r.reconcile(ctx, req, rayCluster)

	} else {
		// Unexpected network IO error.
		// Reconcile: Retry
		return ctrl.Result{
			RequeueAfter: time.Second * 20,
		}, err
	}
}

func (r *Controller) Register(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.RayCluster{}).
		Complete(r)
}

func (r *Controller) reconcile(
	ctx context.Context,
	req ctrl.Request,
	rayCluster v2pb.RayCluster,
) (
	ctrl.Result,
	error,
) {

	return ctrl.Result{}, nil
}
