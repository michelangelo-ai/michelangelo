package starlark

import (
	"go.starlark.net/starlark"
	"go.uber.org/fx"

	"go.temporal.io/sdk/worker"   // Temporal's worker package
	"go.temporal.io/sdk/workflow" // Temporal's worker package

	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/cadstar"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/plugin"
	"github.com/michelangelo-ai/michelangelo/go/temporalworker/plugins/ray"
	"github.com/michelangelo-ai/michelangelo/go/temporalworker/plugins/storage"
)

var Module = fx.Options(
	fx.Invoke(register), // Register function to register workers
)

// register function to register activities with Temporal workers
func register(worker worker.Worker) error {

	// Get the plugins registry and append plugins (Ray, Storage)
	plugins := plugin.Registry
	plugins = append(plugins, ray.Plugin)
	plugins = append(plugins, storage.Plugin)

	service := &cadstar.Service{
		Plugins: plugins,
	}
	workflows := &Workflows{
		service: service,
	}
	worker.RegisterWorkflowWithOptions(workflows.Run, workflow.RegisterOptions{Name: "starlark"})

	return nil
}

// Workflows doc
type Workflows struct {
	service *cadstar.Service
}

// Run doc
func (r *Workflows) Run(
	ctx workflow.Context,
	tar []byte,
	path string,
	function string,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
	environ *starlark.Dict,
) (
	res starlark.Value,
	err error,
) {
	return r.service.Run(ctx, tar, path, function, args, kwargs, environ)
}
