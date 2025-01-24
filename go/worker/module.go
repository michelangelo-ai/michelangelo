package worker

import (
	"github.com/michelangelo-ai/michelangelo/go/worker/activities"
	"github.com/michelangelo-ai/michelangelo/go/worker/workflows"
	"go.uber.org/fx"
)

var Module = fx.Options(
	activities.Module,
	workflows.Module,
)
