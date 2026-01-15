package notification

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/fx"
)

// Module defines the dependency injection options for the fx framework.
// It registers the notification workflows with the workers.
var Module = fx.Options(
	fx.Invoke(register), // Invokes the register function to register workflows with the workers.
)

// register registers the notification workflows to the provided workers.
//
// Params:
// - workers ([]worker.Worker): A list of Cadence workers where workflows will be registered.
func register(workers []worker.Worker) {
	// Initialize the workflows struct.
	w := &workflows{}

	// Register the workflows with each worker.
	for _, worker := range workers {
		worker.RegisterWorkflow(w.PipelineRunNotificationWorkflow, PipelineRunNotificationWorkflowName)
	}
}