package endpointregistry

import (
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var Module = fx.Options(
	fx.Provide(newEndpointRegistry),
	fx.Provide(newDynamicClient),
)

// newEndpointRegistry creates a new EndpointRegistry using the Istio implementation.
func newEndpointRegistry(dynamicClient dynamic.Interface, kubeClient client.Client, logger *zap.Logger) EndpointRegistry {
	return NewIstioEndpointRegistry(dynamicClient, kubeClient, logger)
}

// newDynamicClient creates a Kubernetes dynamic client for working with unstructured resources.
func newDynamicClient() (dynamic.Interface, error) {
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get rest config: %w", err)
	}
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}
	return dynamicClient, nil
}
