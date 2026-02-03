package endpointregistry

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
)

var Module = fx.Options(
	fx.Provide(newIstioEndpointRegistry),
)

// newEndpointRegistry creates a new EndpointRegistry using the Istio implementation.
func newIstioEndpointRegistry(dynamicClient dynamic.Interface, kubeClient client.Client, clientFactory clientfactory.ClientFactory, logger *zap.Logger) EndpointRegistry {
	return NewIstioEndpointRegistry(dynamicClient, kubeClient, clientFactory, logger)
}
