package workflowclient

import (
	"crypto/tls"

	"go.uber.org/fx"

	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/workflowclient/cadenceclient"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"github.com/michelangelo-ai/michelangelo/go/base/workflowclient/temporalclient"
)

const (
	// providerTemporal is the config value that selects the Temporal workflow engine.
	// Any other value (including empty string) selects Cadence. This constant
	// prevents silent misconfiguration from typos in the Provider field.
	providerTemporal = "Temporal"
)

var Module = fx.Options(
	fx.Provide(provide),
)

type ProvideIn struct {
	fx.In
	Config    baseconfig.WorkflowClientConfig
	TLSConfig *tls.Config `optional:"true"`
}

func provide(in ProvideIn) (clientInterface.WorkflowClient, error) {
	if in.Config.Provider == providerTemporal {
		temporalIn := temporalclient.TemporalClientIn{
			Config:    in.Config,
			TLSConfig: in.TLSConfig,
		}
		out, err := temporalclient.NewTemporalClient(temporalIn)
		if err != nil {
			return nil, err
		}
		return out.TemporalClient, nil
	}
	cadenceIn := cadenceclient.CadenceClientIn{
		Config:    in.Config,
		TLSConfig: in.TLSConfig,
	}
	out, err := cadenceclient.NewCadenceClient(cadenceIn)
	if err != nil {
		return nil, err
	}
	return out.CadenceClient, nil
}
