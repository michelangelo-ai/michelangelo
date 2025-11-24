package configmap

import (
	"context"
)

// ModelConfigEntry represents a model configuration entry
type ModelConfigEntry struct {
	Name   string `json:"name"`
	S3Path string `json:"s3_path"`
}

// CreateModelConfigMapRequest contains information needed to create a ModelConfigMap
type CreateModelConfigMapRequest struct {
	InferenceServer string
	Namespace       string
	ModelConfigs    []ModelConfigEntry
	Labels          map[string]string
	Annotations     map[string]string
}

// AddModelToConfigMapRequest contains information needed to add a model to a ModelConfigMap
type AddModelToConfigMapRequest struct {
	InferenceServer string
	Namespace       string
	ModelConfig     ModelConfigEntry
}

// RemoveModelFromConfigMapRequest contains information needed to remove a model from a ModelConfigMap
type RemoveModelFromConfigMapRequest struct {
	InferenceServer string
	Namespace       string
	ModelName       string
}

// GetModelConfigMapRequest contains information needed to get a ModelConfigMap
type GetModelConfigMapRequest struct {
	InferenceServer string
	Namespace       string
}

// DeleteModelConfigMapRequest contains information needed to delete a ModelConfigMap
type DeleteModelConfigMapRequest struct {
	InferenceServer string
	Namespace       string
}

// ModelConfigMapProvider is an interface for managing model configuraitons through ConfigMaps.
type ModelConfigMapProvider interface {
	CreateModelConfigMap(ctx context.Context, request CreateModelConfigMapRequest) error

	GetModelsFromConfigMap(ctx context.Context, request GetModelConfigMapRequest) ([]ModelConfigEntry, error)
	AddModelToConfigMap(ctx context.Context, request AddModelToConfigMapRequest) error
	RemoveModelFromConfigMap(ctx context.Context, request RemoveModelFromConfigMapRequest) error

	DeleteModelConfigMap(ctx context.Context, request DeleteModelConfigMapRequest) error
}
