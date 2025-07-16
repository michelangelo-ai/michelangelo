package rayhttp

import (
	"net/http"

	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/fx"
)

// Config contains configuration options for the Ray HTTP API client.
type Config struct {
	BaseURL string `yaml:"baseUrl"`
}

// Module defines the dependency injection options for the fx framework.
// It provides the HTTP client for Ray operations and registers activities with the worker.
var Module = fx.Options(
	fx.Provide(
		NewHTTPClient,
	),
	fx.Invoke(register),
)

// HTTPClientParams contains the dependencies needed to create an HTTP client.
type HTTPClientParams struct {
	Config Config
}

// NewHTTPClient creates a new HTTP client for Ray API operations.
func NewHTTPClient(p HTTPClientParams) *http.Client {
	// Could be extended to include custom transport, timeouts, etc.
	return &http.Client{}
}

// ActivitiesParams contains the dependencies needed to create and register activities.
type ActivitiesParams struct {
	Workers    []worker.Worker
	HTTPClient *http.Client
	Config     Config
}

// register initializes and registers the Ray HTTP activities with the worker.
func register(p ActivitiesParams) {
	// Initialize the activities with the HTTP client and base URL
	a := &activities{
		httpClient: p.HTTPClient,
		apiBaseURL: p.Config.BaseURL,
	}

	// Register the activities with each worker
	for _, w := range p.Workers {
		w.RegisterActivity(a)
	}
}