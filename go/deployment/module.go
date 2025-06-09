package deployment

import (
	"github.com/michelangelo-ai/michelangelo/go/deployment/provider/proxy"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/deployment/provider/istio"
)

var (
	// Module FX
	Module = fx.Options(
		istio.Module,
		fx.Invoke(register),
	)
)

func register(
	env env.Context,
	mgr manager.Manager,
	proxyProvider proxy.ProxyProvider,
) error {
	return (&Reconciler{
		Client:        mgr.GetClient(),
		proxyProvider: proxyProvider,
		env:           env,
	}).Register(mgr)
}
