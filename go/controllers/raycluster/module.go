package raycluster

import (
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	// Module FX
	Module = fx.Options(
		fx.Provide(newConfig),
		fx.Invoke(register),
	)
)

func register(
	conf Config,
	mgr manager.Manager,
) error {
	return (&Controller{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
	}).Register(mgr)
}
