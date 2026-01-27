package configmap

import (
	"go.uber.org/fx"
)

// Module provides the configmap module
var Module = fx.Options(
	fx.Provide(NewDefaultModelConfigMapProvider),
)
