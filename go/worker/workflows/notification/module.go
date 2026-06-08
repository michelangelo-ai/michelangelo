// Package notification registers the pipeline run notification workflow with
// the Cadence/Temporal worker.
package notification

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	"go.uber.org/fx"
)

// Module provides FX dependency injection for the notification workflow.
var Module = fx.Options(
	fx.Invoke(register),
)

// register registers the notification workflow with each worker instance.
func register(workers []worker.Worker) {
	for _, w := range workers {
		w.RegisterWorkflow(SendPipelineRunNotification, types.PipelineRunNotificationWorkflowName)
	}
}
