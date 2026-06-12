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
type Sink interface {
	// Notify sends msg to all matching destinations in notif.
	// Implementations should return nil when notif contains no destinations
	// relevant to this sink — skipping silently is the correct behaviour.
	Notify(ctx workflow.Context, logger *zap.Logger, notif *v2pb.Notification, msg Message) error
}

// Message carries pre-rendered notification content for every supported channel.
// Generate it once per notification event and pass it to all sinks.
type Message struct {
	// Subject is the email subject line.
	Subject string
	// EmailText is the plain-text email body.
	EmailText string
	// SlackText is the Slack-formatted message body.
	SlackText string
	// SendAs is the From address used by the email sink.
	SendAs string
	// Metadata is an arbitrary key-value map attached to the notification event.
	// Custom sinks may use this to pass channel-specific data.
	Metadata map[string]any
}

// EmailSink delivers notifications via email.
//
// The actual transport is provided by SendMessageToEmailActivity. Replace that
// activity registration with a real SMTP or transactional email implementation
// before relying on email delivery in production.
type EmailSink struct{}

// Notify sends an email to all addresses listed in notif.Emails.
// Returns nil immediately when Emails is empty.
func (s *EmailSink) Notify(ctx workflow.Context, _ *zap.Logger, notif *v2pb.Notification, msg Message) error {
	if len(notif.Emails) == 0 {
		return nil
	}
	return workflow.ExecuteActivity(
		workflow.WithActivityOptions(ctx, workflowActivityOpts),
		notificationActivities.SendMessageToEmailActivity,
		&notificationActivities.SendMessageToEmailActivityRequest{
			To:      notif.Emails,
			Subject: msg.Subject,
			Text:    msg.EmailText,
			SendAs:  msg.SendAs,
		}).Get(ctx, nil)
}

// SlackSink delivers notifications to Slack channels.
//
// The actual transport is provided by SendMessageToSlackActivity. Replace that
// activity registration with a real Slack API implementation before relying on
// Slack delivery in production.
type SlackSink struct{}

// Notify posts a message to every channel in notif.SlackDestinations.
// Errors from individual channels are accumulated with errors.Join so that a
// failure on one channel does not suppress delivery to others.
func (s *SlackSink) Notify(ctx workflow.Context, _ *zap.Logger, notif *v2pb.Notification, msg Message) error {
	var errs error
	for _, channel := range notif.SlackDestinations {
		err := workflow.ExecuteActivity(
			workflow.WithActivityOptions(ctx, workflowActivityOpts),
			notificationActivities.SendMessageToSlackActivity,
			&notificationActivities.SendMessageToSlackActivityRequest{
				Channel: channel,
				Text:    msg.SlackText,
			}).Get(ctx, nil)
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}
