package kuberay

import (
	"go.uber.org/fx"
)

// Module provide rest client for kuberay types
var Module = fx.Module("kuberay",
	fx.Provide(NewRestClient),
)
