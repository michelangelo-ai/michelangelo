package cadenceclient

import (
	"go.uber.org/fx"

	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	workflowfx "github.com/michelangelo-ai/michelangelo/go/worker/workflowfx"
	cadenceClient "go.uber.org/cadence/client"
)

var Module = fx.Options(
	fx.Provide(NewCadenceClient),
)

type CadenceClientOut struct {
	fx.Out
	CadenceClient clientInterface.WorkflowClient
}

func NewCadenceClient(config baseconfig.WorkflowClientConfig) (CadenceClientOut, error) {
	defaultCadenceClientFactory := workflowfx.DefaultCadenceClientFactory{}
	workflowFxConfig := workflowfx.Config{
		Host:      config.Host,
		Transport: config.Transport,
		Client: workflowfx.ClientConfig{
			Domain: config.Domain,
		},
	}
	workflowServiceClient, err := defaultCadenceClientFactory.NewCadenceClient(workflowFxConfig)
	if err != nil {
		return CadenceClientOut{}, err
	}
	client := cadenceClient.NewClient(workflowServiceClient, config.Domain, &cadenceClient.Options{})

	return CadenceClientOut{
		CadenceClient: &CadenceClient{
			Client:   client,
			Provider: "cadence",
			Domain:   config.Domain,
		},
	}, nil
}
