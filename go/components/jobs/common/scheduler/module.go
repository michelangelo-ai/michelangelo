package scheduler

import "go.uber.org/fx"

// Module provides scheduler queue
var Module = fx.Provide(New)
