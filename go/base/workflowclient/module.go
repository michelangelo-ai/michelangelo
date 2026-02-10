package workflowclient

import (
	"go.uber.org/fx"

	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/workflowclient/cadenceclient"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"github.com/michelangelo-ai/michelangelo/go/base/workflowclient/temporalclient"
)

var Module = fx.Options(
	fx.Provide(provide),
)

func provide(config baseconfig.WorkflowClientConfig) (clientInterface.WorkflowClient, error) {
	if config.Provider == "Temporal" {
		out, err := temporalclient.NewTemporalClient(config)
		if err != nil {
			return nil, err
		}
		return out.TemporalClient, nil
	}
	out, err := cadenceclient.NewCadenceClient(config)
	if err != nil {
		return nil, err
	}
	return out.CadenceClient, nil
}
