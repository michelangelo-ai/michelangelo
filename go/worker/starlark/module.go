package starlark

import (
	"fmt"

	"github.com/cadence-workflow/starlark-worker/cadstar"
	"github.com/cadence-workflow/starlark-worker/plugin"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/ray"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/s3"
	"go.uber.org/cadence/worker"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Invoke(register),
)

func register(workers []worker.Worker) error {

	if len(workers) == 0 {
		return fmt.Errorf("no workers provided")
	}

	plugins := plugin.Registry
	plugins[ray.Plugin.ID()] = ray.Plugin
	plugins[s3.Plugin.ID()] = s3.Plugin

	service := &cadstar.Service{
		Plugins: plugins,
	}
	for _, w := range workers {
		service.Register(w)
	}

	return nil
}
