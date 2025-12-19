package gateways

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpointregistry"
)

var Module = fx.Options(
	fx.Provide(newInferenceServerGateway),
)

// newInferenceServerGateway creates a new inference server gateway with clients
func newInferenceServerGateway(kubeClient client.Client, modelConfigMapProvider configmap.ModelConfigMapProvider, endpointRegistry endpointregistry.EndpointRegistry, logger *zap.Logger) Gateway {
	return NewGatewayWithClients(Params{
		Logger:                 logger,
		KubeClient:             kubeClient,
		ModelConfigMapProvider: modelConfigMapProvider,
		EndpointRegistry:       endpointRegistry,
	})
}
