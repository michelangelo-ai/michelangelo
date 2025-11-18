package configmap

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
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
	BackendType     v2pb.BackendType
	ModelConfigs    []ModelConfigEntry
	Labels          map[string]string
	Annotations     map[string]string
}

// UpdateModelConfigMapRequest contains information needed to update a ModelConfigMap
type UpdateModelConfigMapRequest struct {
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
	ModelConfigs    []ModelConfigEntry
	Labels          map[string]string
	Annotations     map[string]string
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

// ModelConfigMapProvider defines an interface for managing Kubernetes ConfigMaps
// specifically for model configurations used in deployment and serving scenarios.
type ModelConfigMapProvider interface {
	CreateModelConfigMap(ctx context.Context, request CreateModelConfigMapRequest) error
	UpdateModelConfigMap(ctx context.Context, request UpdateModelConfigMapRequest) error
	GetModelConfigMap(ctx context.Context, request GetModelConfigMapRequest) ([]ModelConfigEntry, error)
	DeleteModelConfigMap(ctx context.Context, request DeleteModelConfigMapRequest) error
}
