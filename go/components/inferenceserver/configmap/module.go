package configmap

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
)

// Module provides the configmap module
var Module = fx.Options(
	fx.Provide(newModelConfigMapProvider),
)

// newModelConfigMapProvider creates a new model config map provider
func newModelConfigMapProvider(kubeClient client.Client, clientFactory clientfactory.ClientFactory, logger *zap.Logger) ModelConfigMapProvider {
	return NewDefaultModelConfigMapProvider(kubeClient, clientFactory, logger)
}
