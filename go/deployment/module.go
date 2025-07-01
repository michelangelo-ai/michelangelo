package deployment

import (
	"go.uber.org/fx"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
)

// Module provides the deployment controller with all dependencies
var Module = fx.Module("deployment",
	fx.Provide(NewGateway),
	fx.Provide(NewReconciler),
	fx.Invoke(register),
)

// NewGateway creates a new inference server gateway
func NewGateway() inferenceserver.Gateway {
	return inferenceserver.NewGateway()
}

// register sets up the deployment controller with the manager
func register(mgr ctrl.Manager, reconciler *Reconciler) error {
	return reconciler.SetupWithManager(mgr)
}
