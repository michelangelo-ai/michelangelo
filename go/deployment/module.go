package deployment

import (
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/deployment/provider"
	"github.com/michelangelo-ai/michelangelo/go/deployment/provider/kserve"
)

var (
	// Module FX
	Module = fx.Options(
		kserve.Module,
		fx.Invoke(register),
	)
)

func register(
	env env.Context,
	mgr manager.Manager,
	provider provider.Provider,
) error {
	//restConfig := mgr.GetConfig()
	// Create SparkApplication client
	return (&Reconciler{
		Client:          mgr.GetClient(),
		servingProvider: provider,
		env:             env,
	}).Register(mgr)
}
