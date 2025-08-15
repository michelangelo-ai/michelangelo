package http

import (
	"go.uber.org/config"
	"net/http"

	"go.uber.org/fx"
)

const configKey = "compute"

// Config contains shared configuration options for HTTP API clients.
type Config struct {
	BaseURL      string `yaml:"baseUrl"`
	Workspace    string `yaml:"workspace"`
	Environment  string `yaml:"environment"`
	SparkDepsURL string `yaml:"sparkDepsUrl,omitempty"`
}

// Module provides a shared HTTP client for all HTTP-based activities.
var Module = fx.Options(
	fx.Provide(NewHTTPClient),
	fx.Provide(NewConfig),
)

// NewHTTPClient creates a new HTTP client for API operations.
func NewHTTPClient() *http.Client {
	// Could be extended to include custom transport, timeouts, etc.
	return &http.Client{}
}

// NewConfig creates a new Config from a provider.
func NewConfig(provider config.Provider) (Config, error) {
	var conf Config
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
