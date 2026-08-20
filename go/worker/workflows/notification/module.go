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

// SinkConfig holds configuration for notification sinks.
type SinkConfig struct {
	// StudioBaseURL is the base URL for the Studio console (e.g. https://studio.example.com).
	// Used by SlackSink to generate Block Kit "View Details" buttons.
	StudioBaseURL string
}

// provideDefaultSinks supplies the built-in email and Slack sinks.
// Override via fx.Decorate to add or replace channels.
//
// To configure the StudioBaseURL for Block Kit links, provide a SinkConfig to FX:
//
//	fx.Provide(func() notification.SinkConfig {
//	    return notification.SinkConfig{StudioBaseURL: "https://studio.example.com"}
//	})
func provideDefaultSinks(config ...SinkConfig) []Sink {
	var cfg SinkConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	return []Sink{&EmailSink{}, &SlackSink{StudioBaseURL: cfg.StudioBaseURL}}
}

// register registers the notification workflow method with each worker instance.
// The deprecated name "PRNotificationWorkflow" is registered as an alias so that
// in-flight executions dispatched by a pre-upgrade controllermgr can drain
// without hanging. Remove the alias registration after all operators have rolled
// past this release.
//
// Note: Notification activities are registered separately in
// go/worker/activities/notification/module.go to allow operators to inject
// custom transport implementations via FX.
func register(wf *Workflow, workers []worker.Worker) {
	for _, w := range workers {
		w.RegisterWorkflow(wf.SendPipelineRunNotification, types.PipelineRunNotificationWorkflowName)
		w.RegisterWorkflow(wf.SendPipelineRunNotification, types.DeprecatedPRNotificationWorkflowName)
	}
}
