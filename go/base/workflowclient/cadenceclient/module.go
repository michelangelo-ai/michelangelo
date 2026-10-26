package cadenceclient

import (
	"crypto/tls"

	"go.uber.org/fx"
	"go.uber.org/zap"

	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	workflowfx "github.com/michelangelo-ai/michelangelo/go/worker/workflowfx"
	cadenceClient "go.uber.org/cadence/client"
	"go.uber.org/cadence/encoded"
)

var Module = fx.Options(
	fx.Provide(NewCadenceClient),
)

type CadenceClientIn struct {
	fx.In
	Config    baseconfig.WorkflowClientConfig
	Logger    *zap.Logger `optional:"true"`
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

	// Set UseTLS flag and prepare TLS configuration
	workflowFxConfig.UseTLS = in.Config.UseTLS
	var tlsConfig *tls.Config
	if in.Config.UseTLS {
		if in.TLSConfig != nil {
			tlsConfig = in.TLSConfig
		} else {
			// Default to empty TLS configuration if none provided
			tlsConfig = &tls.Config{}
		}
	}

	workflowServiceClient, err := defaultCadenceClientFactory.NewCadenceClient(workflowFxConfig, tlsConfig)
	if err != nil {
		return CadenceClientOut{}, err
	}
	// The same converter is handed to the client and kept on CadenceClient: schedule
	// actions carry pre-encoded input, which must match what a direct start would encode.
	dataConverter := encoded.GetDefaultDataConverter()
	client := cadenceClient.NewClient(workflowServiceClient, in.Config.Domain, &cadenceClient.Options{
		DataConverter: dataConverter,
	})

	return CadenceClientOut{
		CadenceClient: &CadenceClient{
			Client:        client,
			Provider:      "cadence",
			Domain:        in.Config.Domain,
			Logger:        in.Logger,
			DataConverter: dataConverter,
		},
	}, nil
}
