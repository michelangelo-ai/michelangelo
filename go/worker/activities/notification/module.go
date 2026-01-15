package notification

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Module defines the dependency injection options for the fx framework.
// It registers the notification activities with the workers.
var Module = fx.Options(
	fx.Invoke(register), // Invokes the register function to register activities with the workers.
)

// register registers the notification activities to the provided workers.
//
// Params:
// - workers ([]worker.Worker): A list of Cadence workers where activities will be registered.
// - logger (*zap.Logger): Logger for observability.
func register(workers []worker.Worker, logger *zap.Logger) {
	// Initialize the activities struct with the logger.
	a := &activities{
		logger: logger.With(zap.String("component", "notification-activities")),
	}

	// Register the activities with each worker.
	for _, w := range workers {
		w.RegisterActivity(a)
	}
}