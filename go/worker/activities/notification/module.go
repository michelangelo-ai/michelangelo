package notification

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	maSlack "github.com/michelangelo-ai/michelangelo/go/base/notification/slack"
	"go.uber.org/fx"
)

// Module provides FX dependency injection for notification activities.
//
// This module registers the notification activities with all workers.
// The Slack client is injected from go/base/notification/slack.
var Module = fx.Options(
	maSlack.Module,
	fx.Provide(NewActivities),
	fx.Invoke(register),
)

func register(activities *Activities, workers []worker.Worker) {
	for _, w := range workers {
		w.RegisterActivity(activities)
	}
}