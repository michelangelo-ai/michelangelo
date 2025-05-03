package starlark

import (
	"fmt"

	"github.com/cadence-workflow/starlark-worker/plugin"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/worker"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/ray"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/spark"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/storage"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Invoke(register),
)

func register(workers []worker.Worker, backend service.BackendType) error {

	if len(workers) == 0 {
		return fmt.Errorf("no workers provided")
	}

	plugins := plugin.Registry
	plugins[ray.Plugin.ID()] = ray.Plugin
	plugins[spark.Plugin.ID()] = spark.Plugin
	plugins[storage.Plugin.ID()] = storage.Plugin

	workerService, err := service.NewService(plugins, "", backend)
	if err != nil {
		return err
	}
	for _, w := range workers {
		workerService.Register(w)
	}

	return nil
}
