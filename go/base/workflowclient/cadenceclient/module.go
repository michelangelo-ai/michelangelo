package cadenceclient

import (
	"crypto/tls"

	"go.uber.org/fx"

	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	workflowfx "github.com/michelangelo-ai/michelangelo/go/worker/workflowfx"
	cadenceClient "go.uber.org/cadence/client"
)

var Module = fx.Options(
	fx.Provide(NewCadenceClient),
)

type CadenceClientIn struct {
	fx.In
	Config    baseconfig.WorkflowClientConfig
	TLSConfig *tls.Config `optional:"true"`
}

type CadenceClientOut struct {
	fx.Out
	CadenceClient clientInterface.WorkflowClient
}

func NewCadenceClient(in CadenceClientIn) (CadenceClientOut, error) {
	defaultCadenceClientFactory := workflowfx.DefaultCadenceClientFactory{}
	workflowFxConfig := workflowfx.Config{
		Host:      in.Config.Host,
		Transport: in.Config.Transport,
		Client: workflowfx.ClientConfig{
			Domain: in.Config.Domain,
		},
	}

	// Add TLS configuration if UseTLS is enabled
	if in.Config.UseTLS {
		var tlsConfig *tls.Config
		if in.TLSConfig != nil {
			tlsConfig = in.TLSConfig
		} else {
			// Default to empty TLS configuration if none provided
			tlsConfig = &tls.Config{}
		}
		workflowFxConfig.TLSConfig = tlsConfig
	}

	workflowServiceClient, err := defaultCadenceClientFactory.NewCadenceClient(workflowFxConfig)
	if err != nil {
		return CadenceClientOut{}, err
	}
	client := cadenceClient.NewClient(workflowServiceClient, in.Config.Domain, &cadenceClient.Options{})

	return CadenceClientOut{
		CadenceClient: &CadenceClient{
			Client:   client,
			Provider: "cadence",
			Domain:   in.Config.Domain,
		},
	}, nil
}
