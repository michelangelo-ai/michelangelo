package notification

import (
	"errors"

	"github.com/cadence-workflow/starlark-worker/workflow"
	notificationActivities "github.com/michelangelo-ai/michelangelo/go/worker/activities/notification"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.uber.org/zap"
)

// Sink delivers a notification to one or more destinations.
//
// Implementations call Cadence/Temporal activities and must therefore only be
// invoked from within a workflow execution context. Add a new Sink to the slice
// provided by provideDefaultSinks (or override via FX) to support additional
// channels such as PagerDuty or SMS without modifying the workflow.
//
// If your Sink calls a new activity, register it on the worker — see
// RegisterNotificationActivities in go/cmd/worker/main.go for the pattern.
type Sink interface {
	// Notify sends msg to all matching destinations in notif.
	// Implementations should return nil when notif contains no destinations
	// relevant to this sink — skipping silently is the correct behaviour.
	Notify(ctx workflow.Context, logger *zap.Logger, notif *v2pb.Notification, msg Message) error
}

// Well-known format keys for Message.FormattedBodies.
// Use these constants rather than raw strings to avoid typos.
const (
	// FormatHTML is the key for HTML-formatted bodies (email, Teams, etc.).
	FormatHTML = "text/html"
	// FormatSlackMrkdwn is the key for Slack mrkdwn-formatted bodies.
	FormatSlackMrkdwn = "text/slack"
)

// Message is the channel-agnostic notification payload passed to every Sink.
//
// Body is a universal plain-text fallback suitable for any channel.
// FormattedBodies holds optional format-specific overrides keyed by MIME-style
// content types (see FormatHTML, FormatSlackMrkdwn). Each Sink checks for its
// preferred format and falls back to Body. To add a new channel, implement
// Sink — no changes to Message are required.
type Message struct {
	// Subject is a short summary line (e.g. email subject).
	Subject string
	// Body is the plain-text notification body, suitable for any channel.
	Body string
	// FormattedBodies holds optional format-specific body overrides keyed by
	// content type (e.g. FormatHTML, FormatSlackMrkdwn). Sinks check for their
	// preferred format and fall back to Body.
	FormattedBodies map[string]string
	// SendAs is the sender identity for channels that support it (e.g. email From address).
	SendAs string
}

// EmailSink delivers notifications via email.
//
// The actual transport is provided by SendMessageToEmailActivity. Replace that
// activity registration with a real SMTP or transactional email implementation
// before relying on email delivery in production.
type EmailSink struct{}

// Notify sends an email to all addresses listed in notif.Emails.
// Uses text/html from FormattedBodies when available, falling back to Body.
// Returns nil immediately when Emails is empty.
func (s *EmailSink) Notify(ctx workflow.Context, _ *zap.Logger, notif *v2pb.Notification, msg Message) error {
	if len(notif.Emails) == 0 {
		return nil
	}
	req := &notificationActivities.SendMessageToEmailActivityRequest{
		To:      notif.Emails,
		Subject: msg.Subject,
		Text:    msg.Body,
		SendAs:  msg.SendAs,
	}
	if html, ok := msg.FormattedBodies[FormatHTML]; ok && html != "" {
		req.HTML = html
	}
	return workflow.ExecuteActivity(
		workflow.WithActivityOptions(ctx, workflowActivityOpts),
		notificationActivities.SendMessageToEmailActivity,
		req).Get(ctx, nil)
}

// SlackSink delivers notifications to Slack channels.
//
// The actual transport is provided by SendMessageToSlackActivity. Replace that
// activity registration with a real Slack API implementation before relying on
// Slack delivery in production.
type SlackSink struct{}

// Notify posts a message to every channel in notif.SlackDestinations.
// Uses text/slack from FormattedBodies when available, falling back to Body.
// Errors from individual channels are accumulated with errors.Join so that a
// failure on one channel does not suppress delivery to others.
func (s *SlackSink) Notify(ctx workflow.Context, logger *zap.Logger, notif *v2pb.Notification, msg Message) error {
	text := msg.Body
	if mrkdwn, ok := msg.FormattedBodies[FormatSlackMrkdwn]; ok && mrkdwn != "" {
		text = mrkdwn
	}
	var errs error
	for _, channel := range notif.SlackDestinations {
		err := workflow.ExecuteActivity(
			workflow.WithActivityOptions(ctx, workflowActivityOpts),
			notificationActivities.SendMessageToSlackActivity,
			&notificationActivities.SendMessageToSlackActivityRequest{
				Channel: channel,
				Text:    text,
			}).Get(ctx, nil)
		if err != nil {
			if logger != nil {
				logger.Error("Slack notification failed", zap.String("channel", channel), zap.Error(err))
			}
			errs = errors.Join(errs, err)
		}
	}
	return errs
}
