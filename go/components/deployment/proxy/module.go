package proxy

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
)

var Module = fx.Options(
	fx.Provide(newProxyProvider),
)

// newProxyProvider creates a new proxy provider
func newProxyProvider(dynamicClient dynamic.Interface, logger *zap.Logger) ProxyProvider {
	return NewHTTPRouteManager(dynamicClient, logger)
}
