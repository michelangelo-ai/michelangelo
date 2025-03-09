package main

import (
	"context"
	"github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/zapfx"
	temporalworker "github.com/michelangelo-ai/michelangelo/go/temporalworker"
	"go.uber.org/zap"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.uber.org/fx"
)

func main() {
	fx.New(options()).Run()
}

func options() fx.Option {
	return fx.Options(
		temporalworker.Module,
		env.Module,
		config.Module,
		zapfx.Module,
		fx.Invoke(startWorker),
	)
}

// startWorker manages Temporal worker startup and shutdown using fx.Lifecycle.
func startWorker(client client.Client, lc fx.Lifecycle, tworker worker.Worker, logger *zap.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			logger.Info("Starting Temporal Client...")

			go func(w worker.Worker) {
				if err := w.Run(worker.InterruptCh()); err != nil {
					logger.Fatal("Worker failed", zap.Error(err))
				}
			}(tworker)
			return nil
		},
		OnStop: func(context.Context) error {
			logger.Info("Stopping Temporal Client...")
			client.Close()
			return nil
		},
	})
}
