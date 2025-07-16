package deployment

import (
	"github.com/go-logr/zapr"
	"go.uber.org/fx"
	"go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
)

// Module provides the deployment controller with all dependencies
var Module = fx.Module("deployment",
	fx.Provide(NewReconciler),
	fx.Invoke(register),
)

// NewReconciler creates a new deployment reconciler with injected dependencies
func NewReconciler(client ctrl.Manager, logger *zap.Logger, gateway gateways.Gateway) *Reconciler {
	log := zapr.NewLogger(logger).WithName("deployment")
	plugin := oss.NewPlugin(client.GetClient(), gateway, log)
	
	return &Reconciler{
		Client: client.GetClient(),
		Log:    log,
		Plugin: plugin,
	}
}

// register sets up the deployment controller with the manager
func register(mgr ctrl.Manager, reconciler *Reconciler) error {
	return reconciler.SetupWithManager(mgr)
}
