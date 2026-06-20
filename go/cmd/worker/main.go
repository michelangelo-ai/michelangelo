package main

import (
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/worker"
	"github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/zapfx"
	workermod "github.com/michelangelo-ai/michelangelo/go/worker"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/notification"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/ray"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/spark"
	rayplugin "github.com/michelangelo-ai/michelangelo/go/worker/plugins/ray"
	sparkplugin "github.com/michelangelo-ai/michelangelo/go/worker/plugins/spark"
	notificationWorkflows "github.com/michelangelo-ai/michelangelo/go/worker/workflows/notification"

	"go.uber.org/fx"
)

func main() {
	fx.New(options()).Run()
}

func options() fx.Option {
	return fx.Options(
		ray.Module,
		spark.Module,
		notificationWorkflows.Module,
		fx.Invoke(RegisterRayPlugin),
		fx.Invoke(RegisterSparkPlugin),
		fx.Invoke(RegisterNotificationActivities),

		workermod.Module,
		env.Module,
		config.Module,
		zapfx.Module,
	)
}

// RegisterRayPlugin adds the ray plugin to the plugin registry.
func RegisterRayPlugin(registry map[string]service.IPlugin) {
	registry[rayplugin.Plugin.ID()] = rayplugin.Plugin
}

// RegisterSparkPlugin adds the spark plugin to the plugin registry.
func RegisterSparkPlugin(registry map[string]service.IPlugin) {
	registry[sparkplugin.Plugin.ID()] = sparkplugin.Plugin
}

// RegisterNotificationActivities registers the default (no-op) email and Slack
// notification activities. Downstream forks should replace this function with
// their own transport implementations (SMTP, Slack API, PagerDuty, etc.).
func RegisterNotificationActivities(workers []worker.Worker) {
	for _, w := range workers {
		w.RegisterActivity(notification.SendMessageToEmailActivity)
		w.RegisterActivity(notification.SendMessageToSlackActivity)
	}
}
