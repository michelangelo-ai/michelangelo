package deployment

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/zapr"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/pluginmanager"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/proxy"
)

// Module FX
var Module = fx.Options(
	fx.Provide(newPluginRegistry),
	fx.Provide(newProxyProvider),
	fx.Invoke(register),
	oss.Module,
)

func register(
	mgr manager.Manager,
	apiHandlerFactory apiHandler.Factory,
	registrar pluginmanager.Registrar[plugins.Plugin],
) error {
	return NewReconciler(apiHandlerFactory, registrar).SetupWithManager(mgr)
}

// newPluginRegistry creates a new plugin registry
func newPluginRegistry() pluginmanager.Registrar[plugins.Plugin] {
	return pluginmanager.NewSimpleRegistrar[plugins.Plugin](zapr.NewLogger(zap.NewNop()))
}

// newProxyProvider creates a new proxy provider
func newProxyProvider(dynamicClient dynamic.Interface, logger *zap.Logger) proxy.ProxyProvider {
	return proxy.NewHTTPRouteManager(dynamicClient, logger)
}
