package rayhttp

import (
	"net/http"

	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/fx"
)

// Config contains configuration options for the Ray HTTP API client.
type Config struct {
	BaseURL     string `yaml:"baseUrl"`
	Workspace   string `yaml:"workspace"`
	Environment string `yaml:"environment"`
}

// Module defines the dependency injection options for the fx framework.
// It provides the HTTP client for Ray operations and registers activities with the worker.
var Module = fx.Options(
	fx.Provide(
		NewHTTPClient,
	),
	fx.Invoke(register),
)

// NewHTTPClient creates a new HTTP client for Ray API operations.
func NewHTTPClient(config Config) *http.Client {
	// Could be extended to include custom transport, timeouts, etc.
	return &http.Client{}
}

// register initializes and registers the Ray HTTP activities with the worker.
func register(workers []worker.Worker, httpClient *http.Client, config Config) {
	// Initialize the activities with the HTTP client and configuration
	a := &activities{
		httpClient:  httpClient,
		apiBaseURL:  config.BaseURL,
		workspace:   config.Workspace,
		environment: config.Environment,
	}

	// Register the activities with each worker
	for _, w := range workers {
		w.RegisterActivity(a)
	}
}