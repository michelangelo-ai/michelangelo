package temporalclient

import (
	"crypto/tls"

	"github.com/cadence-workflow/starlark-worker/temporal"
	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	workflowfx "github.com/michelangelo-ai/michelangelo/go/worker/workflowfx"
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
	opts := temporalClient.Options{
		HostPort:      config.Host,
		Namespace:     config.Domain,
		DataConverter: temporal.DataConverter{}, // using temporal.DataConverter{} from the starlark-worker package since it supports starlark types
	}

	// Add TLS connection options if UseTLS is enabled
	if config.UseTLS {
		opts.ConnectionOptions = temporalClient.ConnectionOptions{
			TLS: &tls.Config{},
		}
	}

	client, err := defaultTemporalClientFactory.NewTemporalClient(opts)
	if err != nil {
		return TemporalClientOut{}, err
	}
	return TemporalClientOut{
		TemporalClient: &TemporalClient{
			Client:   client,
			Provider: "temporal",
			Domain:   config.Domain,
		},
	}, nil
}
