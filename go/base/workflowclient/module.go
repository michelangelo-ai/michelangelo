package workflowclient

import (
	"crypto/tls"

	"go.uber.org/fx"

	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/workflowclient/cadenceclient"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"github.com/michelangelo-ai/michelangelo/go/base/workflowclient/temporalclient"
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
	if in.Config.Provider == "Temporal" {
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
