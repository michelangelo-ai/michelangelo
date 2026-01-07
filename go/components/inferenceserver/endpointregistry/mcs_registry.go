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

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/secrets"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// MCS API Group and Version constants.
// Using the standard Kubernetes MCS API (KEP-1645).
// For GKE, use "net.gke.io/v1" instead.
const (
	mcsAPIGroup   = "multicluster.x-k8s.io"
	mcsAPIVersion = "v1alpha1"

	serviceImportResource = "serviceimports"

	// Label for tracking which cluster the ServiceImport is associated with
	labelSourceCluster = "michelangelo.ai/source-cluster"
)

var (
	serviceExportGVK = schema.GroupVersionKind{
		Group:   mcsAPIGroup,
		Version: mcsAPIVersion,
		Kind:    "ServiceExport",
	}
	serviceImportGVR = schema.GroupVersionResource{
		Group:    mcsAPIGroup,
		Version:  mcsAPIVersion,
		Resource: serviceImportResource,
	}
)

var _ EndpointRegistry = &mcsEndpointRegistry{}

// mcsEndpointRegistry implements EndpointRegistry using Kubernetes Multi-Cluster Services (MCS).
// It creates ServiceExport in target clusters and ServiceImport in the control plane,
// enabling cross-cluster service discovery and routing.
type mcsEndpointRegistry struct {
	dynamicClient dynamic.Interface
	kubeClient    client.Client
	clientFactory clientfactory.ClientFactory
	logger        *zap.Logger
}

// NewMCSEndpointRegistry creates a new MCS-based endpoint registry.
func NewMCSEndpointRegistry(dynamicClient dynamic.Interface, kubeClient client.Client, logger *zap.Logger) EndpointRegistry {
	sp := secrets.NewProvider(kubeClient)
	return &mcsEndpointRegistry{
		dynamicClient: dynamicClient,
		kubeClient:    kubeClient,
		clientFactory: clientfactory.NewClientFactory(kubeClient, sp, kubeClient.Scheme(), logger),
		logger:        logger,
	}
}

// RegisterEndpoint creates a ServiceExport in the target cluster and a ServiceImport
// in the control plane for the cluster endpoint.
func (r *mcsEndpointRegistry) RegisterEndpoint(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error {
	logger.Info("Registering cluster endpoint via MCS",
		zap.String("inferenceServer", endpoint.InferenceServerName),
		zap.String("namespace", endpoint.Namespace),
		zap.String("clusterID", endpoint.ClusterID),
	)

	// Create ServiceExport in the target cluster
	if err := r.createServiceExport(ctx, logger, endpoint); err != nil {
		return fmt.Errorf("failed to create ServiceExport for cluster %s: %w", endpoint.ClusterID, err)
	}

	// Create ServiceImport in the control plane
	if err := r.createServiceImport(ctx, logger, endpoint); err != nil {
		return fmt.Errorf("failed to create ServiceImport for cluster %s: %w", endpoint.ClusterID, err)
	}

	logger.Info("Successfully registered cluster endpoint via MCS",
		zap.String("inferenceServer", endpoint.InferenceServerName),
		zap.String("clusterID", endpoint.ClusterID),
	)
	return nil
}

// UnregisterEndpoint removes the ServiceExport from the target cluster and
// ServiceImport from the control plane.
func (r *mcsEndpointRegistry) UnregisterEndpoint(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace, clusterID string) error {
	logger.Info("Unregistering cluster endpoint via MCS",
		zap.String("inferenceServer", inferenceServerName),
		zap.String("namespace", namespace),
		zap.String("clusterID", clusterID),
	)

	// Delete ServiceImport from control plane first
	serviceImportName := r.buildServiceImportName(inferenceServerName, clusterID)
	if err := r.dynamicClient.Resource(serviceImportGVR).Namespace(namespace).Delete(ctx, serviceImportName, metav1.DeleteOptions{}); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete ServiceImport %s: %w", serviceImportName, err)
		}
		logger.Debug("ServiceImport not found, already deleted", zap.String("name", serviceImportName))
	}

	// Note: We don't delete the ServiceExport from the target cluster here because:
	// 1. We would need the connection spec to get the target cluster client
	// 2. The backend already handles cleanup in the target cluster
	// 3. The ServiceExport should be deleted when the service is deleted in the target cluster
	//
	// If explicit cleanup is needed, the backend should call a separate method
	// or the ServiceExport should be owned by the service in the target cluster.

	logger.Info("Successfully unregistered cluster endpoint via MCS",
		zap.String("inferenceServer", inferenceServerName),
		zap.String("clusterID", clusterID),
	)
	return nil
}

// GetEndpoints retrieves all registered cluster endpoints for an inference server.
func (r *mcsEndpointRegistry) GetEndpoints(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace string) ([]ClusterEndpoint, error) {
	logger.Debug("Getting endpoints for inference server via MCS",
		zap.String("inferenceServer", inferenceServerName),
		zap.String("namespace", namespace),
	)

	// List ServiceImports with label selector
	labelSelector := fmt.Sprintf("%s=%s", labelInferenceServer, inferenceServerName)
	list, err := r.dynamicClient.Resource(serviceImportGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list ServiceImports: %w", err)
	}

	endpoints := make([]ClusterEndpoint, 0, len(list.Items))
	for _, item := range list.Items {
		endpoint, err := r.parseServiceImport(&item)
		if err != nil {
			logger.Warn("Failed to parse ServiceImport, skipping",
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
func (r *mcsEndpointRegistry) GetEndpoint(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace, clusterID string) (*ClusterEndpoint, error) {
	serviceImportName := r.buildServiceImportName(inferenceServerName, clusterID)

	item, err := r.dynamicClient.Resource(serviceImportGVR).Namespace(namespace).Get(ctx, serviceImportName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get ServiceImport %s: %w", serviceImportName, err)
	}

	return r.parseServiceImport(item)
}

// UpdateEndpoint updates an existing cluster endpoint registration.
func (r *mcsEndpointRegistry) UpdateEndpoint(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error {
	logger.Info("Updating cluster endpoint via MCS",
		zap.String("inferenceServer", endpoint.InferenceServerName),
		zap.String("clusterID", endpoint.ClusterID),
	)

	// Update ServiceImport in the control plane
	if err := r.updateServiceImport(ctx, logger, endpoint); err != nil {
		return fmt.Errorf("failed to update ServiceImport for cluster %s: %w", endpoint.ClusterID, err)
	}

	return nil
}

// createServiceExport creates a ServiceExport in the target cluster to export
// the inference service for multi-cluster discovery.
func (r *mcsEndpointRegistry) createServiceExport(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error {
	// Build connection spec from endpoint information
	connectionSpec := &v2pb.ConnectionSpec{
		Host:      endpoint.Host,
		Port:      endpoint.Port,
		TokenTag:  endpoint.TokenSecretRef,
		CaDataTag: endpoint.CASecretRef,
	}

	// Get client for the target cluster
	targetClient, err := r.clientFactory.GetClient(ctx, connectionSpec)
	if err != nil {
		return fmt.Errorf("failed to get client for target cluster: %w", err)
	}

	// The ServiceExport name must match the service name in the target cluster
	serviceName := fmt.Sprintf("%s-inference-service", endpoint.InferenceServerName)

	// Check if the service exists in the target cluster
	existingService := &corev1.Service{}
	if getErr := targetClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: endpoint.Namespace}, existingService); getErr != nil {
		if errors.IsNotFound(getErr) {
			logger.Warn("Service not found in target cluster, skipping ServiceExport creation",
				zap.String("service", serviceName),
				zap.String("cluster", endpoint.ClusterID))
			return nil
		}
		return fmt.Errorf("failed to check service existence: %w", getErr)
	}

	// Check if ServiceExport already exists using unstructured client
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(serviceExportGVK)
	if getErr := targetClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: endpoint.Namespace}, existing); getErr == nil {
		logger.Info("ServiceExport already exists in target cluster, skipping creation",
			zap.String("name", existing.GetName()))
		return nil
	} else if !errors.IsNotFound(getErr) {
		return fmt.Errorf("failed to check existing ServiceExport: %w", getErr)
	}

	// Create ServiceExport using unstructured client
	serviceExport := r.buildServiceExport(endpoint, serviceName)
	if err := targetClient.Create(ctx, serviceExport); err != nil {
		return fmt.Errorf("failed to create ServiceExport: %w", err)
	}

	logger.Info("Created ServiceExport in target cluster",
		zap.String("name", serviceName),
		zap.String("cluster", endpoint.ClusterID),
	)
	return nil
}

// createServiceImport creates a ServiceImport in the control plane to import
// the service from the target cluster.
func (r *mcsEndpointRegistry) createServiceImport(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error {
	serviceImportName := r.buildServiceImportName(endpoint.InferenceServerName, endpoint.ClusterID)

	// Check if ServiceImport already exists
	existing, err := r.dynamicClient.Resource(serviceImportGVR).Namespace(endpoint.Namespace).Get(ctx, serviceImportName, metav1.GetOptions{})
	if err == nil {
		logger.Info("ServiceImport already exists, skipping creation", zap.String("name", serviceImportName))
		return r.updateServiceImportSpec(ctx, existing, endpoint)
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check existing ServiceImport: %w", err)
	}

	serviceImport := r.buildServiceImport(endpoint)
	if _, err := r.dynamicClient.Resource(serviceImportGVR).Namespace(endpoint.Namespace).Create(ctx, serviceImport, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create ServiceImport: %w", err)
	}

	logger.Info("Created ServiceImport",
		zap.String("name", serviceImportName),
		zap.String("cluster", endpoint.ClusterID),
	)
	return nil
}

// updateServiceImport updates an existing ServiceImport.
func (r *mcsEndpointRegistry) updateServiceImport(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error {
	serviceImportName := r.buildServiceImportName(endpoint.InferenceServerName, endpoint.ClusterID)

	existing, err := r.dynamicClient.Resource(serviceImportGVR).Namespace(endpoint.Namespace).Get(ctx, serviceImportName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return r.createServiceImport(ctx, logger, endpoint)
		}
		return fmt.Errorf("failed to get ServiceImport: %w", err)
	}

	return r.updateServiceImportSpec(ctx, existing, endpoint)
}

// updateServiceImportSpec updates the spec of an existing ServiceImport.
func (r *mcsEndpointRegistry) updateServiceImportSpec(ctx context.Context, existing *unstructured.Unstructured, endpoint ClusterEndpoint) error {
	newImport := r.buildServiceImport(endpoint)

	// Update annotations and spec
	existing.SetAnnotations(newImport.GetAnnotations())
	if err := unstructured.SetNestedField(existing.Object, newImport.Object["spec"], "spec"); err != nil {
		return fmt.Errorf("failed to set spec: %w", err)
	}

	if _, err := r.dynamicClient.Resource(serviceImportGVR).Namespace(endpoint.Namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update ServiceImport: %w", err)
	}

	return nil
}

// buildServiceExport constructs a ServiceExport for the target cluster.
func (r *mcsEndpointRegistry) buildServiceExport(endpoint ClusterEndpoint, serviceName string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": fmt.Sprintf("%s/%s", mcsAPIGroup, mcsAPIVersion),
			"kind":       "ServiceExport",
			"metadata": map[string]any{
				"name":      serviceName,
				"namespace": endpoint.Namespace,
				"labels": map[string]any{
					labelInferenceServer: endpoint.InferenceServerName,
					labelClusterID:       endpoint.ClusterID,
					labelComponent:       "endpoint-registry",
					labelManagedBy:       "michelangelo",
				},
			},
		},
	}
	u.SetGroupVersionKind(serviceExportGVK)
	return u
}

// buildServiceImport constructs a ServiceImport for the control plane.
// The ServiceImport references the exported service from the target cluster.
func (r *mcsEndpointRegistry) buildServiceImport(endpoint ClusterEndpoint) *unstructured.Unstructured {
	serviceImportName := r.buildServiceImportName(endpoint.InferenceServerName, endpoint.ClusterID)
	// The source service name in the target cluster
	sourceServiceName := fmt.Sprintf("%s-inference-service", endpoint.InferenceServerName)

	servicePort := endpoint.ServicePort
	if servicePort == 0 {
		servicePort = defaultHTTPPort
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": fmt.Sprintf("%s/%s", mcsAPIGroup, mcsAPIVersion),
			"kind":       "ServiceImport",
			"metadata": map[string]any{
				"name":      serviceImportName,
				"namespace": endpoint.Namespace,
				"labels": map[string]any{
					labelInferenceServer: endpoint.InferenceServerName,
					labelClusterID:       endpoint.ClusterID,
					labelSourceCluster:   endpoint.ClusterID,
					labelComponent:       "endpoint-registry",
					labelManagedBy:       "michelangelo",
				},
				"annotations": map[string]any{
					annotationClusterID:              endpoint.ClusterID,
					annotationAPIHost:                endpoint.Host,
					annotationAPIPort:                endpoint.Port,
					annotationTokenSecret:            endpoint.TokenSecretRef,
					annotationCASecret:               endpoint.CASecretRef,
					"michelangelo.ai/source-service": sourceServiceName,
				},
			},
			"spec": map[string]any{
				// Type can be "ClusterSetIP" (allocates VIP) or "Headless"
				"type": "ClusterSetIP",
				"ports": []any{
					map[string]any{
						"name":     "http",
						"port":     int64(servicePort),
						"protocol": "TCP",
					},
					map[string]any{
						"name":     "grpc",
						"port":     int64(defaultGRPCPort),
						"protocol": "TCP",
					},
				},
				// sessionAffinity can be "None" or "ClientIP"
				"sessionAffinity": "None",
			},
		},
	}
}

// parseServiceImport extracts a ClusterEndpoint from a ServiceImport.
func (r *mcsEndpointRegistry) parseServiceImport(item *unstructured.Unstructured) (*ClusterEndpoint, error) {
	annotations := item.GetAnnotations()
	labels := item.GetLabels()

	// Extract port from spec
	ports, found, err := unstructured.NestedSlice(item.Object, "spec", "ports")
	var servicePort uint32 = defaultHTTPPort
	if err == nil && found && len(ports) > 0 {
		if portMap, ok := ports[0].(map[string]any); ok {
			if portNum, exists := portMap["port"]; exists {
				switch v := portNum.(type) {
				case int64:
					servicePort = uint32(v)
				case float64:
					servicePort = uint32(v)
				}
			}
		}
	}

	// The ServiceHost for MCS is derived from the ServiceImport name
	// MCS creates a derived service with the format: <name>.<namespace>.svc.clusterset.local
	serviceHost := fmt.Sprintf("%s.%s.svc.clusterset.local", item.GetName(), item.GetNamespace())

	return &ClusterEndpoint{
		ClusterID:           annotations[annotationClusterID],
		InferenceServerName: labels[labelInferenceServer],
		Namespace:           item.GetNamespace(),
		Host:                annotations[annotationAPIHost],
		Port:                annotations[annotationAPIPort],
		ServiceHost:         serviceHost,
		ServicePort:         servicePort,
		TokenSecretRef:      annotations[annotationTokenSecret],
		CASecretRef:         annotations[annotationCASecret],
	}, nil
}

// buildServiceImportName generates the name for a ServiceImport.
func (r *mcsEndpointRegistry) buildServiceImportName(inferenceServerName, clusterID string) string {
	return fmt.Sprintf("%s-%s-mcs", inferenceServerName, clusterID)
}

// DeleteServiceExportFromCluster deletes the ServiceExport from a target cluster.
// This is a separate method that can be called when full cleanup is needed.
func (r *mcsEndpointRegistry) DeleteServiceExportFromCluster(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error {
	// Build connection spec from endpoint information
	connectionSpec := &v2pb.ConnectionSpec{
		Host:      endpoint.Host,
		Port:      endpoint.Port,
		TokenTag:  endpoint.TokenSecretRef,
		CaDataTag: endpoint.CASecretRef,
	}

	// Get client for the target cluster
	targetClient, err := r.clientFactory.GetClient(ctx, connectionSpec)
	if err != nil {
		return fmt.Errorf("failed to get client for target cluster: %w", err)
	}

	serviceName := fmt.Sprintf("%s-inference-service", endpoint.InferenceServerName)

	// Delete using unstructured client
	serviceExport := &unstructured.Unstructured{}
	serviceExport.SetGroupVersionKind(serviceExportGVK)
	serviceExport.SetName(serviceName)
	serviceExport.SetNamespace(endpoint.Namespace)

	if err := targetClient.Delete(ctx, serviceExport); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete ServiceExport %s: %w", serviceName, err)
		}
		logger.Debug("ServiceExport not found, already deleted", zap.String("name", serviceName))
	}

	logger.Info("Deleted ServiceExport from target cluster",
		zap.String("name", serviceName),
		zap.String("cluster", endpoint.ClusterID),
	)
	return nil
}
