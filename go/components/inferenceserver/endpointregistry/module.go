package endpointregistry

import (
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RegistryType represents the type of endpoint registry to use.
type RegistryType string

const (
	// RegistryTypeIstio uses Istio ServiceEntry + ExternalName Service for endpoint registration.
	RegistryTypeIstio RegistryType = "istio"
	// RegistryTypeMCS uses Kubernetes Multi-Cluster Services (ServiceExport/ServiceImport).
	RegistryTypeMCS RegistryType = "mcs"

	// EnvEndpointRegistryType is the environment variable to configure the registry type.
	EnvEndpointRegistryType = "ENDPOINT_REGISTRY_TYPE"
)

var Module = fx.Options(
	fx.Provide(newEndpointRegistry),
	fx.Provide(newDynamicClient),
)

// newEndpointRegistry creates a new EndpointRegistry based on configuration.
// The registry type can be configured via the ENDPOINT_REGISTRY_TYPE environment variable.
// Supported values: "istio" (default), "mcs"
func newEndpointRegistry(dynamicClient dynamic.Interface, kubeClient client.Client, logger *zap.Logger) EndpointRegistry {
	return NewMCSEndpointRegistry(dynamicClient, kubeClient, logger)
	// return NewIstioEndpointRegistry(dynamicClient, kubeClient, logger)
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
