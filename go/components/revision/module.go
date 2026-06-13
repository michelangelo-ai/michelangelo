package revision

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
)

var (
	// Module is the Uber FX module for the Revision controller.
	//
	// Handlers for new entity types are registered via FX by providing them
	// tagged with the "revision-handler" group:
	//
	//   fx.Provide(fx.Annotate(NewMyHandler, fx.As(new(revision.Handler)), fx.ResultTags(`group:"revision-handler"`)))
	Module = fx.Options(
		fx.Invoke(register),
	)
)

type registerParams struct {
	fx.In

	Mgr               manager.Manager
	APIHandlerFactory apiHandler.Factory
	Logger            *zap.Logger
	// Handlers is the set of entity-type-specific revision lifecycle handlers.
	Handlers []Handler `group:"revision-handler"`
}

func register(p registerParams) error {
	return NewReconciler(p.APIHandlerFactory, p.Logger, p.Handlers).Register(p.Mgr)
}
