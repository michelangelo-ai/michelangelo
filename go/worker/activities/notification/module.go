package notification

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/fx"
)

// Module provides FX dependency injection for notification activities.
var Module = fx.Options(
	fx.Invoke(register),
)

// register registers notification activities with the workers.
//
// This function registers all notification-related activities with the Cadence worker
// instances, making them available for execution when workflow clients start them.
//
// Params:
// - workers: Array of worker instances to register activities with.
//
// Registered Components:
// - SendMessageToEmailActivity: Sends email notifications
// - SendMessageToSlackActivity: Sends Slack notifications
func register(workers []worker.Worker) {
	for _, w := range workers {
		w.RegisterActivity(SendMessageToEmailActivity)
		w.RegisterActivity(SendMessageToSlackActivity)
	}
}