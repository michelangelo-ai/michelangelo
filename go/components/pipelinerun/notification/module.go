// Package notification module provides FX dependency injection for PipelineRun notifications.
package notification

import (
	"go.uber.org/config"
	"go.uber.org/fx"
)

// _configKey is the top-level YAML key for notification configuration.
const _configKey = "notification"

// Module is the FX module for PipelineRun notification functionality.
//
// It reads notification configuration from the "notification" key of the
// application config provider and provides a PipelineRunNotifier instance.
// Include it in applications that need pipeline run notification capabilities:
//
//	fx.New(
//	    notification.Module,
//	    // other modules...
//	)
var Module = fx.Options(
	fx.Provide(provideConfig),
	fx.Provide(NewPipelineRunNotifier),
)

// provideConfig reads notification configuration from the shared config provider.
func provideConfig(provider config.Provider) (Config, error) {
	cfg := Config{}
	err := provider.Get(_configKey).Populate(&cfg)
	return cfg, err
}
