package worker

import (
	"fmt"

	"github.com/cadence-workflow/starlark-worker/plugin"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/worker"
	"github.com/michelangelo-ai/michelangelo/go/cauldron/worker/http"
	"github.com/michelangelo-ai/michelangelo/go/cauldron/worker/plugins/rayhttp"
	"github.com/michelangelo-ai/michelangelo/go/cauldron/worker/plugins/sparkhttp"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Invoke(register),
	http.Module,
)

func register(workers []worker.Worker, backend service.BackendType) error {

	if len(workers) == 0 {
		return fmt.Errorf("no workers provided")
	}

	plugins := plugin.Registry
	plugins[rayhttp.Plugin.ID()] = rayhttp.Plugin
	plugins[sparkhttp.Plugin.ID()] = sparkhttp.Plugin

	workerService, err := service.NewService(plugins, "", backend)
	if err != nil {
		return err
	}
	for _, w := range workers {
		workerService.Register(w)
	}

	return nil
}
