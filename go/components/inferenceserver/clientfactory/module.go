package clientfactory

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/secrets"
)

var Module = fx.Options(
	fx.Provide(newClientFactory),
)

// newClientFactory creates a new ClientFactory with the configured control plane cluster ID
func newClientFactory(kubeClient client.Client, logger *zap.Logger) ClientFactory {
	sp := secrets.NewProvider(kubeClient)
	return NewClientFactory(kubeClient, sp, kubeClient.Scheme(), logger)
}
