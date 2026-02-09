//go:generate mamockgen ModelConfigProvider

package modelconfig

import (
	"context"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ModelConfigEntry represents a model configuration with name and storage location.
type ModelConfigEntry struct {
	Name        string `json:"name"`
	StoragePath string `json:"storage_path"`
}

// ModelConfigProvider manages model configurations for inference servers.
// This facilitates model management through a sidecar pattern, where a sidecar container
// watches the config and loads/unloads models accordingly.
// Configurations are stored in a backing store (e.g., Kubernetes ConfigMap, or other storage).
// The InferenceServer controller creates/deletes the config, while the Deployment controller adds/removes model entries.
type ModelConfigProvider interface {
	// CreateModelConfig creates a new model config with model configurations.
	CreateModelConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string, labels map[string]string, annotations map[string]string) error

	// CheckModelConfigExists checks if a model config exists for an inference server.
	CheckModelConfigExists(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string) (bool, error)

	// DeleteModelConfig removes the entire model config for an inference server.
	DeleteModelConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string) error

	// GetModelsFromConfig retrieves all models from a config.
	GetModelsFromConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string) ([]ModelConfigEntry, error)

	// AddModelToConfig adds a single model to an existing config.
	AddModelToConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string, entry ModelConfigEntry) error

	// RemoveModelFromConfig removes a model from a config.
	RemoveModelFromConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServerName string, namespace string, modelName string) error
}
