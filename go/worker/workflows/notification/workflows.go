// Package notification provides the pipeline run notification workflow.
package notification

import (
	"errors"
	"time"

	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.uber.org/zap"
)

var workflowActivityOpts = workflow.ActivityOptions{
	ScheduleToStartTimeout: 1 * time.Minute,
	StartToCloseTimeout:    30 * time.Minute,
	HeartbeatTimeout:       1 * time.Minute,
	RetryPolicy: &workflow.RetryPolicy{
		InitialInterval:    1 * time.Second,
		BackoffCoefficient: 2.0,
		MaximumInterval:    30 * time.Second,
		MaximumAttempts:    3,
	},
}

// Workflow holds workflow-level dependencies injected at worker registration time.
//
// Keeping these as struct fields — rather than embedding them in the serialized
// request — allows non-serializable values (functions, interfaces) to be injected
// via FX without modifying the Cadence/Temporal workflow input schema.
type Workflow struct {
	backend       workflow.Workflow
	phaseResolver types.PhaseResolver
	sinks         []Sink
}

// NewWorkflow creates a Workflow with the given backend, PhaseResolver and notification sinks.
//
// backend is the starlark-worker Workflow backend provided by workflowfx. It is
// attached to the workflow context via workflow.WithBackend so that
// workflow.ExecuteActivity resolves correctly. Pass nil only in unit tests that
// do not invoke activities through a real workflow context.
//
// Pass nil for phaseResolver to use DefaultPhaseResolver, which covers the
// built-in pipeline types. Operators with custom pipeline types should supply
// their own resolver via FX:
//
//	fx.Decorate(func() types.PhaseResolver { return myCustomResolver })
//
// Pass a non-empty sinks slice to override the default email and Slack sinks.
// Add new sinks (e.g. PagerDuty, SMS) without modifying this workflow:
//
//	fx.Decorate(func() []Sink { return []Sink{&EmailSink{}, &PagerDutySink{}} })
func NewWorkflow(backend workflow.Workflow, phaseResolver types.PhaseResolver, sinks []Sink) *Workflow {
	if phaseResolver == nil {
		phaseResolver = types.DefaultPhaseResolver
	}
	return &Workflow{
		backend:       backend,
		phaseResolver: phaseResolver,
		sinks:         sinks,
	}
}

// SendPipelineRunNotification fans out notifications for a pipeline run state
// change to all registered sinks.
//
// Each configured notification is matched against the current pipeline run state;
// only matching notifications are delivered. Delivery failures are accumulated
// with errors.Join so that a failure on one sink does not suppress others.
func (wf *Workflow) SendPipelineRunNotification(ctx workflow.Context, req *types.PipelineRunNotificationRequest) error {
	if req == nil || req.PipelineRun == nil {
		return errors.New("notification request or pipeline run is nil")
	}

	// Attach the workflow backend so that workflow.ExecuteActivity can resolve
	// activities. This mirrors the pattern used by other Go workflows in this
	// codebase (e.g. trigger.CronTrigger).
	if wf.backend != nil {
		ctx = workflow.WithBackend(ctx, wf.backend)
	}
	logger := workflow.GetLogger(ctx)

	pipelineRun := req.PipelineRun
	var errs error

	msg := Message{
		Subject: types.GenerateSubject(pipelineRun),
		Body:    types.GenerateBody(pipelineRun, req.StudioBaseURL, wf.phaseResolver),
		FormattedBodies: map[string]string{
			FormatSlackMrkdwn: types.GenerateText(pipelineRun, v2pb.NOTIFICATION_TYPE_SLACK, req.StudioBaseURL, wf.phaseResolver),
		},
		SendAs: req.SenderEmail,
	}

	for _, notif := range pipelineRun.Spec.Notifications {
		if !types.ContainsEventType(notif.EventTypes, pipelineRun.Status.State) {
			continue
		}

		for _, sink := range wf.sinks {
			if err := sink.Notify(ctx, logger, notif, msg); err != nil {
				if logger != nil {
					logger.Error("Notification sink failed", zap.Error(err))
				}
				errs = errors.Join(errs, err)
			}
		}
	}

	return errs
}
