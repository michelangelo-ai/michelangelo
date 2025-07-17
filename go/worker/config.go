package worker

import (
	"go.uber.org/config"
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/worker/activities/rayhttp"
)

const configKey = "worker"

// Config represents the worker configuration.
type Config struct {
	RayHTTP rayhttp.Config `yaml:"rayHttp"`
}

// Params provides dependencies for worker.
type Params struct {
	fx.In

	Config Config
}

// NewConfig creates a new Config from a provider.
func NewConfig(provider config.Provider) (Config, error) {
	var conf Config
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}

// GetRayHTTPConfig returns the Ray HTTP API configuration.
func GetRayHTTPConfig(p Params) rayhttp.Config {
	return p.Config.RayHTTP
}
