package endpointregistry

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var Module = fx.Options(
	fx.Provide(newEndpointRegistry),
)

// newEndpointRegistry creates a new EndpointRegistry using the Istio implementation.
func newEndpointRegistry(dynamicClient dynamic.Interface, kubeClient client.Client, logger *zap.Logger) EndpointRegistry {
	return NewIstioEndpointRegistry(dynamicClient, kubeClient, logger)
}
