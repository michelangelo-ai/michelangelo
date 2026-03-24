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

type TemporalClientIn struct {
	fx.In
	Config    baseconfig.WorkflowClientConfig
	TLSConfig *tls.Config `optional:"true"`
}

type TemporalClientOut struct {
	fx.Out
	TemporalClient clientInterface.WorkflowClient
}

var _ clientInterface.WorkflowClient = &TemporalClient{}

var Module = fx.Options(
	fx.Provide(NewTemporalClient),
)

// NewTemporalClient creates a new TemporalClient
func NewTemporalClient(in TemporalClientIn) (TemporalClientOut, error) {
	defaultTemporalClientFactory := workflowfx.DefaultTemporalClientFactory{}
	opts := temporalClient.Options{
		HostPort:      in.Config.Host,
		Namespace:     in.Config.Domain,
		DataConverter: temporal.DataConverter{}, // using temporal.DataConverter{} from the starlark-worker package since it supports starlark types
	}

	// Add TLS connection options if UseTLS is enabled
	if in.Config.UseTLS {
		var tlsConfig *tls.Config
		if in.TLSConfig != nil {
			tlsConfig = in.TLSConfig
		} else {
			// Default to empty TLS configuration if none provided
			tlsConfig = &tls.Config{}
		}

		opts.ConnectionOptions = temporalClient.ConnectionOptions{
			TLS: tlsConfig,
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
			Domain:   in.Config.Domain,
		},
	}, nil
}
