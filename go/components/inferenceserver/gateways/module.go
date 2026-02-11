package gateways

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpointregistry"
)

var Module = fx.Options(
	fx.Provide(newInferenceServerGateway),
)

// newInferenceServerGateway creates a new inference server gateway with clients
func newInferenceServerGateway(logger *zap.Logger, kubeClient client.Client, clientFactory clientfactory.ClientFactory, modelConfigMapProvider configmap.ModelConfigMapProvider, endpointRegistry endpointregistry.EndpointRegistry) Gateway {
	return NewGatewayWithClients(Params{
		Logger:                 logger,
		KubeClient:             kubeClient,
		ClientFactory:          clientFactory,
		ModelConfigMapProvider: modelConfigMapProvider,
		EndpointRegistry:       endpointRegistry,
	})
}
