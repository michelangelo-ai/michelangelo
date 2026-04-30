package clientfactory

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/secrets"
)

// Module wires the ClientFactory into the fx graph.
var Module = fx.Options(
	fx.Provide(newClientFactory),
)

func newClientFactory(kubeClient client.Client, logger *zap.Logger) ClientFactory {
	return NewRemoteClientFactory(
		secrets.NewProvider(kubeClient),
		kubeClient.Scheme(),
		logger,
	)
}
