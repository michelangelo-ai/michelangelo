// Package notification provides Cadence/Temporal activities for delivering
// pipeline run notifications.
//
// Default implementations log the request and return nil. They are intentional
// no-ops: no message is sent unless the activity body is replaced with a real
// transport (SMTP, Slack API, etc.).
//
// The preferred customization path for operators using fx is fx.Decorate on the
// Sink interface in the notification workflow module:
//
//	fx.Decorate(func() []notification.Sink {
//	    return []notification.Sink{&MyEmailSink{}, &MySlackSink{}}
//	})
//
// Replacing the function body of SendMessageToEmailActivity or
// SendMessageToSlackActivity directly is a last-resort alternative for operators
// not using fx. In either case, real transport integration is required before
// notifications will be delivered in production.
package notification

import (
	"context"
	"errors"

	"github.com/cadence-workflow/starlark-worker/activity"
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
	// Text is the message content.
	Text string `json:"text"`
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

// SendMessageToSlackActivity is the default Slack notification activity.
//
// This implementation logs the request and returns nil without sending any
// message. The preferred customization path is fx.Decorate on the Sink interface
// in the notification workflow module. Replacing the body of this function
// directly is a last-resort alternative for operators not using fx — integrate
// a real transport (Slack API, etc.) before relying on Slack notifications in
// production.
func SendMessageToSlackActivity(ctx context.Context, req *SendMessageToSlackActivityRequest) error {
	if req == nil {
		return errors.New("SendMessageToSlackActivityRequest cannot be nil")
	}
	if logger := activity.GetLogger(ctx); logger != nil {
		logger.Warn("SendMessageToSlackActivity called (no-op: no transport configured)",
			zap.String("channel", req.Channel),
			zap.String("text", req.Text))
	}
	return nil
}

// SendMessageToEmailActivity is the default email notification activity.
//
// This implementation logs the request and returns nil without sending any
// message. The preferred customization path is fx.Decorate on the Sink interface
// in the notification workflow module. Replacing the body of this function
// directly is a last-resort alternative for operators not using fx — integrate
// a real transport (SMTP, SendGrid, etc.) before relying on email notifications
// in production.
func SendMessageToEmailActivity(ctx context.Context, req *SendMessageToEmailActivityRequest) error {
	if req == nil {
		return errors.New("SendMessageToEmailActivityRequest cannot be nil")
	}
	if logger := activity.GetLogger(ctx); logger != nil {
		logger.Warn("SendMessageToEmailActivity called (no-op: no transport configured)",
			zap.Strings("to", req.To),
			zap.String("subject", req.Subject),
			zap.String("send_as", req.SendAs))
	}
	return nil
}
