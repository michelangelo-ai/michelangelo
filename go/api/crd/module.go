package crd

import (
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Provide(
		ParseConfig,
		NewCRDGateway,
	),
)
