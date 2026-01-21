package notification

import (
	"time"

	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	notificationActivities "github.com/michelangelo-ai/michelangelo/go/worker/activities/notification"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
)

// Notification Workflow name
const (
	PRNotificationWorkflowName = "PRNotificationWorkflow"
)

var (
	workflowActivityOpts = workflow.ActivityOptions{
		ScheduleToStartTimeout: 1 * time.Minute,
		StartToCloseTimeout:    30 * time.Minute,
		HeartbeatTimeout:       1 * time.Minute,
	}
)

// sendSlackNotification sends a slack notification through workflow activity execution.
//
// This method executes the SendMessageToSlackActivity as part of a notification workflow.
//
// Params:
// - ctx: The workflow context for the operation.
// - channel: The Slack channel to send the message to.
// - text: The message content to send.
//
// Returns:
// - error: Error information if the activity execution fails.
func sendSlackNotification(ctx workflow.Context, channel, text string) error {
	logger := workflow.GetLogger(ctx)
	ao := workflowActivityOpts
	if err := workflow.ExecuteActivity(
		workflow.WithActivityOptions(ctx, ao),
		notificationActivities.SendMessageToSlackActivity,
		&notificationActivities.SendMessageToSlackActivityRequest{
			Channel: channel,
			Text:    text,
		}).Get(ctx, nil); err != nil {
		logger.Error("The slack message failed to send with", zap.Error(err))
		return err
	}
	logger.Info("The slack message was sent successfully")
	return nil
}

// sendEmailNotification sends an email notification through workflow activity execution.
//
// This method executes the SendMessageToEmailActivity as part of a notification workflow.
//
// Params:
// - ctx: The workflow context for the operation.
// - to: List of email addresses to send the notification to.
// - subject: The email subject line.
// - text: The email message content.
//
// Returns:
// - error: Error information if the activity execution fails.
func sendEmailNotification(ctx workflow.Context, to []string, subject, text string) error {
	logger := workflow.GetLogger(ctx)
	ao := workflowActivityOpts
	if err := workflow.ExecuteActivity(
		workflow.WithActivityOptions(ctx, ao),
		notificationActivities.SendMessageToEmailActivity,
		&notificationActivities.SendMessageToEmailActivityRequest{
			To:      to,
			Subject: subject,
			Text:    text,
			SendAs:  "michelangelo@uber.com",
		}).Get(ctx, nil); err != nil {
		logger.Error("The email message failed to send with", zap.Error(err))
		return err
	}
	logger.Info("The email message sent successfully")
	return nil
}

// SendPRNotification sends notifications for a pipeline run based on configured notification settings.
//
// This method is executed as a workflow to process pipeline run state changes and send
// appropriate notifications via email and Slack channels based on the notification configuration.
//
// Params:
// - ctx: The workflow context for the operation.
// - pipelineRun: The pipeline run object containing notification configurations and current state.
//
// Returns:
// - error: Error information if any notification delivery fails.
func SendPRNotification(ctx workflow.Context, pipelineRun *v2pb.PipelineRun) error {
	ctx = workflow.WithActivityOptions(ctx, workflowActivityOpts)
	logger := workflow.GetLogger(ctx)
	notifications := pipelineRun.Spec.Notifications
	var err error
	for _, notif := range notifications {
		eventTypes := notif.EventTypes
		if types.ContainsEventType(eventTypes, pipelineRun.Status.State) {
			err = sendEmailNotification(ctx, notif.Emails,
				types.GenerateSubject(pipelineRun),
				types.GenerateText(pipelineRun, "email"))
			if err != nil {
				logger.Error("Email notification sent failed with", zap.Error(err))
			}
			for _, slack := range notif.SlackDestinations {
				err = sendSlackNotification(ctx, slack,
					types.GenerateText(pipelineRun, "slack"))
				if err != nil {
					logger.Error("Slack notification sent failed with", zap.Error(err))
				}
			}
		}
	}
	return err
}
