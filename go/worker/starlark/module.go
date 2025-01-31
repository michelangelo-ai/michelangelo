package starlark

import (
	"github.com/cadence-workflow/starlark-worker/cadstar"
	"github.com/cadence-workflow/starlark-worker/plugin"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/ray"
	"go.uber.org/cadence/worker"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Invoke(register),
)

func register(workers []worker.Worker) {

	plugins := plugin.Registry
	plugins[ray.Plugin.ID()] = ray.Plugin

	service := &cadstar.Service{
		Plugins: plugins,
	}
	for _, w := range workers {
		service.Register(w)
	}
}
