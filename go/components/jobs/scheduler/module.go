package scheduler

import (
	commonscheduler "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/scheduler"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler/framework"
	"go.uber.org/fx"
)

// Module provides scheduler
var Module = fx.Options(
	fx.Provide(
		fx.Annotate(
			NewScheduler,
			fx.As(new(JobQueue)),
		),
	),
	commonscheduler.Module,
	framework.Module,
)
