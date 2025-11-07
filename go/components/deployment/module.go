package deployment

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/logr"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/pluginmanager"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
)

// Module FX
var Module = fx.Options(
	fx.Provide(func() pluginmanager.Registrar[plugins.Plugin] {
		return pluginmanager.NewSimpleRegistrar[plugins.Plugin](logr.Discard())
	}),
	fx.Invoke(register),
	oss.Module,
	fx.Provide(gateways.NewConfigMapProvider),
)

func register(
	mgr manager.Manager,
	env env.Context,
	apiHandlerFactory apiHandler.Factory,
	logger *zap.Logger,
	registrar pluginmanager.Registrar[plugins.Plugin],
) error {
	return NewReconciler(apiHandlerFactory, registrar).SetupWithManager(mgr)
}
