package s3

import (
	"go.uber.org/cadence/worker"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Provide(newConfig),
	fx.Invoke(register),
)

func register(config Config, workers []worker.Worker) error {
	a := &activities{
		config: &config,
	}
	for _, w := range workers {
		w.RegisterActivity(a)
	}
	return nil
}
