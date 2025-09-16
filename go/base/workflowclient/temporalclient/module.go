package temporalclient

import (
	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	workflowfx "github.com/michelangelo-ai/michelangelo/go/worker/workflowfx"
	"github.com/cadence-workflow/starlark-worker/temporal"
	temporalClient "go.temporal.io/sdk/client"
	"go.uber.org/fx"
)

type TemporalClientOut struct {
	fx.Out
	TemporalClient clientInterface.WorkflowClient
}

var Module = fx.Options(
	fx.Provide(NewTemporalClient),
)

// NewTemporalClient creates a new TemporalClient
func NewTemporalClient(config baseconfig.WorkflowClientConfig) (TemporalClientOut, error) {
	defaultTemporalClientFactory := workflowfx.DefaultTemporalClientFactory{}
	client, err := defaultTemporalClientFactory.NewTemporalClient(temporalClient.Options{
		HostPort:      config.Host,
		Namespace:     config.Domain,
		DataConverter: temporal.DataConverter{},
	})
	if err != nil {
		return TemporalClientOut{}, err
	}
	return TemporalClientOut{
		TemporalClient: &TemporalClient{
			Client:   client,
			Provider: "temporal",
		},
	}, nil
}
