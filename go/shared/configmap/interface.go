package configmap

import "context"

type ConfigMapProvider interface {
	CreateModelConfigMap(ctx context.Context, request ConfigMapRequest) error
	GetModelConfigMap(ctx context.Context, inferenceServerName, namespace string) ([]ModelConfigEntry, error)
	UpdateModelConfigMap(ctx context.Context, request ConfigMapRequest) error
	DeleteModelConfigMap(ctx context.Context, inferenceServerName, namespace string) error
	UpdateDeploymentModel(ctx context.Context, inferenceServerName, namespace, deploymentName, modelName string, roleType string) error
}
