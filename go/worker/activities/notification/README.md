# Notification Activities

This package provides activities for sending pipeline run notifications via email and Slack.

## Configuration

### Slack Notifications

Slack notifications are **optional**. If no Slack token is configured, Slack notifications are skipped gracefully.

#### Enable Slack (via YAML config)

```yaml
# config.yaml
slack:
  token: "xoxb-your-slack-bot-token"
```

The Slack client is automatically wired via FX dependency injection when a token is present.

#### Disable Slack (default)

Simply omit the `slack` configuration section. Notifications will log a warning and skip Slack delivery:

```
WARN Slack client not configured, skipping notification channel=C123ABC
```

### Email Notifications

Email notifications are currently **no-op stubs**. To enable email delivery:

#### Option 1: Via FX Decorate (Recommended)

Replace the Activities struct with your own implementation:

```go
import (
    "github.com/michelangelo-ai/michelangelo/go/worker/activities/notification"
    "go.uber.org/fx"
)

fx.Decorate(func(slackClient *maSlack.Client) *notification.Activities {
    return &MyActivities{
        slackClient: slackClient,
        emailClient: myEmailTransport, // Your SMTP/SendGrid/SES client
    }
})
```

#### Option 2: Via FX Provide (For Custom Transports)

Provide your own activity registration:

```go
fx.Provide(func(emailTransport MyEmailClient, slackClient *maSlack.Client) *notification.Activities {
    return &notification.Activities{
        SlackClient: slackClient,
        EmailClient: emailTransport, // You'd need to add this field
    }
})
```

## Architecture

### Activities Struct

```go
type Activities struct {
    slackClient *maSlack.Client // Optional, injected via FX
}
```

Activities are **methods on the Activities struct**, allowing dependency injection via FX.

### Registration

Activities are registered automatically by `notification.Module`:

```go
import "github.com/michelangelo-ai/michelangelo/go/worker/activities/notification"

fx.Options(
    notification.Module, // Auto-registers activities
)
```

### Workflow References

Activities are referenced by **string name** in workflows:

```go
workflow.ExecuteActivity(ctx, "SendMessageToSlackActivity", req)
workflow.ExecuteActivity(ctx, "SendMessageToEmailActivity", req)
```

This allows activities to be registered in one package and called from another without circular dependencies.

## Customization Examples

### Example 1: Add AWS SES Email Support

```go
package main

import (
    "github.com/aws/aws-sdk-go/service/ses"
    "github.com/michelangelo-ai/michelangelo/go/worker/activities/notification"
)

type CustomActivities struct {
    *notification.Activities // Embed for Slack support
    sesClient *ses.SES
}

func (a *CustomActivities) SendMessageToEmailActivity(ctx context.Context, req *notification.SendMessageToEmailActivityRequest) error {
    // Real SES implementation
    _, err := a.sesClient.SendEmail(&ses.SendEmailInput{
        // ... configure email
    })
    return err
}

// In main.go:
fx.Decorate(func(slackClient *maSlack.Client, sesClient *ses.SES) *notification.Activities {
    custom := &CustomActivities{
        Activities: notification.NewActivities(slackClient),
        sesClient: sesClient,
    }
    return &notification.Activities{/* cast or adapt */}
})
```

### Example 2: Disable Slack (Email Only)

```go
// In main.go, DON'T include notification.Module
// Instead, provide your own:
fx.Provide(func() *notification.Activities {
    return &notification.Activities{
        slackClient: nil, // Slack disabled
    }
}),
fx.Invoke(func(activities *notification.Activities, workers []worker.Worker) {
    for _, w := range workers {
        w.RegisterActivity(activities)
    }
}),
```

### Example 3: Custom Slack Domain Restriction

```go
type RestrictedSlackSink struct {
    inner *notification.SlackSink
}

func (s *RestrictedSlackSink) Notify(ctx workflow.Context, logger *zap.Logger, notif *v2pb.Notification, msg notification.Message) error {
    // Only allow company channels
    for _, dest := range notif.SlackDestinations {
        if !strings.HasPrefix(dest, "C") { // Only channel IDs
            logger.Warn("Blocked non-channel Slack destination", zap.String("dest", dest))
            return nil
        }
    }
    return s.inner.Notify(ctx, logger, notif, msg)
}

// In workflow module:
fx.Decorate(func() []notification.Sink {
    return []notification.Sink{
        &notification.EmailSink{},
        &RestrictedSlackSink{inner: &notification.SlackSink{}},
    }
})
```

## Configuration Schema

### Slack

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `slack.token` | string | No | Slack bot token (starts with `xoxb-`) |

**Example:**
```yaml
slack:
  token: xoxb-YOUR-BOT-TOKEN-HERE
```

### Studio Base URL (for Block Kit links)

Configure console links in Block Kit "View Details" buttons:

```go
import notificationWorkflows "github.com/michelangelo-ai/michelangelo/go/worker/workflows/notification"

fx.Provide(func() notificationWorkflows.SinkConfig {
    return notificationWorkflows.SinkConfig{
        StudioBaseURL: "https://studio.example.com",
    }
})
```

## Testing

### Test Without Sending Real Messages

Activities gracefully skip when clients are nil:

```go
activities := notification.NewActivities(nil) // No Slack client
err := activities.SendMessageToSlackActivity(ctx, req)
// Returns nil without error, logs warning
```

### Test With Mock Client

```go
mockSlack := &MockSlackClient{}
activities := &notification.Activities{slackClient: mockSlack}
```

## Troubleshooting

### "Slack client not configured, skipping notification"

**Cause:** No `slack.token` in config.

**Solution:** Add Slack token to your YAML config, or ignore if Slack is not needed.

### "SendMessageToEmailActivity called (no-op: no email transport configured)"

**Cause:** Email activity is a stub by default.

**Solution:** Implement email transport using one of the customization patterns above.

### Activities not registered

**Cause:** `notification.Module` not included in FX options.

**Solution:** Add `notification.Module` to your `fx.Options()` in `cmd/worker/main.go`.

## See Also

- [Workflow Notification Module](../../workflows/notification/README.md)
- [Slack Client Package](../../../base/notification/slack/)
- [Notification Types](../../../base/notification/types/)