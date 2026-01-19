package notification

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/fx"
)

// Module provides FX dependency injection for notification workflow and activities.
var Module = fx.Options(
	fx.Invoke(register),
)

// register registers notification workflows with the workers.
//
// This function registers all notification-related workflows with the Cadence worker
// instances, making them available for execution when workflow clients start them.
// Activities are registered separately via the activities module.
//
// Params:
// - workers: Array of worker instances to register workflows with.
//
// Registered Components:
// - SendPRNotification workflow: Processes pipeline run notifications
func register(workers []worker.Worker) {
	for _, w := range workers {
		// Register workflow
		w.RegisterWorkflow(SendPRNotification, PRNotificationWorkflowName)
	}
}
