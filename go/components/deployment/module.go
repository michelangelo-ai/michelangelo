package deployment

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/zapr"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/pluginmanager"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
)

// Module FX
var Module = fx.Options(
	fx.Provide(newPluginRegistry),
	fx.Invoke(register),
)

// newPluginRegistry creates a new plugin registry
func newPluginRegistry() pluginmanager.Registrar[plugins.Plugin] {
	return pluginmanager.NewSimpleRegistrar[plugins.Plugin](zapr.NewLogger(zap.NewNop()))
}

func register(
	mgr manager.Manager,
	apiHandlerFactory apiHandler.Factory,
	registrar pluginmanager.Registrar[plugins.Plugin],
) error {
	return NewReconciler(apiHandlerFactory, registrar).SetupWithManager(mgr)
}
