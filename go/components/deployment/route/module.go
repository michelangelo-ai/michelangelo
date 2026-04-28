package route

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
)

var Module = fx.Options(
	fx.Provide(newRouteProvider),
)

// newRouteProvider creates a new route provider
func newRouteProvider(dynamicClient dynamic.Interface, logger *zap.Logger) RouteProvider {
	return NewHTTPRouteManager(dynamicClient, logger)
}
