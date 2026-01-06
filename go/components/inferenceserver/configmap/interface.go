//go:generate mamockgen ModelConfigMapProvider

package configmap

import (
	"context"
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
	CreateModelConfigMap(ctx context.Context, inferenceServer string, namespace string, modelConfigs []ModelConfigEntry, labels map[string]string, annotations map[string]string) error
	// DeleteModelConfigMap removes the entire ConfigMap for an inference server.
	DeleteModelConfigMap(ctx context.Context, inferenceServer string, namespace string) error
	// GetModelsFromConfigMap retrieves all model configurations from a ConfigMap.
	GetModelsFromConfigMap(ctx context.Context, inferenceServer string, namespace string) ([]ModelConfigEntry, error)
	// AddModelToConfigMap adds a single model configuration to an existing ConfigMap.
	AddModelToConfigMap(ctx context.Context, inferenceServer string, namespace string, modelConfig ModelConfigEntry) error
	// RemoveModelFromConfigMap removes a model configuration from a ConfigMap.
	RemoveModelFromConfigMap(ctx context.Context, inferenceServer string, namespace string, modelName string) error
}
