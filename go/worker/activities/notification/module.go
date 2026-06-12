package notification

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/fx"
)

// Module provides FX dependency injection for notification activities.
var Module = fx.Options(
	fx.Invoke(register),
)

func register(workers []worker.Worker) {
	for _, w := range workers {
		w.RegisterActivity(SendMessageToEmailActivity)
		w.RegisterActivity(SendMessageToSlackActivity)
	}
}
