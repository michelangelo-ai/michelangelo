package kueue

import "go.uber.org/fx"

// Module provides the Kueue backend's supporting pieces. KueueJobQueue itself
// is constructed by the scheduler module's backend factory (which owns the
// default-vs-kueue selection), so only the LocalQueues checker is provided
// here.
var Module = fx.Options(
	fx.Provide(NewLocalQueues),
)
