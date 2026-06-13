package pipeline

import (
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/revision"
)

const configKey = "pipeline"

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
		fx.Provide(newConfig),
		fx.Invoke(registerMetrics),
		fx.Invoke(register),
	)
)

func newConfig(provider config.Provider) (Config, error) {
	cfg := Config{}
	err := provider.Get(configKey).Populate(&cfg)
	return cfg, err
}

// registerMetrics registers pipeline metrics with Prometheus
// This is invoked once during application initialization
func registerMetrics() {
	RegisterPipelineMetrics()
}

type registerParams struct {
	fx.In

	Mgr               manager.Manager
	Env               env.Context
	APIHandlerFactory apiHandler.Factory
	Logger            *zap.Logger
	// RevisionManager is optional; when absent Register constructs one from
	// the API handler. Internal callers can inject a store-backed implementation.
	RevisionManager revision.Manager `optional:"true"`
	Config          Config
}

func register(p registerParams) error {
	return NewReconciler(p.Env, p.APIHandlerFactory, p.Logger, p.RevisionManager, p.Config).Register(p.Mgr)
}
