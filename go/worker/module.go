package worker

import (
	"github.com/michelangelo-ai/michelangelo/go/worker/activities"
	"github.com/michelangelo-ai/michelangelo/go/worker/enginefx"
	"github.com/michelangelo-ai/michelangelo/go/worker/starlark"
	"github.com/michelangelo-ai/michelangelo/go/worker/workflows"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Provide(getYARPCClients),
	enginefx.Module,
	activities.Module,
	workflows.Module,
	starlark.Module,
)
