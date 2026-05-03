package routes

import (
	"go.uber.org/fx"
)

// Module provides the RouteProvider singleton.
var Module = fx.Options(
	fx.Provide(NewDefaultRouteProvider),
)
