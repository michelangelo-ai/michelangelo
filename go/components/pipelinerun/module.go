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
	// Module is the Uber FX module for the PipelineRun controller.
	//
	// It provides dependency injection for the controller and its plugin,
	// which contains actors for different pipeline execution stages. The module
	// automatically registers the controller with the controller-runtime manager.
	//
	// To use this module, include it in your FX application:
	//   fx.New(
	//       pipelinerun.Module,
	//       // other modules...
	//   )
	Module = fx.Options(
		plugin.Module,
		fx.Invoke(register),
	)
)

// register initializes and registers the PipelineRun controller with the manager.
//
// This function is automatically invoked by the FX framework when the Module
// is loaded. It creates a new Reconciler with the plugin and dependencies,
// then registers it with the controller-runtime manager to watch PipelineRun resources.
//
// Dependencies are injected by FX:
//   - mgr: The controller-runtime manager for registering the controller
//   - env: Environment context for runtime configuration
//   - apiHandlerFactory: Factory for creating API handlers
//   - logger: Structured logger for the controller
//   - plugin: Contains ConditionActors for pipeline execution stages
//
// Returns an error if controller registration fails.
func register(
	mgr manager.Manager,
	env env.Context,
	apiHandlerFactory apiHandler.Factory,
	logger *zap.Logger,
	plugin *plugin.Plugin,
) error {
	return NewReconciler(plugin, logger, apiHandlerFactory).Register(mgr)
}
