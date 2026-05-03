package discovery

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
)

var Module = fx.Options(
	fx.Provide(newModelDiscoveryProvider),
)

func newModelDiscoveryProvider(dynamicClient dynamic.Interface, logger *zap.Logger) ModelDiscoveryProvider {
	return NewHTTPRouteManager(dynamicClient, logger)
}
