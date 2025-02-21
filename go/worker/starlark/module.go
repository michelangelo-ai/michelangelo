package starlark

import (
	"fmt"

	"github.com/uber-go/tally"
	"go.uber.org/cadence/worker"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/cadence-workflow/starlark-worker/cadstar"
	"github.com/cadence-workflow/starlark-worker/plugin"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/ray"
)

var Module = fx.Options(
	fx.Invoke(register),
	fx.Provide(getDataConvertor),
)

func register(workers []worker.Worker) error {

	if len(workers) == 0 {
		return fmt.Errorf("no workers provided")
	}

	plugins := plugin.Registry
	plugins[ray.Plugin.ID()] = ray.Plugin

	service := &cadstar.Service{
		Plugins: plugins,
	}
	for _, w := range workers {
		service.Register(w)
	}

	return nil
}

func getDataConvertor(logger *zap.Logger) worker.Options {
	metrics := tally.NoopScope
	return worker.Options{
		MetricsScope:  metrics,
		Logger:        logger,
		DataConverter: &cadstar.DataConverter{Logger: logger},
	}
}
