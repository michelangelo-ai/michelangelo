package pipeline

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
)

var (
	// Module is the Uber FX module for the Pipeline controller.
	//
	// It provides dependency injection for the controller by invoking the
	// register function, which sets up the Pipeline reconciler with the
	// controller-runtime manager.
	//
	// To use this module, include it in your FX application options:
	//   fx.New(
	//       pipeline.Module,
	//       // other modules...
	//   )
	Module = fx.Options(
		fx.Invoke(registerMetrics),
		fx.Invoke(register),
	)
)

// registerMetrics registers pipeline metrics with Prometheus
// This is invoked once during application initialization
func registerMetrics() {
	RegisterPipelineMetrics()
}

// register initializes and registers the Pipeline controller with the manager.
//
// This function is automatically invoked by the FX framework when the Module
// is loaded. It creates a new Reconciler with the provided dependencies and
// registers it with the controller-runtime manager to watch Pipeline resources.
//
// Dependencies are injected by FX:
//   - mgr: The controller-runtime manager for registering the controller
//   - env: Environment context for runtime configuration
//   - apiHandlerFactory: Factory for creating API handlers
//   - logger: Structured logger for the controller
//
// Returns an error if controller registration fails.
func register(
	mgr manager.Manager,
	env env.Context,
	apiHandlerFactory apiHandler.Factory,
	logger *zap.Logger,
) error {
	return (&Reconciler{
		env:               env,
		apiHandlerFactory: apiHandlerFactory,
		logger:            logger,
	}).Register(mgr)
}
