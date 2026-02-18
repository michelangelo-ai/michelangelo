package starlark

import (
	"fmt"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/cachedoutput"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/model"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/storage"
)

// RegisterStoragePlugin adds the storage plugin to the plugin registry.
func RegisterStoragePlugin(registry map[string]service.IPlugin) {
	registry[storage.Plugin.ID()] = storage.Plugin
}

// RegisterCachedOutputPlugin adds the cachedoutput plugin to the plugin registry.
func RegisterCachedOutputPlugin(registry map[string]service.IPlugin) {
	registry[cachedoutput.Plugin.ID()] = cachedoutput.Plugin
}

// RegisterModelPlugin adds the model plugin to the plugin registry.
func RegisterModelPlugin(registry map[string]service.IPlugin) {
	registry[model.Plugin.ID()] = model.Plugin
}

// CreateStarlarkService creates the starlark service with all registered plugins.
func CreateStarlarkService(registry map[string]service.IPlugin, workers []worker.Worker, backend service.BackendType) error {
	if len(workers) == 0 {
		return fmt.Errorf("no workers provided")
	}

	workerService, err := service.NewService(registry, "", backend)
	if err != nil {
		return err
	}
	for _, w := range workers {
		workerService.Register(w)
	}

	return nil
}

var Module = fx.Options(
	fx.Invoke(RegisterStoragePlugin),
	fx.Invoke(RegisterCachedOutputPlugin),
	fx.Invoke(RegisterModelPlugin),
	fx.Invoke(CreateStarlarkService),
)
