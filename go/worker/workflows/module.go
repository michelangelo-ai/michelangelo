package workflows

import (
	"github.com/michelangelo-ai/michelangelo/go/worker/workflows/notification"
	"github.com/michelangelo-ai/michelangelo/go/worker/workflows/ray"
	"github.com/michelangelo-ai/michelangelo/go/worker/workflows/trigger"
	"go.uber.org/fx"
)

var Module = fx.Options(
	ray.Module,
	trigger.Module,
	notification.Module,
)
