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
	// It wires the Revision reconciler and dispatches lifecycle events to
	// Handlers registered under the "revision-handler" FX group. To add a
	// handler for a new entity type, provide it tagged with that group:
	//
	//   fx.Provide(fx.Annotate(NewMyHandler, fx.As(new(revision.Handler)), fx.ResultTags(`group:"revision-handler"`)))
	//
	// To use this module, include it in your FX application options:
	//   fx.New(
	//       revision.Module,
	//       // other modules...
	//   )
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
	// Callers register handlers via fx.Annotate with group:"revision-handler".
	Handlers []Handler `group:"revision-handler"`
}

func register(p registerParams) error {
	return NewReconciler(p.APIHandlerFactory, p.Logger, p.Handlers).Register(p.Mgr)
}
