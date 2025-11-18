package common

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/shared/configmap"
)

// UpdateDeploymentModel updates the model for a specific deployment using simplified shared ConfigMap approach
// This adds entries directly to the shared model-config ConfigMap (no deployment-registry)
func UpdateDeploymentModel(ctx context.Context, logger *zap.Logger, provider configmap.ModelConfigMapProvider, inferenceServerName, namespace, deploymentName, modelName string, roleType string) error {
	logger.Info("Updating deployment model directly in shared ConfigMap",
		zap.String("inferenceServer", inferenceServerName),
		zap.String("deployment", deploymentName),
		zap.String("model", modelName),
		zap.String("role", roleType))

	// Simply add the model to the shared ConfigMap
	if modelName != "" {
		if err := addModelToConfig(ctx, logger, provider, inferenceServerName, namespace, modelName, fmt.Sprintf("s3://deploy-models/%s/", modelName)); err != nil {
			logger.Error("Failed to add model to shared ConfigMap", zap.String("model", modelName), zap.Error(err))
			return fmt.Errorf("failed to add model to shared ConfigMap: %w", err)
		}
	}

	logger.Info("Successfully updated deployment model in shared ConfigMap", zap.String("deployment", deploymentName), zap.String("model", modelName), zap.String("role", roleType))
	return nil
}

// AddModelToConfig adds a new model to existing configuration
func addModelToConfig(ctx context.Context, logger *zap.Logger, provider configmap.ModelConfigMapProvider, inferenceServerName, namespace, modelName, modelPath string) error {
	// Get current config
	currentConfigs, err := provider.GetModelConfigMap(ctx, configmap.GetModelConfigMapRequest{
		InferenceServer: inferenceServerName,
		Namespace:       namespace,
	})
	if err != nil {
		logger.Error("Failed to get current model config", zap.Error(err))
		return fmt.Errorf("failed to get current model config: %w", err)
	}

	// Add new model if not found
	found := false
	for i, config := range currentConfigs {
		if config.Name == modelName {
			currentConfigs[i].S3Path = modelPath
			found = true
			break
		}
	}

	if !found {
		currentConfigs = append(currentConfigs, configmap.ModelConfigEntry{
			Name:   modelName,
			S3Path: modelPath,
		})
	}

	// Update ConfigMap
	request := configmap.UpdateModelConfigMapRequest{
		InferenceServer: inferenceServerName,
		Namespace:       namespace,
		ModelConfigs:    currentConfigs,
	}

	return provider.UpdateModelConfigMap(ctx, request)
}
