package buildkite

import (
	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/fx"
)

// Module registers Buildkite activities on every worker. Credentials are read
// only in the worker process, never from workflow arguments or history.
var Module = fx.Options(
	fx.Provide(newClient),
	fx.Invoke(register),
)

func register(workers []worker.Worker, client *client) {
	instance := &activities{client: client}
	for _, w := range workers {
		w.RegisterActivity(instance)
	}
}
