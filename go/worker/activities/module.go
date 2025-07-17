package activities

import (
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/cachedoutput"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/rayhttp"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/spark"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/storage"
	"go.uber.org/fx"
)

var Module = fx.Options(
	ray.Module,
	rayhttp.Module,
	spark.Module,
	storage.Module,
	cachedoutput.Module,
)
