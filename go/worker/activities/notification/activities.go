package notification

import (
	"context"

	"github.com/cadence-workflow/starlark-worker/activity"
	"go.uber.org/zap"
)

// Notification Activity names
const (
	SendMessageToEmailActivityName = "SendMessageToEmailActivity"
	SendMessageToSlackActivityName = "SendMessageToSlackActivity"
)

const (
	_defaultMaEmail = "michelangelo@uber.com"
)

type (
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

// SendMessageToSlackActivity sends a message to a Slack channel.
//
// This method is executed as part of a Starlark worker activity for pipeline run notifications.
//
// Params:
// - ctx: The context for the operation.
// - req: The request containing the Slack channel and message text.
//
// Returns:
// - error: Error information if the operation fails.
func SendMessageToSlackActivity(ctx context.Context, req *SendMessageToSlackActivityRequest) error {
	// TODO(#700): Implement slack sending logic
	// This would typically integrate with internal CAG (Communication API Gateway) service
	logger := activity.GetLogger(ctx)
	if logger == nil {
		// For testing contexts where activity logger is not available
		logger = zap.NewNop()
	}
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

// SendMessageToEmailActivity sends an email message.
//
// This method is executed as part of a Starlark worker activity for pipeline run notifications.
//
// Params:
// - ctx: The context for the operation.
// - req: The request containing email recipients, subject, and message content.
//
// Returns:
// - error: Error information if the operation fails.
func SendMessageToEmailActivity(ctx context.Context, req *SendMessageToEmailActivityRequest) error {
	// TODO(#701): Implement email sending logic
	// This would typically integrate with internal CAG (Communication API Gateway) service
	logger := activity.GetLogger(ctx)
	if logger == nil {
		// For testing contexts where activity logger is not available
		logger = zap.NewNop()
	}
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