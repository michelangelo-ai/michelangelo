package cluster

import (
	"go.uber.org/fx"
)

// Module provides the cluster reconciler.
var Module = fx.Options(
	fx.Provide(NewReconciler),
	fx.Provide(NewResourcePoolCache),
	fx.Provide(NewSkuConfigCache),
)
