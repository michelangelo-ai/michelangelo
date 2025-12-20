package endpointregistry

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Istio ServiceEntry constants
	istioNetworkingGroup   = "networking.istio.io"
	istioNetworkingVersion = "v1"
	serviceEntryResource   = "serviceentries"

	// Annotation keys for ServiceEntry
	annotationClusterID   = "michelangelo.ai/cluster-id"
	annotationAPIHost     = "michelangelo.ai/api-host"
	annotationAPIPort     = "michelangelo.ai/api-port"
	annotationTokenSecret = "michelangelo.ai/token-secret"
	annotationCASecret    = "michelangelo.ai/ca-secret"

	// Label keys
	labelInferenceServer = "michelangelo.ai/inference-server"
	labelClusterID       = "michelangelo.ai/cluster-id"
	labelComponent       = "app.kubernetes.io/component"
	labelManagedBy       = "app.kubernetes.io/managed-by"

	// Default ports
	defaultHTTPPort = 80
	defaultGRPCPort = 8001
)

var serviceEntryGVR = schema.GroupVersionResource{
	Group:    istioNetworkingGroup,
	Version:  istioNetworkingVersion,
	Resource: serviceEntryResource,
}

var _ EndpointRegistry = &istioEndpointRegistry{}

// istioEndpointRegistry implements EndpointRegistry using Istio ServiceEntry
// and Kubernetes ExternalName Services.
type istioEndpointRegistry struct {
	dynamicClient dynamic.Interface
	kubeClient    client.Client
	logger        *zap.Logger
}

// NewIstioEndpointRegistry creates a new Istio-based endpoint registry.
func NewIstioEndpointRegistry(dynamicClient dynamic.Interface, kubeClient client.Client, logger *zap.Logger) EndpointRegistry {
	return &istioEndpointRegistry{
		dynamicClient: dynamicClient,
		kubeClient:    kubeClient,
		logger:        logger,
	}
}

// RegisterEndpoint creates a ServiceEntry and ExternalName Service for the cluster endpoint.
func (r *istioEndpointRegistry) RegisterEndpoint(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error {
	logger.Info("Registering cluster endpoint",
		zap.String("inferenceServer", endpoint.InferenceServerName),
		zap.String("namespace", endpoint.Namespace),
		zap.String("clusterID", endpoint.ClusterID),
	)

	// Create ServiceEntry in the control plane
	if err := r.createServiceEntry(ctx, logger, endpoint); err != nil {
		return fmt.Errorf("failed to create ServiceEntry for cluster %s: %w", endpoint.ClusterID, err)
	}

	// Create ExternalName Service for HTTPRoute compatibility
	if err := r.createExternalNameService(ctx, logger, endpoint); err != nil {
		return fmt.Errorf("failed to create ExternalName Service for cluster %s: %w", endpoint.ClusterID, err)
	}

	logger.Info("Successfully registered cluster endpoint",
		zap.String("inferenceServer", endpoint.InferenceServerName),
		zap.String("clusterID", endpoint.ClusterID),
	)
	return nil
}

// UnregisterEndpoint removes the ServiceEntry and ExternalName Service for a cluster.
func (r *istioEndpointRegistry) UnregisterEndpoint(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace, clusterID string) error {
	logger.Info("Unregistering cluster endpoint",
		zap.String("inferenceServer", inferenceServerName),
		zap.String("namespace", namespace),
		zap.String("clusterID", clusterID),
	)

	// Delete ServiceEntry
	serviceEntryName := r.buildServiceEntryName(inferenceServerName, clusterID)
	if err := r.dynamicClient.Resource(serviceEntryGVR).Namespace(namespace).Delete(ctx, serviceEntryName, metav1.DeleteOptions{}); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete ServiceEntry %s: %w", serviceEntryName, err)
		}
		logger.Debug("ServiceEntry not found, already deleted", zap.String("name", serviceEntryName))
	}

	// Delete ExternalName Service
	externalServiceName := r.buildExternalServiceName(inferenceServerName, clusterID)
	externalService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalServiceName,
			Namespace: namespace,
		},
	}
	if err := r.kubeClient.Delete(ctx, externalService); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete ExternalName Service %s: %w", externalServiceName, err)
		}
		logger.Debug("ExternalName Service not found, already deleted", zap.String("name", externalServiceName))
	}

	logger.Info("Successfully unregistered cluster endpoint",
		zap.String("inferenceServer", inferenceServerName),
		zap.String("clusterID", clusterID),
	)
	return nil
}

// GetEndpoints retrieves all registered cluster endpoints for an inference server.
func (r *istioEndpointRegistry) GetEndpoints(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace string) ([]ClusterEndpoint, error) {
	logger.Debug("Getting endpoints for inference server",
		zap.String("inferenceServer", inferenceServerName),
		zap.String("namespace", namespace),
	)

	// List ServiceEntries with label selector
	labelSelector := fmt.Sprintf("%s=%s", labelInferenceServer, inferenceServerName)
	list, err := r.dynamicClient.Resource(serviceEntryGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list ServiceEntries: %w", err)
	}

	endpoints := make([]ClusterEndpoint, 0, len(list.Items))
	for _, item := range list.Items {
		endpoint, err := r.parseServiceEntry(&item)
		if err != nil {
			logger.Warn("Failed to parse ServiceEntry, skipping",
				zap.String("name", item.GetName()),
				zap.Error(err),
			)
			continue
		}
		endpoints = append(endpoints, *endpoint)
	}

	return endpoints, nil
}

// GetEndpoint retrieves a specific cluster endpoint.
func (r *istioEndpointRegistry) GetEndpoint(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace, clusterID string) (*ClusterEndpoint, error) {
	serviceEntryName := r.buildServiceEntryName(inferenceServerName, clusterID)

	item, err := r.dynamicClient.Resource(serviceEntryGVR).Namespace(namespace).Get(ctx, serviceEntryName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get ServiceEntry %s: %w", serviceEntryName, err)
	}

	return r.parseServiceEntry(item)
}

// UpdateEndpoint updates an existing cluster endpoint registration.
func (r *istioEndpointRegistry) UpdateEndpoint(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error {
	logger.Info("Updating cluster endpoint",
		zap.String("inferenceServer", endpoint.InferenceServerName),
		zap.String("clusterID", endpoint.ClusterID),
	)

	// Update ServiceEntry
	if err := r.updateServiceEntry(ctx, logger, endpoint); err != nil {
		return fmt.Errorf("failed to update ServiceEntry for cluster %s: %w", endpoint.ClusterID, err)
	}

	// Update ExternalName Service
	if err := r.updateExternalNameService(ctx, logger, endpoint); err != nil {
		return fmt.Errorf("failed to update ExternalName Service for cluster %s: %w", endpoint.ClusterID, err)
	}

	return nil
}

// createServiceEntry creates an Istio ServiceEntry for the cluster endpoint.
func (r *istioEndpointRegistry) createServiceEntry(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error {
	serviceEntryName := r.buildServiceEntryName(endpoint.InferenceServerName, endpoint.ClusterID)

	// Check if ServiceEntry already exists
	existing, err := r.dynamicClient.Resource(serviceEntryGVR).Namespace(endpoint.Namespace).Get(ctx, serviceEntryName, metav1.GetOptions{})
	if err == nil {
		logger.Info("ServiceEntry already exists, skipping creation", zap.String("name", serviceEntryName))
		// Update if needed
		return r.updateServiceEntrySpec(ctx, existing, endpoint)
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check existing ServiceEntry: %w", err)
	}

	serviceEntry := r.buildServiceEntry(endpoint)
	if _, err := r.dynamicClient.Resource(serviceEntryGVR).Namespace(endpoint.Namespace).Create(ctx, serviceEntry, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create ServiceEntry: %w", err)
	}

	logger.Info("Created ServiceEntry",
		zap.String("name", serviceEntryName),
		zap.String("host", endpoint.ServiceHost),
	)
	return nil
}

// createExternalNameService creates a Kubernetes ExternalName Service that points
// to the ServiceEntry host. This enables HTTPRoute to reference the endpoint.
func (r *istioEndpointRegistry) createExternalNameService(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error {
	serviceName := r.buildExternalServiceName(endpoint.InferenceServerName, endpoint.ClusterID)

	// Check if Service already exists
	existing := &corev1.Service{}
	err := r.kubeClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: endpoint.Namespace}, existing)
	if err == nil {
		logger.Info("ExternalName Service already exists, skipping creation", zap.String("name", serviceName))
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check existing ExternalName Service: %w", err)
	}

	service := r.buildExternalNameService(endpoint)
	if err := r.kubeClient.Create(ctx, service); err != nil {
		return fmt.Errorf("failed to create ExternalName Service: %w", err)
	}

	logger.Info("Created ExternalName Service",
		zap.String("name", serviceName),
		zap.String("externalName", endpoint.ServiceHost),
	)
	return nil
}

// updateServiceEntry updates an existing ServiceEntry.
func (r *istioEndpointRegistry) updateServiceEntry(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error {
	serviceEntryName := r.buildServiceEntryName(endpoint.InferenceServerName, endpoint.ClusterID)

	existing, err := r.dynamicClient.Resource(serviceEntryGVR).Namespace(endpoint.Namespace).Get(ctx, serviceEntryName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Create if not exists
			return r.createServiceEntry(ctx, logger, endpoint)
		}
		return fmt.Errorf("failed to get ServiceEntry: %w", err)
	}

	return r.updateServiceEntrySpec(ctx, existing, endpoint)
}

// updateServiceEntrySpec updates the spec of an existing ServiceEntry.
func (r *istioEndpointRegistry) updateServiceEntrySpec(ctx context.Context, existing *unstructured.Unstructured, endpoint ClusterEndpoint) error {
	newEntry := r.buildServiceEntry(endpoint)

	// Update annotations and spec
	existing.SetAnnotations(newEntry.GetAnnotations())
	if err := unstructured.SetNestedField(existing.Object, newEntry.Object["spec"], "spec"); err != nil {
		return fmt.Errorf("failed to set spec: %w", err)
	}

	if _, err := r.dynamicClient.Resource(serviceEntryGVR).Namespace(endpoint.Namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update ServiceEntry: %w", err)
	}

	return nil
}

// updateExternalNameService updates an existing ExternalName Service.
func (r *istioEndpointRegistry) updateExternalNameService(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error {
	serviceName := r.buildExternalServiceName(endpoint.InferenceServerName, endpoint.ClusterID)

	existing := &corev1.Service{}
	err := r.kubeClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: endpoint.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.createExternalNameService(ctx, logger, endpoint)
		}
		return fmt.Errorf("failed to get ExternalName Service: %w", err)
	}

	// Update external name
	existing.Spec.ExternalName = endpoint.ServiceHost
	if err := r.kubeClient.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update ExternalName Service: %w", err)
	}

	return nil
}

// buildServiceEntry constructs an Istio ServiceEntry for the endpoint.
func (r *istioEndpointRegistry) buildServiceEntry(endpoint ClusterEndpoint) *unstructured.Unstructured {
	serviceEntryName := r.buildServiceEntryName(endpoint.InferenceServerName, endpoint.ClusterID)

	servicePort := endpoint.ServicePort
	if servicePort == 0 {
		servicePort = defaultHTTPPort
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", istioNetworkingGroup, istioNetworkingVersion),
			"kind":       "ServiceEntry",
			"metadata": map[string]interface{}{
				"name":      serviceEntryName,
				"namespace": endpoint.Namespace,
				"labels": map[string]interface{}{
					labelInferenceServer: endpoint.InferenceServerName,
					labelClusterID:       endpoint.ClusterID,
					labelComponent:       "endpoint-registry",
					labelManagedBy:       "michelangelo",
				},
				"annotations": map[string]interface{}{
					annotationClusterID:   endpoint.ClusterID,
					annotationAPIHost:     endpoint.Host,
					annotationAPIPort:     endpoint.Port,
					annotationTokenSecret: endpoint.TokenSecretRef,
					annotationCASecret:    endpoint.CASecretRef,
				},
			},
			"spec": map[string]interface{}{
				"hosts": []interface{}{
					// Use a virtual hostname that clients will use
					fmt.Sprintf("%s-%s.inference.local", endpoint.InferenceServerName, endpoint.ClusterID),
				},
				"location": "MESH_EXTERNAL",
				"ports": []interface{}{
					map[string]interface{}{
						"number":   int64(servicePort),
						"name":     "http",
						"protocol": "HTTP",
					},
					map[string]interface{}{
						"number":   int64(defaultGRPCPort),
						"name":     "grpc",
						"protocol": "GRPC",
					},
				},
				"resolution": "DNS",
				"endpoints": []interface{}{
					map[string]interface{}{
						"address": endpoint.Host, // The actual node hostname/IP
						"ports": map[string]interface{}{
							"http": int64(servicePort),
							"grpc": int64(defaultGRPCPort),
						},
					},
				},
			},
		},
	}
}

// buildExternalNameService constructs a Kubernetes ExternalName Service.
// The ExternalName points to the ServiceEntry's virtual hostname, enabling
// HTTPRoutes to route through the ServiceEntry to the actual backend.
func (r *istioEndpointRegistry) buildExternalNameService(endpoint ClusterEndpoint) *corev1.Service {
	serviceName := r.buildExternalServiceName(endpoint.InferenceServerName, endpoint.ClusterID)
	// Point to the ServiceEntry's virtual hostname (not the raw node hostname)
	serviceEntryHost := fmt.Sprintf("%s-%s.inference.local", endpoint.InferenceServerName, endpoint.ClusterID)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: endpoint.Namespace,
			Labels: map[string]string{
				labelInferenceServer: endpoint.InferenceServerName,
				labelClusterID:       endpoint.ClusterID,
				labelComponent:       "endpoint-registry",
				labelManagedBy:       "michelangelo",
			},
			Annotations: map[string]string{
				annotationClusterID:   endpoint.ClusterID,
				annotationAPIHost:     endpoint.Host,
				annotationAPIPort:     endpoint.Port,
				annotationTokenSecret: endpoint.TokenSecretRef,
				annotationCASecret:    endpoint.CASecretRef,
			},
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: serviceEntryHost,
		},
	}
}

// parseServiceEntry extracts a ClusterEndpoint from a ServiceEntry.
func (r *istioEndpointRegistry) parseServiceEntry(item *unstructured.Unstructured) (*ClusterEndpoint, error) {
	annotations := item.GetAnnotations()
	labels := item.GetLabels()

	// Extract hosts from spec
	hosts, found, err := unstructured.NestedStringSlice(item.Object, "spec", "hosts")
	if err != nil || !found || len(hosts) == 0 {
		return nil, fmt.Errorf("ServiceEntry has no hosts configured")
	}

	// Extract port from spec
	ports, found, err := unstructured.NestedSlice(item.Object, "spec", "ports")
	var servicePort uint32 = defaultHTTPPort
	if err == nil && found && len(ports) > 0 {
		if portMap, ok := ports[0].(map[string]interface{}); ok {
			if portNum, exists := portMap["number"]; exists {
				switch v := portNum.(type) {
				case int64:
					servicePort = uint32(v)
				case float64:
					servicePort = uint32(v)
				}
			}
		}
	}

	return &ClusterEndpoint{
		ClusterID:           annotations[annotationClusterID],
		InferenceServerName: labels[labelInferenceServer],
		Namespace:           item.GetNamespace(),
		Host:                annotations[annotationAPIHost],
		Port:                annotations[annotationAPIPort],
		ServiceHost:         hosts[0],
		ServicePort:         servicePort,
		TokenSecretRef:      annotations[annotationTokenSecret],
		CASecretRef:         annotations[annotationCASecret],
	}, nil
}

// buildServiceEntryName generates the name for a ServiceEntry.
func (r *istioEndpointRegistry) buildServiceEntryName(inferenceServerName, clusterID string) string {
	return fmt.Sprintf("%s-%s-se", inferenceServerName, clusterID)
}

// buildExternalServiceName generates the name for an ExternalName Service.
func (r *istioEndpointRegistry) buildExternalServiceName(inferenceServerName, clusterID string) string {
	return fmt.Sprintf("%s-%s-external", inferenceServerName, clusterID)
}
