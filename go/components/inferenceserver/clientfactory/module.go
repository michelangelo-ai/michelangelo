package clientfactory

import (
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/secrets"
)

// Module wires the ClientFactory into the fx graph.
var Module = fx.Options(
	fx.Provide(baseconfig.ProvideConfig[secrets.Config](secrets.ConfigKey)),
	fx.Provide(newClientFactory),
)

func newClientFactory(kubeClient client.Client, cfg secrets.Config, logger *zap.Logger) (ClientFactory, error) {
	provider, err := newSecretProvider(kubeClient, cfg)
	if err != nil {
		return nil, err
	}
	return NewRemoteClientFactory(
		provider,
		kubeClient.Scheme(),
		logger,
	), nil
}

// newSecretProvider returns the SecretProvider selected by
// `secrets.provider`: the sample control-plane provider by default, or the
// External Secrets Operator backed provider when configured as "eso".
func newSecretProvider(kubeClient client.Client, cfg secrets.Config) (secrets.SecretProvider, error) {
	switch cfg.Provider {
	case "", secrets.ProviderSample:
		return secrets.NewProvider(kubeClient), nil
	case secrets.ProviderESO:
		return secrets.NewESOProvider(kubeClient, cfg.ESO), nil
	default:
		return nil, fmt.Errorf(
			"unknown secrets.provider %q: must be %q or %q",
			cfg.Provider, secrets.ProviderSample, secrets.ProviderESO)
	}
}
