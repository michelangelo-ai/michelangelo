package pipelinerun

import (
	"go.uber.org/config"
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/notification"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/plugin"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"go.uber.org/zap"
)

const configKey = "pipelineRun"

var Module = fx.Options(
	plugin.Module,
	notification.Module,
	fx.Invoke(registerMetrics),
	fx.Provide(newConfig),
	fx.Invoke(register),
)

func newConfig(provider config.Provider) (Config, error) {
	cfg := Config{}
	err := provider.Get(configKey).Populate(&cfg)
	return cfg, err
}

func registerMetrics() {
	RegisterPipelineRunMetrics()
}

func register(
	mgr manager.Manager,
	apiHandlerFactory apiHandler.Factory,
	logger *zap.Logger,
	p *plugin.Plugin,
	workflowClient clientInterface.WorkflowClient,
	notifier *notification.PipelineRunNotifier,
	cfg Config,
	metadataStorageConfig storage.MetadataStorageConfig,
) error {
	return NewReconciler(p, workflowClient, logger, apiHandlerFactory, notifier, cfg, metadataStorageConfig).Register(mgr)
}
