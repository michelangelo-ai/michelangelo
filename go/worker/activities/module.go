package activities

import (
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/s3"
	"go.uber.org/fx"
)

var Module = fx.Options(
	ray.Module,
	s3.Module,
)
