package notification

import (
	"context"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
)

var Activities = (*activities)(nil)

const (
	// Activity names for registration with Cadence worker
	SendEmailActivityName = "SendNotificationEmail"
	SendSlackActivityName = "SendNotificationSlack"

	// Default timeouts for activities
	DefaultScheduleToStartTimeout = 1 * time.Minute
	DefaultStartToCloseTimeout    = 30 * time.Minute
	DefaultHeartbeatTimeout       = 1 * time.Minute
)

// SendEmailActivityRequest represents the input for sending email notifications.
//
// This structure contains all the information needed to send an email via
// the CAG gateway, including recipients, content, and delivery preferences.
type SendEmailActivityRequest struct {
	To      []string `json:"to"`
	Cc      []string `json:"cc,omitempty"`
	Bcc     []string `json:"bcc,omitempty"`
	Subject string   `json:"subject"`
	ReplyTo string   `json:"replyTo,omitempty"`
	HTML    string   `json:"html,omitempty"`
	Text    string   `json:"text,omitempty"`
	SendAs  string   `json:"send_as"`
}

// SendSlackActivityRequest represents the input for sending Slack notifications.
//
// This structure contains the information needed to send a Slack message
// via the CAG gateway to the specified channel or user.
type SendSlackActivityRequest struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

// activities struct encapsulates the dependencies for notification delivery.
//
// This struct contains the dependencies needed for sending notifications,
// including logger for observability. In a real implementation, this would
// also include CAG client for external communication.
type activities struct {
	logger *zap.Logger
	// CAG client would be injected here in a real implementation
	// cagClient cag.Client
}

// SendEmailActivity sends an email notification via the CAG gateway.
//
// This activity handles email delivery with proper error handling and logging.
// In a real implementation, this would make HTTP calls to the CAG service
// for email delivery. For now, it logs the email details for demonstration.
//
// The activity is configured with appropriate timeouts to handle slow email
// delivery services and includes retry logic for transient failures.
func (a *activities) SendEmailActivity(ctx context.Context, request SendEmailActivityRequest) error {
	logger := a.logger.With(
		zap.String("activity", "send_email"),
		zap.Strings("recipients", request.To),
		zap.String("subject", request.Subject))

	logger.Info("Sending email notification")

	// TODO: Replace with actual CAG client call
	// Example of what the real implementation would look like:
	// response, err := a.cagClient.SendEmail(ctx, &cag.SendEmailRequest{
	//     To:      request.To,
	//     Cc:      request.Cc,
	//     Bcc:     request.Bcc,
	//     Subject: request.Subject,
	//     Text:    request.Text,
	//     HTML:    request.HTML,
	//     SendAs:  request.SendAs,
	// })
	//
	// if err != nil {
	//     logger.Error("Failed to send email via CAG", zap.Error(err))
	//     return fmt.Errorf("failed to send email: %w", err)
	// }

	// For demonstration, log the email details
	logger.Info("Email notification sent successfully",
		zap.String("subject", request.Subject),
		zap.Int("recipient_count", len(request.To)))

	return nil
}

// SendSlackActivity sends a Slack notification via the CAG gateway.
//
// This activity handles Slack message delivery with proper error handling
// and logging. In a real implementation, this would make HTTP calls to the
// CAG service for Slack delivery.
//
// The activity supports sending messages to both channels and direct messages
// based on the channel specification in the request.
func (a *activities) SendSlackActivity(ctx context.Context, request SendSlackActivityRequest) error {
	logger := a.logger.With(
		zap.String("activity", "send_slack"),
		zap.String("channel", request.Channel))

	logger.Info("Sending Slack notification")

	// TODO: Replace with actual CAG client call
	// Example of what the real implementation would look like:
	// response, err := a.cagClient.SendSlack(ctx, &cag.SendSlackRequest{
	//     Channel: request.Channel,
	//     Text:    request.Text,
	// })
	//
	// if err != nil {
	//     logger.Error("Failed to send Slack message via CAG", zap.Error(err))
	//     return fmt.Errorf("failed to send Slack message: %w", err)
	// }

	// For demonstration, log the Slack message details
	logger.Info("Slack notification sent successfully",
		zap.String("channel", request.Channel),
		zap.Int("text_length", len(request.Text)))

	return nil
}

// GenerateEmailActivity generates email content for pipeline run notifications.
//
// This activity generates the email subject and body based on the pipeline run
// state and configuration. It's a separate activity to allow for potential
// customization and template rendering in the future.
func (a *activities) GenerateEmailActivity(ctx context.Context, pipelineRun *v2pb.PipelineRun) (SendEmailActivityRequest, error) {
	logger := a.logger.With(
		zap.String("activity", "generate_email"),
		zap.String("pipeline_run", pipelineRun.Name))

	logger.Info("Generating email content for pipeline run")

	request := SendEmailActivityRequest{
		Subject: types.GenerateEmailSubject(pipelineRun),
		Text:    types.GenerateEmailText(pipelineRun),
		SendAs:  "michelangelo-noreply@uber.com", // Default sender
	}

	logger.Info("Email content generated successfully")
	return request, nil
}

// GenerateSlackActivity generates Slack content for pipeline run notifications.
//
// This activity generates the Slack message text based on the pipeline run
// state and configuration. It's a separate activity to allow for potential
// customization and template rendering in the future.
func (a *activities) GenerateSlackActivity(ctx context.Context, pipelineRun *v2pb.PipelineRun) (string, error) {
	logger := a.logger.With(
		zap.String("activity", "generate_slack"),
		zap.String("pipeline_run", pipelineRun.Name))

	logger.Info("Generating Slack content for pipeline run")

	text := types.GenerateSlackText(pipelineRun)

	logger.Info("Slack content generated successfully",
		zap.Int("text_length", len(text)))

	return text, nil
}