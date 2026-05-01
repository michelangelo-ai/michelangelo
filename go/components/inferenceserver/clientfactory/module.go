package clientfactory

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/secrets"
)

var Module = fx.Options(
	fx.Provide(newRemoteClientFactory),
)

// newRemoteClientFactory creates a new RemoteClientFactory with the configured control plane cluster ID
func newRemoteClientFactory(kubeClient client.Client, logger *zap.Logger) ClientFactory {
	return NewRemoteClientFactory(secrets.NewProvider(kubeClient), kubeClient.Scheme(), logger)
}
