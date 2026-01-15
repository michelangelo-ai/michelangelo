package notification

import (
	"fmt"

	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/notification"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
)

var Workflows = (*workflows)(nil)

const (
	// Workflow name for pipeline run notifications
	PipelineRunNotificationWorkflowName = "PipelineRunNotificationWorkflow"
)

type workflows struct{}

// PipelineRunNotificationWorkflow processes a pipeline run notification event.
//
// This workflow orchestrates the sending of notifications for a pipeline run
// state change. It processes each notification configuration, filters by
// event type, and sends appropriate email and Slack messages.
//
// The workflow is designed to be idempotent and handles failures gracefully
// by continuing to process other notifications even if some fail.
func (w *workflows) PipelineRunNotificationWorkflow(
	ctx workflow.Context,
	event *types.NotificationEvent,
) error {
	logger := workflow.GetLogger(ctx)

	if event == nil || event.PipelineRun == nil {
		return fmt.Errorf("invalid notification event: missing pipeline run data")
	}

	pipelineRun := event.PipelineRun
	logger.Info("Processing pipeline run notification",
		zap.String("pipeline_run", pipelineRun.Name),
		zap.String("namespace", pipelineRun.Namespace),
		zap.String("event_type", event.EventType.String()),
		zap.Int("notification_count", len(event.Notifications)))

	// Process each notification configuration
	for i, notificationConfig := range event.Notifications {
		notificationLogger := logger.With(
			zap.Int("notification_index", i),
			zap.String("notification_type", notificationConfig.NotificationType.String()))

		// Check if this notification should be sent for the event type
		shouldSend := false
		for _, eventType := range notificationConfig.EventTypes {
			if eventType == event.EventType {
				shouldSend = true
				break
			}
		}

		if !shouldSend {
			notificationLogger.Debug("Notification not configured for this event type")
			continue
		}

		// Send email notifications if configured
		if len(notificationConfig.Emails) > 0 {
			if err := w.sendEmailNotification(ctx, pipelineRun, notificationConfig); err != nil {
				notificationLogger.Error("Failed to send email notification", zap.Error(err))
				// Continue processing other notifications
			}
		}

		// Send Slack notifications if configured
		if len(notificationConfig.SlackDestinations) > 0 {
			if err := w.sendSlackNotification(ctx, pipelineRun, notificationConfig); err != nil {
				notificationLogger.Error("Failed to send Slack notification", zap.Error(err))
				// Continue processing other notifications
			}
		}
	}

	logger.Info("Pipeline run notification processing completed")
	return nil
}

// sendEmailNotification sends email notifications for a pipeline run.
func (w *workflows) sendEmailNotification(
	ctx workflow.Context,
	pipelineRun *v2pb.PipelineRun,
	notificationConfig *v2pb.Notification,
) error {
	if len(notificationConfig.Emails) == 0 {
		return nil
	}

	// Generate email content
	var emailRequest notification.SendEmailActivityRequest
	if err := workflow.ExecuteActivity(ctx, notification.Activities.GenerateEmailActivity, pipelineRun).Get(ctx, &emailRequest); err != nil {
		return fmt.Errorf("failed to generate email content: %w", err)
	}

	// Set recipients from notification configuration
	emailRequest.To = notificationConfig.Emails

	// Send email via activity
	return workflow.ExecuteActivity(ctx, notification.Activities.SendEmailActivity, emailRequest).Get(ctx, nil)
}

// sendSlackNotification sends Slack notifications for a pipeline run.
func (w *workflows) sendSlackNotification(
	ctx workflow.Context,
	pipelineRun *v2pb.PipelineRun,
	notificationConfig *v2pb.Notification,
) error {
	if len(notificationConfig.SlackDestinations) == 0 {
		return nil
	}

	// Generate Slack content
	var slackText string
	if err := workflow.ExecuteActivity(ctx, notification.Activities.GenerateSlackActivity, pipelineRun).Get(ctx, &slackText); err != nil {
		return fmt.Errorf("failed to generate Slack content: %w", err)
	}

	// Send to each Slack destination
	for _, destination := range notificationConfig.SlackDestinations {
		slackRequest := notification.SendSlackActivityRequest{
			Channel: destination,
			Text:    slackText,
		}

		if err := workflow.ExecuteActivity(ctx, notification.Activities.SendSlackActivity, slackRequest).Get(ctx, nil); err != nil {
			return fmt.Errorf("failed to send Slack notification to %s: %w", destination, err)
		}
	}

	return nil
}