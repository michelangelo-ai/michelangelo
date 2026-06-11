// Package notification registers the pipeline run notification workflow with
// the Cadence/Temporal worker.
package notification

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	"go.uber.org/fx"
)

// Module provides FX dependency injection for the notification workflow.
//
// By default the built-in DefaultPhaseResolver is used to build deep links in
// notification bodies. Operators with custom pipeline types can override it:
//
//	fx.Decorate(func() types.PhaseResolver { return myCustomResolver })
var Module = fx.Options(
	fx.Provide(providePhaseResolver),
	fx.Provide(NewWorkflow),
	fx.Invoke(register),
)

// providePhaseResolver supplies the default PhaseResolver to FX.
// Override this binding to support custom pipeline type labels.
func providePhaseResolver() types.PhaseResolver {
	return types.DefaultPhaseResolver
}

// register registers the notification workflow with each worker instance.
func register(wf *Workflow, workers []worker.Worker) {
	for _, w := range workers {
		w.RegisterWorkflow(wf.SendPipelineRunNotification, types.PipelineRunNotificationWorkflowName)
	}
}

