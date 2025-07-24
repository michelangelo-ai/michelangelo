package rayhttp

import (
	"net/http"

	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/fx"

	httpconfig "github.com/michelangelo-ai/michelangelo/go/worker/activities/http"
)

// Config contains configuration options for the Ray HTTP API client.
type Config struct {
	httpconfig.Config
}

// Module defines the dependency injection options for the fx framework.
// It registers activities with the worker.
var Module = fx.Options(
	fx.Invoke(register),
)

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