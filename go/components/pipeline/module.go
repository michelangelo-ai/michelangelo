package pipeline

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/revision"
)

var (
	// Module is the Uber FX module for the Pipeline controller.
	//
	// Provides a revision.Manager backed by the API handler; consumers can
	// swap the implementation via fx.Decorate.
	Module = fx.Options(
		fx.Provide(revision.NewManager),
		fx.Invoke(registerMetrics),
		fx.Invoke(register),
	)
)

func registerMetrics() {
	RegisterPipelineMetrics()
}

// register initializes and registers the Pipeline controller with the manager.
func register(
	mgr manager.Manager,
	env env.Context,
	apiHandlerFactory apiHandler.Factory,
	logger *zap.Logger,
	revisionManager revision.Manager,
) error {
	return (&Reconciler{
		env:               env,
		apiHandlerFactory: apiHandlerFactory,
		logger:            logger,
		revisionManager:   revisionManager,
	}).Register(mgr)
}
