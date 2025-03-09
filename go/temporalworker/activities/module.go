package activities

import (
	"github.com/michelangelo-ai/michelangelo/go/temporalworker/activities/ray"
	"github.com/michelangelo-ai/michelangelo/go/temporalworker/activities/storage"
	"go.uber.org/fx"
)

var Module = fx.Options(
	ray.Module,
	storage.Module,
)
