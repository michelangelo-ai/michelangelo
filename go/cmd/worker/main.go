package main

import (
	"github.com/michelangelo-ai/michelangelo/go/base/cadencefx"
	"github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/zapfx"
	"github.com/michelangelo-ai/michelangelo/go/worker"
	"go.uber.org/fx"
)

func main() {
	fx.New(options()).Run()
}

func options() fx.Option {
	return fx.Options(
		worker.Module,
		env.Module,
		config.Module,
		zapfx.Module,
		cadencefx.Module,
	)
}
