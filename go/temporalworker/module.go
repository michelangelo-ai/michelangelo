package worker

import (
	"github.com/michelangelo-ai/michelangelo/go/temporalworker/activities"
	"github.com/michelangelo-ai/michelangelo/go/temporalworker/starlark"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Provide(initWorker),
	fx.Provide(getYARPCClients),
	activities.Module,
	starlark.Module,
)
