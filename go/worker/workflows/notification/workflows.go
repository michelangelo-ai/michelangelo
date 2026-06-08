// Package notification provides the pipeline run notification workflow.
package notification

import (
	"errors"
	"time"

	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	notificationActivities "github.com/michelangelo-ai/michelangelo/go/worker/activities/notification"
	"go.uber.org/zap"
)

var workflowActivityOpts = workflow.ActivityOptions{
	ScheduleToStartTimeout: 1 * time.Minute,
	StartToCloseTimeout:    30 * time.Minute,
	HeartbeatTimeout:       1 * time.Minute,
}

// sendSlackNotification executes SendMessageToSlackActivity as a workflow activity.
func sendSlackNotification(ctx workflow.Context, channel, text string) error {
	logger := workflow.GetLogger(ctx)
	err := workflow.ExecuteActivity(
		workflow.WithActivityOptions(ctx, workflowActivityOpts),
		notificationActivities.SendMessageToSlackActivity,
		&notificationActivities.SendMessageToSlackActivityRequest{
			Channel: channel,
			Text:    text,
		}).Get(ctx, nil)
	if err != nil {
		logger.Error("Slack notification failed", zap.Error(err))
		return err
	}
	logger.Info("Slack notification sent")
	return nil
}

// sendEmailNotification executes SendMessageToEmailActivity as a workflow activity.
func sendEmailNotification(ctx workflow.Context, to []string, subject, text, sendAs string) error {
	logger := workflow.GetLogger(ctx)
	err := workflow.ExecuteActivity(
		workflow.WithActivityOptions(ctx, workflowActivityOpts),
		notificationActivities.SendMessageToEmailActivity,
		&notificationActivities.SendMessageToEmailActivityRequest{
			To:      to,
			Subject: subject,
			Text:    text,
			SendAs:  sendAs,
		}).Get(ctx, nil)
	if err != nil {
		logger.Error("Email notification failed", zap.Error(err))
		return err
	}
	logger.Info("Email notification sent")
	return nil
}

// SendPipelineRunNotification fans out email and Slack notifications for a
// pipeline run state change.
//
// Notification delivery failures are accumulated with errors.Join so that a
// failure on one channel does not suppress errors from others. The workflow
// returns a non-nil error only when at least one notification fails.
func SendPipelineRunNotification(ctx workflow.Context, req *types.PipelineRunNotificationRequest) error {
	ctx = workflow.WithActivityOptions(ctx, workflowActivityOpts)
	logger := workflow.GetLogger(ctx)

	pipelineRun := req.PipelineRun
	var errs error

	for _, notif := range pipelineRun.Spec.Notifications {
		if !types.ContainsEventType(notif.EventTypes, pipelineRun.Status.State) {
			continue
		}

		notifText := types.GenerateText(pipelineRun, "email", req.StudioBaseURL, nil)
		if len(notif.Emails) > 0 {
			if err := sendEmailNotification(ctx, notif.Emails,
				types.GenerateSubject(pipelineRun),
				notifText,
				req.SenderEmail,
			); err != nil {
				logger.Error("Email notification failed", zap.Error(err))
				errs = errors.Join(errs, err)
			}
		}

		slackText := types.GenerateText(pipelineRun, "slack", req.StudioBaseURL, nil)
		for _, channel := range notif.SlackDestinations {
			if err := sendSlackNotification(ctx, channel, slackText); err != nil {
				logger.Error("Slack notification failed", zap.String("channel", channel), zap.Error(err))
				errs = errors.Join(errs, err)
			}
		}
	}

	return errs
}
