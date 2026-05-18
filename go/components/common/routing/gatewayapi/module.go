package gatewayapi

import "go.uber.org/fx"

// Module provides the Gateway API HTTPRoute implementation of routing.Manager.
var Module = fx.Options(
	fx.Provide(New),
)
