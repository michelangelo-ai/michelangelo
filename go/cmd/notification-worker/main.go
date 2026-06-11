// Package main is the entry point for the notification-worker binary.
//
// The notification-worker runs the pipeline run notification workflow and its
// activities independently of the shared worker binary. Deploying it separately
// allows operators to:
//
//   - Configure a dedicated Cadence/Temporal task list for notifications.
//   - Supply a custom notification transport (Slack API, SMTP, webhook) by
//     replacing the default no-op activity implementations.
//   - Scale or restart notification delivery without affecting other workers.
//
// # Configuration
//
// The binary reads go/cmd/notification-worker/config/base.yaml at startup.
// All fields support ${ENV_VAR:default} syntax for environment-variable override.
// Key fields:
//
//	notification.taskList       — must match PipelineRunNotifier.Config.TaskList
//	notification.studioBaseURL  — base URL for deep links in messages (must end with /)
//	notification.senderEmail    — From address for email notifications
//
// # Extending notification delivery
//
// The default SendMessageToSlackActivity and SendMessageToEmailActivity
// implementations log the request and return nil. To deliver real messages,
// replace the activity module with your own transport before calling app.Run():
//
//	func main() {
//	    fx.New(
//	        // Replace the default no-op module with your own transport:
//	        mysmtp.ActivityModule,   // provides SendMessageToEmailActivity
//	        myslack.ActivityModule,  // provides SendMessageToSlackActivity
//
//	        notificationWorkflows.Module,
//	        worker.Module,
//	        env.Module,
//	        config.Module,
//	        zapfx.Module,
//	    ).Run()
//	}
//
// To add a new notification channel (e.g. PagerDuty) without editing the
// workflow, implement the notification.Sink interface and override the sinks
// binding via fx.Decorate in your module:
//
//	fx.Decorate(func() []notification.Sink {
//	    return []notification.Sink{
//	        &notification.EmailSink{},
//	        &notification.SlackSink{},
//	        &mypagerduty.Sink{},
//	    }
//	})
package main

import (
	"github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/zapfx"
	"github.com/michelangelo-ai/michelangelo/go/worker"
	notificationActivities "github.com/michelangelo-ai/michelangelo/go/worker/activities/notification"
	notificationWorkflows "github.com/michelangelo-ai/michelangelo/go/worker/workflows/notification"

	"go.uber.org/fx"
)

func main() {
	fx.New(options()).Run()
}

func options() fx.Option {
	return fx.Options(
		notificationActivities.Module,
		notificationWorkflows.Module,

		worker.Module,
		env.Module,
		config.Module,
		zapfx.Module,
	)
}
