package route

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var Module = fx.Options(
	fx.Provide(newRouteProvider),
)

// newRouteProvider creates a new route provider
func newRouteProvider(logger *zap.Logger) RouteProvider {
	return NewHTTPRouteManager(logger)
}
