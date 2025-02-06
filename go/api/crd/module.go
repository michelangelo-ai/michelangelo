package crd

import (
	"go.uber.org/fx"
)

const moduleName = "crd_manager"

var Module = fx.Options(
	fx.Provide(
		ParseConfig,
		NewCRDGateway,
	),
)
