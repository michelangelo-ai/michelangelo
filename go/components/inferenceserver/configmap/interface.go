//go:generate mamockgen ModelConfigMapProvider

package configmap

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ModelConfigEntry represents a model configuration with name and storage location.
type ModelConfigEntry struct {
	Name        string `json:"name"`
	StoragePath string `json:"storage_path"`
}

// ModelConfigMapProvider manages model configurations stored in Kubernetes ConfigMaps.
// Used by inference servers to configure which models to load.
type ModelConfigMapProvider interface {
	// CreateModelConfigMap creates a new ConfigMap with model configurations.
	CreateModelConfigMap(ctx context.Context, inferenceServer string, namespace string, modelConfigs []ModelConfigEntry, labels map[string]string, annotations map[string]string, targetCluster *v2pb.ClusterTarget) error
	// DeleteModelConfigMap removes the entire ConfigMap for an inference server.
	DeleteModelConfigMap(ctx context.Context, inferenceServer string, namespace string, targetCluster *v2pb.ClusterTarget) error
	// GetModelsFromConfigMap retrieves all model configurations from a ConfigMap.
	GetModelsFromConfigMap(ctx context.Context, inferenceServer string, namespace string, targetCluster *v2pb.ClusterTarget) ([]ModelConfigEntry, error)
	// AddModelToConfigMap adds a single model configuration to an existing ConfigMap.
	AddModelToConfigMap(ctx context.Context, inferenceServer string, namespace string, modelConfig ModelConfigEntry, targetCluster *v2pb.ClusterTarget) error
	// RemoveModelFromConfigMap removes a model configuration from a ConfigMap.
	RemoveModelFromConfigMap(ctx context.Context, inferenceServer string, namespace string, modelName string, targetCluster *v2pb.ClusterTarget) error
}
