package scheduler

import (
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/scheduler"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/scheduler/framework"
	"go.uber.org/fx"
)

// Module provides scheduler
var Module = fx.Options(
	fx.Provide(
		fx.Annotate(
			NewController,
			fx.As(new(JobQueue)),
			fx.As(new(ResourcePoolSelector)),
		),
	),
	scheduler.Module,
	framework.Module,
)
