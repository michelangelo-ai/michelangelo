package inferenceserver

import (
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/provider/serving"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/provider/tritoninferenceserver"
)

var (
	// Module FX
	Module = fx.Options(
		tritoninferenceserver.Module,
		fx.Invoke(register),
	)
)

func register(
	env env.Context,
	mgr manager.Manager,
	servingProvider serving.Provider,
) error {
	return (&Reconciler{
		Client:          mgr.GetClient(),
		servingProvider: servingProvider,
		env:             env,
	}).Register(mgr)
}
