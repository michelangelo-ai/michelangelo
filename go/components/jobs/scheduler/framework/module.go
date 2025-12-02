package framework

import (
	"go.uber.org/fx"
)

// Module provides the framework for the scheduler.
var Module = fx.Options(
	fx.Provide(NewClusterOnlyAssignmentStrategy),
)
