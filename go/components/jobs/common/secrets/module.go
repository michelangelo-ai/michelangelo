package secrets

import "go.uber.org/fx"

// Module provides the common objects
var Module = fx.Module("secrets",
	fx.Provide(New),
	fx.Provide(NewInClusterClientSet),
)
