package workflows

import (
	"github.com/michelangelo-ai/michelangelo/go/worker/workflows/ray"
	"go.uber.org/fx"
)

var Module = fx.Options(
	ray.Module,
)
