package pipeline

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
)


var (
	// Module FX
	Module = fx.Options(
		fx.Invoke(register),
	)
)

func register(
	mgr manager.Manager,
	env env.Context,
	apiHandlerFactory apiHandler.Factory,
	logger *zap.Logger,
) error {
	return (&Reconciler{
		env:    env,
		apiHandlerFactory: apiHandlerFactory,
		logger: logger,
	}).Register(mgr)
}