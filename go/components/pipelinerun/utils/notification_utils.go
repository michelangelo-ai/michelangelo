package utils

import (
	"context"
	"time"

	"github.com/cadence-workflow/starlark-worker/activity"
	"github.com/cadence-workflow/starlark-worker/worker"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Workflow activity option defaults
const (
	DefaultScheduleToStartTimeout = 1 * time.Minute
	DefaultStartToCloseTimeout    = 30 * time.Minute
	DefaultHeartbeatTimeout       = 1 * time.Minute
	_defaultMaEmail               = "michelangelo@uber.com"
)

// Notification Workflow and Activity names
const (
	SendMessageToEmailActivityName = "SendMessageToEmailActivity"
	SendMessageToSlackActivityName = "SendMessageToSlackActivity"
	PRNotificationWorkflowName     = "PRNotificationWorkflow"
)

var (
	workflowActivityOpts = workflow.ActivityOptions{
		ScheduleToStartTimeout: DefaultScheduleToStartTimeout,
		StartToCloseTimeout:    DefaultStartToCloseTimeout,
		HeartbeatTimeout:       DefaultHeartbeatTimeout,
	}
)

type (
	// workflows struct encapsulates the trigger workflow
	workflows struct {
		workflow workflow.Workflow
	}
	// SendMessageToSlackActivityRequest is the request to send a message to slack
	SendMessageToSlackActivityRequest struct {
		Channel string `json:"channel"`
		Text    string `json:"text"`
	}
	// SendMessageToEmailActivityRequest is the request to send an email
	SendMessageToEmailActivityRequest struct {
		To      []string `json:"to" description:"list of email addresses."`
		Cc      []string `json:"cc,omitempty"`
		Bcc     []string `json:"bcc,omitempty"`
		Subject string   `json:"subject" description:"email subject line."`
		ReplyTo string   `json:"replyTo,omitempty"`
		HTML    string   `json:"html,omitempty"`
		Text    string   `json:"text,omitempty"`
		SendAs  string   `json:"send_as" description:"email address to be shown as the sender."`
		// Note: Removed attachments and categories that depend on external CAG types
		// These can be added back when integrating with internal systems
	}
)

// SendMessageToSlackActivity sends a message to slack
func SendMessageToSlackActivity(ctx context.Context, req *SendMessageToSlackActivityRequest) error {
	// TODO: Implement slack sending logic
	// This would typically integrate with internal CAG (Communication API Gateway) service
	logger := activity.GetLogger(ctx)
	logger.Info("Sending slack notification",
		zap.String("channel", req.Channel),
		zap.String("text", req.Text))

	// Placeholder implementation - replace with actual CAG integration
	// deps, err := activityctx.GetActivityDepsFromContext(ctx)
	// if err != nil {
	//     return fmt.Errorf("Failed to get activity deps from context (slack)")
	// }
	// err = deps.CAG.SendSlack(ctx, &cag.SendSlackRequest{
	//     Channel: req.Channel,
	//     Text:    req.Text,
	// })
	// if err != nil {
	//     logger.Error("CAG request for slack actor failed", zap.Error(err))
	//     return fmt.Errorf("CAG request for slack actor failed with err:%v", err)
	// }

	return nil
}

// SendMessageToEmailActivity sends a message to email
func SendMessageToEmailActivity(ctx context.Context, req SendMessageToEmailActivityRequest) error {
	// TODO: Implement email sending logic
	// This would typically integrate with internal CAG (Communication API Gateway) service
	logger := activity.GetLogger(ctx)
	logger.Info("Sending email notification",
		zap.Strings("to", req.To),
		zap.String("subject", req.Subject))

	// Placeholder implementation - replace with actual CAG integration
	// deps, err := activityctx.GetActivityDepsFromContext(ctx)
	// if err != nil {
	//     return fmt.Errorf("Failed to get activity deps from context (email)")
	// }
	// err = deps.CAG.SendEmail(ctx, &cag.SendEmailRequest{
	//     To:      req.To,
	//     Cc:      req.Cc,
	//     Bcc:     req.Bcc,
	//     Subject: req.Subject,
	//     ReplyTo: req.ReplyTo,
	//     HTML:    req.HTML,
	//     Text:    req.Text,
	//     SendAs:  req.SendAs,
	// })
	// if err != nil {
	//     logger.Error("CAG request for email actor failed", zap.Error(err))
	//     return fmt.Errorf("CAG request for email actor failed with err:%v", err)
	// }

	return nil
}

// SendSlackNotification sends a slack notification
func sendSlackNotification(ctx workflow.Context, channel, text string) error {
	logger := workflow.GetLogger(ctx)
	ao := workflowActivityOpts
	if err := workflow.ExecuteActivity(
		workflow.WithActivityOptions(ctx, ao),
		SendMessageToSlackActivity,
		&SendMessageToSlackActivityRequest{
			Channel: channel,
			Text:    text,
		}).Get(ctx, nil); err != nil {
		logger.Error("The slack message failed to send with", zap.Error(err))
		return err
	}
	logger.Info("The slack message was sent successfully")
	return nil
}

// SendEmailNotification sends an email notification
func sendEmailNotification(ctx workflow.Context, to []string, subject, text string) error {
	logger := workflow.GetLogger(ctx)
	ao := workflowActivityOpts
	if err := workflow.ExecuteActivity(
		workflow.WithActivityOptions(ctx, ao),
		SendMessageToEmailActivity,
		&SendMessageToEmailActivityRequest{
			To:      to,
			Subject: subject,
			Text:    text,
			SendAs:  _defaultMaEmail,
		}).Get(ctx, nil); err != nil {
		logger.Error("The email message failed to send with", zap.Error(err))
		return err
	}
	logger.Info("The email message sent successfully")
	return nil
}

// SendPRNotification sends a notification for a pipeline run
func (r *workflows) SendPRNotification(ctx workflow.Context, pipelineRun *v2pb.PipelineRun) error {
	ctx = workflow.WithBackend(ctx, r.workflow)
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

// Module provides FX dependency injection for notification workflow and activities.
var Module = fx.Options(
	fx.Invoke(registerNotificationWorkflowsAndActivities),
)

// registerNotificationWorkflowsAndActivities registers notification workflows and activities with the workers.
func registerNotificationWorkflowsAndActivities(workers []worker.Worker, workflow workflow.Workflow) {
	ws := workflows{workflow: workflow}
	for _, w := range workers {
		// Register workflow
		w.RegisterWorkflow(ws.SendPRNotification, PRNotificationWorkflowName)

		// Register activities
		w.RegisterActivity(SendMessageToEmailActivity)
		w.RegisterActivity(SendMessageToSlackActivity)
	}
}
