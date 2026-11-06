package secrets

import (
	"go.uber.org/fx"

	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
)

// Module provides the common objects
var Module = fx.Module("secrets",
	fx.Provide(baseconfig.ProvideConfig[Config](configKey)),
	fx.Provide(New),
	fx.Provide(NewInClusterClientSet),
)
