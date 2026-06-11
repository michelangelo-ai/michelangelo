// Package notification provides the pipeline run notification workflow.
package notification

import (
	"errors"
	"time"

	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	"go.uber.org/zap"
)

var workflowActivityOpts = workflow.ActivityOptions{
	ScheduleToStartTimeout: 1 * time.Minute,
	StartToCloseTimeout:    30 * time.Minute,
	HeartbeatTimeout:       1 * time.Minute,
}

// Workflow holds workflow-level dependencies injected at worker registration time.
//
// Keeping these as struct fields — rather than embedding them in the serialized
// request — allows non-serializable values (functions, interfaces) to be injected
// via FX without modifying the Cadence/Temporal workflow input schema.
type Workflow struct {
	phaseResolver types.PhaseResolver
	sinks         []Sink
}

// NewWorkflow creates a Workflow with the given PhaseResolver and notification sinks.
//
// Pass nil for phaseResolver to use DefaultPhaseResolver, which covers the
// built-in pipeline types. Operators with custom pipeline types should supply
// their own resolver via FX:
//
//	fx.Decorate(func() types.PhaseResolver { return myCustomResolver })
//
// Pass a non-empty sinks slice to override the default email and Slack sinks.
// Add new sinks (e.g. PagerDuty, webhook) without modifying this workflow:
//
//	fx.Decorate(func() []Sink { return []Sink{&EmailSink{}, &PagerDutySink{}} })
func NewWorkflow(phaseResolver types.PhaseResolver, sinks []Sink) *Workflow {
	if phaseResolver == nil {
		phaseResolver = types.DefaultPhaseResolver
	}
	return &Workflow{
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
	ctx = workflow.WithActivityOptions(ctx, workflowActivityOpts)
	logger := workflow.GetLogger(ctx)

	pipelineRun := req.PipelineRun
	var errs error

	for _, notif := range pipelineRun.Spec.Notifications {
		if !types.ContainsEventType(notif.EventTypes, pipelineRun.Status.State) {
			continue
		}

		msg := Message{
			Subject:   types.GenerateSubject(pipelineRun),
			EmailText: types.GenerateText(pipelineRun, types.NotificationTypeEmail, req.StudioBaseURL, wf.phaseResolver),
			SlackText: types.GenerateText(pipelineRun, types.NotificationTypeSlack, req.StudioBaseURL, wf.phaseResolver),
			SendAs:    req.SenderEmail,
		}

		for _, sink := range wf.sinks {
			if err := sink.Notify(ctx, notif, msg); err != nil {
				logger.Error("Notification sink failed", zap.Error(err))
				errs = errors.Join(errs, err)
			}
		}
	}

	return errs
}
