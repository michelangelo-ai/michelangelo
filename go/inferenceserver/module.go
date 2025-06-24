package inferenceserver

import (
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/provider/serving"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/provider/llmd"
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/provider/tritoninferenceserver"
)

var (
	// Module FX
	Module = fx.Options(
		tritoninferenceserver.Module,
		llmd.Module,
		fx.Invoke(register),
	)
)

// ProviderParams contains all the providers
type ProviderParams struct {
	fx.In
	TritonProvider serving.Provider `name:"triton"`
	LLMDProvider   serving.Provider `name:"llmd"`
}

func register(
	env env.Context,
	mgr manager.Manager,
	providers ProviderParams,
) error {
	return (&Reconciler{
		Client:         mgr.GetClient(),
		tritonProvider: providers.TritonProvider,
		llmdProvider:   providers.LLMDProvider,
		env:            env,
	}).Register(mgr)
}
