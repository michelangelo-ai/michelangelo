//go:generate mamockgen ModelConfigMapProvider

package configmap

import (
	"context"
)

// ModelConfigEntry represents a model configuration with name and storage location.
type ModelConfigEntry struct {
	Name   string `json:"name"`
	S3Path string `json:"s3_path"`
}

// CreateModelConfigMapRequest specifies parameters for creating a model configuration ConfigMap.
type CreateModelConfigMapRequest struct {
	InferenceServer string
	Namespace       string
	ModelConfigs    []ModelConfigEntry
	Labels          map[string]string
	Annotations     map[string]string
}

// AddModelToConfigMapRequest specifies parameters for adding a model to an existing ConfigMap.
type AddModelToConfigMapRequest struct {
	InferenceServer string
	Namespace       string
	ModelConfig     ModelConfigEntry
}

// RemoveModelFromConfigMapRequest specifies parameters for removing a model from a ConfigMap.
type RemoveModelFromConfigMapRequest struct {
	InferenceServer string
	Namespace       string
	ModelName       string
}

// GetModelConfigMapRequest specifies parameters for retrieving model configurations.
type GetModelConfigMapRequest struct {
	InferenceServer string
	Namespace       string
}

// DeleteModelConfigMapRequest specifies parameters for deleting a model configuration ConfigMap.
type DeleteModelConfigMapRequest struct {
	InferenceServer string
	Namespace       string
}

// ModelConfigMapProvider manages model configurations stored in Kubernetes ConfigMaps.
// Used by inference servers (particularly Triton) to configure which models to load.
type ModelConfigMapProvider interface {
	// CreateModelConfigMap creates a new ConfigMap with model configurations.
	CreateModelConfigMap(ctx context.Context, request CreateModelConfigMapRequest) error

	// GetModelsFromConfigMap retrieves all model configurations from a ConfigMap.
	GetModelsFromConfigMap(ctx context.Context, request GetModelConfigMapRequest) ([]ModelConfigEntry, error)

	// AddModelToConfigMap adds a single model configuration to an existing ConfigMap.
	AddModelToConfigMap(ctx context.Context, request AddModelToConfigMapRequest) error

	// RemoveModelFromConfigMap removes a model configuration from a ConfigMap.
	RemoveModelFromConfigMap(ctx context.Context, request RemoveModelFromConfigMapRequest) error

	// DeleteModelConfigMap removes the entire ConfigMap for an inference server.
	DeleteModelConfigMap(ctx context.Context, request DeleteModelConfigMapRequest) error
}
