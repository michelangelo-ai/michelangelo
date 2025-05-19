package starlark

import (
	"fmt"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/cachedoutput"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/deployment"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/ray"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/spark"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/storage"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/uapi"
)

// RegisterStoragePlugin adds the storage plugin to the plugin registry.
func RegisterStoragePlugin(registry map[string]service.IPlugin) {
	registry[storage.Plugin.ID()] = storage.Plugin
}

// RegisterCachedOutputPlugin adds the cachedoutput plugin to the plugin registry.
func RegisterCachedOutputPlugin(registry map[string]service.IPlugin) {
	registry[cachedoutput.Plugin.ID()] = cachedoutput.Plugin
}

// RegisterRayPlugin adds the ray plugin to the plugin registry.
func RegisterRayPlugin(registry map[string]service.IPlugin) {
	registry[ray.Plugin.ID()] = ray.Plugin
}

// RegisterSparkPlugin adds the spark plugin to the plugin registry.
func RegisterSparkPlugin(registry map[string]service.IPlugin) {
	registry[spark.Plugin.ID()] = spark.Plugin
}

// RegisterUAPIPlugin adds the uapi plugin to the plugin registry.
func RegisterUAPIPlugin(registry map[string]service.IPlugin) {
	registry[uapi.Plugin.ID()] = uapi.Plugin
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
	fx.Invoke(RegisterRayPlugin),
	fx.Invoke(RegisterSparkPlugin),
	fx.Invoke(RegisterUAPIPlugin),
	fx.Invoke(CreateStarlarkService),
)
