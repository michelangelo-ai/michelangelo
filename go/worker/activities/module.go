package activities

import (
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/worker/activities/cachedoutput"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/model"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/storage"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/trigger"
)

// Module provides activity registrations for the shared worker binary.
//
// Notification activities are intentionally excluded — they are registered
// in cmd/worker/main.go so that downstream forks can supply their own
// transport implementations without conflicting with the defaults.
var Module = fx.Options(
	storage.Module,
	model.Module,
	cachedoutput.Module,
	trigger.Module,
)
