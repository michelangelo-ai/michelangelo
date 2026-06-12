package activities

import (
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/worker/activities/cachedoutput"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/model"
	notificationActivities "github.com/michelangelo-ai/michelangelo/go/worker/activities/notification"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/storage"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/trigger"
)

// Module provides activity registrations for the shared worker binary.
var Module = fx.Options(
	storage.Module,
	model.Module,
	cachedoutput.Module,
	trigger.Module,
	notificationActivities.Module,
)
