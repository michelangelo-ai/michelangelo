package activities

import (
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/worker/activities/cachedoutput"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/model"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/storage"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/trigger"
)

// Module provides activity registrations for the shared worker binary.
// Notification activities are registered separately by the notification-worker
// binary (go/cmd/notification-worker).
var Module = fx.Options(
	storage.Module,
	model.Module,
	cachedoutput.Module,
	trigger.Module,
)
