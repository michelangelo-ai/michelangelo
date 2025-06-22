package pipelinerun

import (
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/plugin"
	"go.uber.org/zap"
)

var (
	// Module FX
	Module = fx.Options(
		plugin.Module,
		fx.Invoke(register),
	)
)

func register(
	mgr manager.Manager,
	env env.Context,
	apiHandlerFactory apiHandler.Factory,
	logger *zap.Logger,
	plugin *plugin.Plugin,
) error {
	return NewReconciler(plugin, logger, apiHandlerFactory).Register(mgr)
}
