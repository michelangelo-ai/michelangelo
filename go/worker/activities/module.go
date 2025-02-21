package activities

import (
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	"go.uber.org/fx"
)

var Module = fx.Options(
	ray.Module,
)
