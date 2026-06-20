package workflows

import (
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/worker/workflows/ray"
	"github.com/michelangelo-ai/michelangelo/go/worker/workflows/trigger"
)

// Module provides workflow registrations for the shared worker binary.
var Module = fx.Options(
	ray.Module,
	trigger.Module,
)
