package http

import (
	"net/http"

	"go.uber.org/fx"
)

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
)

// NewHTTPClient creates a new HTTP client for API operations.
func NewHTTPClient() *http.Client {
	// Could be extended to include custom transport, timeouts, etc.
	return &http.Client{}
}