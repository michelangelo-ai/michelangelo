package cluster

import (
	"go.uber.org/fx"
)

// Module provides the cluster reconciler and scheduler.
var Module = fx.Options(
	fx.Provide(NewReconciler),
)
