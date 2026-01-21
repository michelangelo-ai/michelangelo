package activities

import (
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/worker/activities/cachedoutput"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/model"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/notification"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/spark"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/storage"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/trigger"
)

var Module = fx.Options(
	ray.Module,
	spark.Module,
	storage.Module,
	model.Module,
	cachedoutput.Module,
	trigger.Module,
	notification.Module,
)
