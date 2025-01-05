// Package env provides information about the running service's environment.
package env

import (
	"go.uber.org/fx"

	"os"
)

const (
	_configPathKey         = "CONFIG_DIR"
	_runtimeEnvironmentKey = "RUNTIME_ENVIRONMENT"
	EnvProduction          = "production"
	EnvStaging             = "staging"
	EnvTest                = "test"
	EnvDevelopment         = "development"
)

// Module provides a Context, which describes the runtime context of the
// service. It's useful for other components to use when choosing a default
// configuration. It doesn't require any configuration.
var Module = fx.Module("env",
	fx.Provide(New),
)

// Result defines the objects that the env module provides.
type Result struct {
	fx.Out

	Environment Context
}

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New() Result {
	return Result{
		Environment: Context{
			ConfigPath:         os.Getenv(_configPathKey),
			Hostname:           getHostname(),
			RuntimeEnvironment: os.Getenv(_runtimeEnvironmentKey),
		},
	}
}

// Context describes the service's runtime environment
type Context struct {
	Hostname           string `yaml:"hostname"`
	ConfigPath         string `yaml:"configPath"` // directories to search for configuration files
	RuntimeEnvironment string `yaml:"runtimeEnvironment"`
}

func getHostname() string {
	if host, err := os.Hostname(); err == nil {
		return host
	}
	return ""
}
