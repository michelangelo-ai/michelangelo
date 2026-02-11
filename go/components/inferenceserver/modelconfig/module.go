package modelconfig

import (
	"go.uber.org/fx"
)

// Module provides the modelconfig module
var Module = fx.Options(
	fx.Provide(newModelConfigProvider),
)

// newModelConfigProvider creates a new model config provider
func newModelConfigProvider() ModelConfigProvider {
	return NewDefaultModelConfigProvider()
}
