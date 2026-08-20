// Package notification provides Cadence/Temporal activities for delivering
// pipeline run notifications.
package notification

import (
	"context"
	"errors"

	"github.com/cadence-workflow/starlark-worker/activity"
	maSlack "github.com/michelangelo-ai/michelangelo/go/base/notification/slack"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

// Activity names registered with the Cadence/Temporal worker.
const (
	SendMessageToEmailActivityName = "SendMessageToEmailActivity"
	SendMessageToSlackActivityName = "SendMessageToSlackActivity"
)

// SendMessageToSlackActivityRequest holds the parameters for a Slack notification.
type SendMessageToSlackActivityRequest struct {
	// Channel is the Slack channel or user to send the message to.
	Channel string `json:"channel"`
	// Text is the message content (used as fallback when Blocks is provided).
	Text string `json:"text"`
	// Blocks is an optional list of Slack Block Kit blocks for rich formatting.
	// When provided, Text is used as the notification fallback.
	Blocks []slack.Block `json:"blocks,omitempty"`
}

// SendMessageToEmailActivityRequest holds the parameters for an email notification.
type SendMessageToEmailActivityRequest struct {
	// To is the list of primary recipient email addresses.
	To []string `json:"to"`
	// Cc is the list of CC recipient email addresses.
	Cc []string `json:"cc,omitempty"`
	// Bcc is the list of BCC recipient email addresses.
	Bcc []string `json:"bcc,omitempty"`
	// Subject is the email subject line.
	Subject string `json:"subject"`
	// ReplyTo is an optional Reply-To address.
	ReplyTo string `json:"replyTo,omitempty"`
	// HTML is the HTML body of the email.
	HTML string `json:"html,omitempty"`
	// Text is the plain-text body of the email.
	Text string `json:"text,omitempty"`
	// SendAs is the From address shown to recipients.
	SendAs string `json:"send_as"`
	// Additional fields (attachments, categories, headers) can be added here
	// when integrating with a real email transport (SMTP, SendGrid, etc.).
}

// Activities holds notification activity implementations.
type Activities struct {
	slackClient *maSlack.Client
}

// NewActivities creates a new Activities instance with the given Slack client.
// If slackClient is nil, Slack notifications will be skipped.
func NewActivities(slackClient *maSlack.Client) *Activities {
	return &Activities{slackClient: slackClient}
}

// SendMessageToSlackActivity sends a Slack notification using Block Kit or plain text.
//
// When Blocks is provided, the Slack API will render the Block Kit UI and use
// Text as the notification fallback. Falls back to plain text when Blocks is empty.
//
// If no Slack client is configured, the notification is skipped with a warning.
func (a *Activities) SendMessageToSlackActivity(ctx context.Context, req *SendMessageToSlackActivityRequest) error {
	if req == nil {
		return errors.New("SendMessageToSlackActivityRequest cannot be nil")
	}

	logger := activity.GetLogger(ctx)
	if logger == nil {
		logger = zap.NewNop()
	}

	if a.slackClient == nil {
		logger.Warn("Slack client not configured, skipping notification",
			zap.String("channel", req.Channel))
		return nil
	}

	// Send with Block Kit if blocks are provided
	if len(req.Blocks) > 0 {
		if err := a.slackClient.PostBlocks(req.Channel, req.Blocks...); err != nil {
			logger.Error("Failed to send Block Kit notification", zap.Error(err))
			return err
		}
		logger.Info("Slack Block Kit notification sent", zap.String("channel", req.Channel))
		return nil
	}

	// Fall back to plain text
	if err := a.slackClient.PostMessage(req.Channel, req.Text); err != nil {
		logger.Error("Failed to send Slack notification", zap.Error(err))
		return err
	}
	logger.Info("Slack notification sent", zap.String("channel", req.Channel))
	return nil
}

// SendMessageToEmailActivity sends an email notification.
//
// This is a no-op stub. Operators should integrate a real email transport
// (SMTP, SendGrid, etc.) by replacing this implementation or using fx.Decorate
// to provide a custom EmailSink.
func (a *Activities) SendMessageToEmailActivity(ctx context.Context, req *SendMessageToEmailActivityRequest) error {
	if req == nil {
		return errors.New("SendMessageToEmailActivityRequest cannot be nil")
	}

	logger := activity.GetLogger(ctx)
	if logger == nil {
		logger = zap.NewNop()
	}

	logger.Warn("SendMessageToEmailActivity called (no-op: no email transport configured)",
		zap.Strings("to", req.To),
		zap.String("subject", req.Subject),
		zap.String("send_as", req.SendAs))
	return nil
}
