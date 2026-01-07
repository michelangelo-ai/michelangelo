package endpointregistry

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	// ConfigMap name prefix for storing endpoint metadata
	endpointConfigMapPrefix = "mcs-endpoints-"
)

var (
	serviceExportGVK = schema.GroupVersionKind{
		Group:   mcsAPIGroup,
		Version: mcsAPIVersion,
		Kind:    "ServiceExport",
	}
)

var _ EndpointRegistry = &mcsEndpointRegistry{}

// mcsEndpointRegistry implements EndpointRegistry using Kubernetes Multi-Cluster Services (MCS).
//
// Architecture:
//   - ServiceExport is created in target clusters to export the inference service
//   - Istio (with multi-cluster remote secrets) discovers ServiceExports and handles routing
//   - A ConfigMap in the control plane stores endpoint metadata for querying
//
// This approach leverages Istio's native MCS support for traffic routing while
// maintaining a simple metadata store for endpoint queries.
type mcsEndpointRegistry struct {
	kubeClient    client.Client
	clientFactory clientfactory.ClientFactory
	logger        *zap.Logger
}

// NewMCSEndpointRegistry creates a new MCS-based endpoint registry.
func NewMCSEndpointRegistry(kubeClient client.Client, logger *zap.Logger) EndpointRegistry {
	sp := secrets.NewProvider(kubeClient)
	return &mcsEndpointRegistry{
		kubeClient:    kubeClient,
		clientFactory: clientfactory.NewClientFactory(kubeClient, sp, kubeClient.Scheme(), logger),
		logger:        logger,
	}
}

// RegisterEndpoint creates a ServiceExport in the target cluster and stores
// endpoint metadata in a ConfigMap in the control plane.
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

	// Store endpoint metadata in ConfigMap
	if err := r.storeEndpointMetadata(ctx, logger, endpoint); err != nil {
		return fmt.Errorf("failed to store endpoint metadata for cluster %s: %w", endpoint.ClusterID, err)
	}

	logger.Info("Successfully registered cluster endpoint via MCS",
		zap.String("inferenceServer", endpoint.InferenceServerName),
		zap.String("clusterID", endpoint.ClusterID),
	)
	return nil
}

// UnregisterEndpoint removes the endpoint metadata from the ConfigMap.
// Note: ServiceExport cleanup in target cluster is handled by the backend.
func (r *mcsEndpointRegistry) UnregisterEndpoint(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace, clusterID string) error {
	logger.Info("Unregistering cluster endpoint via MCS",
		zap.String("inferenceServer", inferenceServerName),
		zap.String("namespace", namespace),
		zap.String("clusterID", clusterID),
	)

	// Remove endpoint metadata from ConfigMap
	if err := r.removeEndpointMetadata(ctx, logger, inferenceServerName, namespace, clusterID); err != nil {
		return fmt.Errorf("failed to remove endpoint metadata for cluster %s: %w", clusterID, err)
	}

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

	configMapName := r.buildConfigMapName(inferenceServerName)
	cm := &corev1.ConfigMap{}
	if err := r.kubeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, cm); err != nil {
		if errors.IsNotFound(err) {
			return []ClusterEndpoint{}, nil
		}
		return nil, fmt.Errorf("failed to get endpoint ConfigMap: %w", err)
	}

	endpoints := make([]ClusterEndpoint, 0, len(cm.Data))
	for _, data := range cm.Data {
		var endpoint ClusterEndpoint
		if err := json.Unmarshal([]byte(data), &endpoint); err != nil {
			logger.Warn("Failed to unmarshal endpoint data, skipping", zap.Error(err))
			continue
		}
		endpoints = append(endpoints, endpoint)
	}

	return endpoints, nil
}

// GetEndpoint retrieves a specific cluster endpoint.
func (r *mcsEndpointRegistry) GetEndpoint(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace, clusterID string) (*ClusterEndpoint, error) {
	configMapName := r.buildConfigMapName(inferenceServerName)
	cm := &corev1.ConfigMap{}
	if err := r.kubeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, cm); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get endpoint ConfigMap: %w", err)
	}

	data, exists := cm.Data[clusterID]
	if !exists {
		return nil, nil
	}

	var endpoint ClusterEndpoint
	if err := json.Unmarshal([]byte(data), &endpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal endpoint data: %w", err)
	}

	return &endpoint, nil
}

// UpdateEndpoint updates an existing cluster endpoint registration.
func (r *mcsEndpointRegistry) UpdateEndpoint(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error {
	logger.Info("Updating cluster endpoint via MCS",
		zap.String("inferenceServer", endpoint.InferenceServerName),
		zap.String("clusterID", endpoint.ClusterID),
	)

	// Update endpoint metadata in ConfigMap
	if err := r.storeEndpointMetadata(ctx, logger, endpoint); err != nil {
		return fmt.Errorf("failed to update endpoint metadata for cluster %s: %w", endpoint.ClusterID, err)
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

// storeEndpointMetadata stores endpoint metadata in a ConfigMap in the control plane.
func (r *mcsEndpointRegistry) storeEndpointMetadata(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error {
	configMapName := r.buildConfigMapName(endpoint.InferenceServerName)

	// Serialize endpoint to JSON
	endpointData, err := json.Marshal(endpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal endpoint: %w", err)
	}

	// Try to get existing ConfigMap
	cm := &corev1.ConfigMap{}
	err = r.kubeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: endpoint.Namespace}, cm)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get ConfigMap: %w", err)
		}

		// ConfigMap doesn't exist, create it
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: endpoint.Namespace,
				Labels: map[string]string{
					labelInferenceServer: endpoint.InferenceServerName,
					labelComponent:       "endpoint-registry",
					labelManagedBy:       "michelangelo",
				},
			},
			Data: map[string]string{
				endpoint.ClusterID: string(endpointData),
			},
		}
		if createErr := r.kubeClient.Create(ctx, cm); createErr != nil {
			return fmt.Errorf("failed to create ConfigMap: %w", createErr)
		}
		logger.Info("Created endpoint metadata ConfigMap",
			zap.String("name", configMapName),
			zap.String("clusterID", endpoint.ClusterID),
		)
		return nil
	}

	// ConfigMap exists, update it
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[endpoint.ClusterID] = string(endpointData)

	if err := r.kubeClient.Update(ctx, cm); err != nil {
		return fmt.Errorf("failed to update ConfigMap: %w", err)
	}

	logger.Info("Updated endpoint metadata in ConfigMap",
		zap.String("name", configMapName),
		zap.String("clusterID", endpoint.ClusterID),
	)
	return nil
}

// removeEndpointMetadata removes endpoint metadata from the ConfigMap.
func (r *mcsEndpointRegistry) removeEndpointMetadata(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace, clusterID string) error {
	configMapName := r.buildConfigMapName(inferenceServerName)

	cm := &corev1.ConfigMap{}
	err := r.kubeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			// ConfigMap doesn't exist, nothing to remove
			return nil
		}
		return fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	// Remove the cluster entry
	delete(cm.Data, clusterID)

	// If ConfigMap is empty, delete it
	if len(cm.Data) == 0 {
		if err := r.kubeClient.Delete(ctx, cm); err != nil {
			return fmt.Errorf("failed to delete empty ConfigMap: %w", err)
		}
		logger.Info("Deleted empty endpoint metadata ConfigMap",
			zap.String("name", configMapName),
		)
		return nil
	}

	// Otherwise, update the ConfigMap
	if err := r.kubeClient.Update(ctx, cm); err != nil {
		return fmt.Errorf("failed to update ConfigMap: %w", err)
	}

	logger.Info("Removed endpoint metadata from ConfigMap",
		zap.String("name", configMapName),
		zap.String("clusterID", clusterID),
	)
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

// buildConfigMapName generates the name for the endpoint metadata ConfigMap.
func (r *mcsEndpointRegistry) buildConfigMapName(inferenceServerName string) string {
	return fmt.Sprintf("%s%s", endpointConfigMapPrefix, inferenceServerName)
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
