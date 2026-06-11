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
// Default bindings:
//   - PhaseResolver: types.DefaultPhaseResolver (covers built-in pipeline types)
//   - Sinks: EmailSink and SlackSink
//
// Override either binding via fx.Decorate to extend without forking:
//
//	// Custom phase resolver
//	fx.Decorate(func() types.PhaseResolver { return myResolver })
//
//	// Additional notification channel
//	fx.Decorate(func() []Sink { return []Sink{&EmailSink{}, &SlackSink{}, &PagerDutySink{}} })
var Module = fx.Options(
	fx.Provide(providePhaseResolver),
	fx.Provide(provideDefaultSinks),
	fx.Provide(NewWorkflow),
	fx.Invoke(register),
)

// providePhaseResolver supplies the default PhaseResolver to FX.
func providePhaseResolver() types.PhaseResolver {
	return types.DefaultPhaseResolver
}

// provideDefaultSinks supplies the built-in email and Slack sinks.
// Override via fx.Decorate to add or replace channels.
func provideDefaultSinks() []Sink {
	return []Sink{&EmailSink{}, &SlackSink{}}
}

// register registers the notification workflow method with each worker instance.
func register(wf *Workflow, workers []worker.Worker) {
	for _, w := range workers {
		w.RegisterWorkflow(wf.SendPipelineRunNotification, types.PipelineRunNotificationWorkflowName)
	}
}
