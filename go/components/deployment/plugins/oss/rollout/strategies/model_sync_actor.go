package strategies

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/shared/configmap"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// // ModelSyncActor handles model synchronization to inference servers
// type ModelSyncActor struct {
// 	Client  client.Client
// 	Gateway gateways.Gateway
// 	Logger  *zap.Logger
// }

// func (a *ModelSyncActor) GetType() string {
// 	return "ModelSynced"
// }

// func (a *ModelSyncActor) GetLogger() *zap.Logger {
// 	return a.Logger
// }

// func (a *ModelSyncActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
// 	// Check if the desired model is actually loaded and ready in Triton
// 	if resource.Spec.DesiredRevision != nil {
// 		modelName := resource.Spec.DesiredRevision.Name
// 		inferenceServerName := resource.Spec.GetInferenceServer().Name

// 		a.Logger.Info("Checking if model is loaded in Triton", zap.String("model", modelName), zap.String("inference_server", inferenceServerName))

// 		// Check if model is ready in Triton using the gateway health check
// 		if a.Gateway != nil {
// 			statusRequest := gateways.ModelStatusRequest{
// 				ModelName:       modelName,
// 				InferenceServer: inferenceServerName,
// 				Namespace:       resource.Namespace,
// 				BackendType:     v2pb.BACKEND_TYPE_TRITON,
// 			}

// 			ready, err := a.Gateway.CheckModelStatus(ctx, a.Logger.With(zap.String("model", modelName)), statusRequest)
// 			if err != nil {
// 				a.Logger.Error("Failed to check model status in Triton", zap.String("model", modelName), zap.Error(err))
// 				return &apipb.Condition{
// 					Type:    a.GetType(),
// 					Status:  apipb.CONDITION_STATUS_FALSE,
// 					Reason:  "ModelStatusCheckFailed",
// 					Message: fmt.Sprintf("Failed to check model %s status: %v", modelName, err),
// 				}, nil
// 			}

// 			if ready {
// 				a.Logger.Info("Model is ready in Triton", zap.String("model", modelName))
// 				return &apipb.Condition{
// 					Type:    a.GetType(),
// 					Status:  apipb.CONDITION_STATUS_TRUE,
// 					Reason:  "ModelReady",
// 					Message: fmt.Sprintf("Model %s is loaded and ready in Triton", modelName),
// 				}, nil
// 			}

// 			a.Logger.Info("Model is not ready in Triton", zap.String("model", modelName))
// 			return &apipb.Condition{
// 				Type:    a.GetType(),
// 				Status:  apipb.CONDITION_STATUS_FALSE,
// 				Reason:  "ModelNotReady",
// 				Message: fmt.Sprintf("Model %s is not ready in Triton", modelName),
// 			}, nil
// 		}

// 		// For now, assume model sync is needed if gateway is not available
// 		return &apipb.Condition{
// 			Type:    a.GetType(),
// 			Status:  apipb.CONDITION_STATUS_FALSE,
// 			Reason:  "ModelSyncPending",
// 			Message: fmt.Sprintf("Model %s sync is pending", modelName),
// 		}, nil
// 	}

// 	return &apipb.Condition{
// 		Type:    a.GetType(),
// 		Status:  apipb.CONDITION_STATUS_FALSE,
// 		Reason:  "NoModelSpecified",
// 		Message: "No model specified for sync",
// 	}, nil
// }

// func (a *ModelSyncActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
// 	a.Logger.Info("Running model sync for deployment", zap.String("deployment", resource.Name))

// 	if resource.Spec.DesiredRevision != nil {
// 		modelName := resource.Spec.DesiredRevision.Name
// 		inferenceServerName := resource.Spec.GetInferenceServer().Name

// 		a.Logger.Info("Syncing model to inference server",
// 			zap.String("model", modelName),
// 			zap.String("inference_server", inferenceServerName))

// 		// Update the ConfigMap with the new model
// 		if a.Gateway != nil {
// 			updateRequest := gateways.ModelConfigUpdateRequest{
// 				InferenceServer: inferenceServerName,
// 				Namespace:       resource.Namespace,
// 				BackendType:     v2pb.BACKEND_TYPE_TRITON,
// 				ModelConfigs: []gateways.ModelConfigEntry{
// 					{
// 						Name:   modelName,
// 						S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName),
// 					},
// 				},
// 			}

// 			if err := a.Gateway.UpdateModelConfig(ctx, a.Logger, updateRequest); err != nil {
// 				a.Logger.Error("Failed to update model config via gateway", zap.String("model", modelName), zap.Error(err))
// 				return &apipb.Condition{
// 					Type:    a.GetType(),
// 					Status:  apipb.CONDITION_STATUS_FALSE,
// 					Reason:  "ModelConfigUpdateFailed",
// 					Message: fmt.Sprintf("Failed to update model config: %v", err),
// 				}, nil
// 			}

// 			a.Logger.Info("Model configuration updated successfully", zap.String("model", modelName))

// 			// Verify model is loaded and ready in Triton after config update
// 			statusRequest := gateways.ModelStatusRequest{
// 				ModelName:       modelName,
// 				InferenceServer: inferenceServerName,
// 				Namespace:       resource.Namespace,
// 				BackendType:     v2pb.BACKEND_TYPE_TRITON,
// 			}

// 			ready, err := a.Gateway.CheckModelStatus(ctx, a.Logger, statusRequest)
// 			if err != nil {
// 				a.Logger.Error("Failed to verify model status after config update", zap.String("model", modelName), zap.Error(err))
// 				return &apipb.Condition{
// 					Type:    a.GetType(),
// 					Status:  apipb.CONDITION_STATUS_FALSE,
// 					Reason:  "ModelVerificationFailed",
// 					Message: fmt.Sprintf("Failed to verify model %s after config update: %v", modelName, err),
// 				}, nil
// 			}

// 			if !ready {
// 				a.Logger.Info("Model is not ready after config update, will retry", zap.String("model", modelName))
// 				return &apipb.Condition{
// 					Type:    a.GetType(),
// 					Status:  apipb.CONDITION_STATUS_FALSE,
// 					Reason:  "ModelNotReadyAfterUpdate",
// 					Message: fmt.Sprintf("Model %s is not ready after config update", modelName),
// 				}, nil
// 			}

// 			a.Logger.Info("Model verified as ready after config update", zap.String("model", modelName))
// 		}

// 		// Update candidate revision to track progress
// 		resource.Status.CandidateRevision = resource.Spec.DesiredRevision
// 		a.Logger.Info("Model sync completed successfully", zap.String("model", modelName))
// 	}

// 	return &apipb.Condition{
// 		Type:    a.GetType(),
// 		Status:  apipb.CONDITION_STATUS_TRUE,
// 		Reason:  "Success",
// 		Message: "Operation completed successfully",
// 	}, nil
// }

// ModelSyncActor handles model synchronization to inference servers using deployment-level ConfigMap management
type ModelSyncActor struct {
	client            client.Client
	gateway           gateways.Gateway
	configMapProvider configmap.ConfigMapProvider
	logger            *zap.Logger
}

func (a *ModelSyncActor) GetType() string {
	return "ModelSynced"
}

func (a *ModelSyncActor) GetLogger() *zap.Logger {
	return a.logger
}

func (a *ModelSyncActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if the desired model is actually loaded and ready in Triton
	if deployment.Spec.DesiredRevision != nil {
		modelName := deployment.Spec.DesiredRevision.Name

		// Check if model is loaded in Triton using the gateway health check
		if a.gateway != nil {
			inferenceServerName := deployment.Spec.GetInferenceServer().Name

			// Check if the desired model is ready in Triton
			modelStatusRequest := gateways.CheckModelStatusRequest{
				ModelName:       modelName,
				InferenceServer: inferenceServerName,
				DeploymentName:  deployment.Name, // Include deployment name for deployment-specific routing
				Namespace:       deployment.Namespace,
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			}

			// Implement retry logic with configurable timeout for health checks
			modelReady, err := a.checkModelStatusWithTimeout(ctx, a.logger, modelStatusRequest)
			if err != nil {
				// Check if this is a timeout error vs other errors
				if err.Error() == "health check timeout exceeded" {
					a.logger.Info("Model health check timed out after 10 minutes", zap.String("model", modelName))
					return &apipb.Condition{
						Type:    a.GetType(),
						Status:  apipb.CONDITION_STATUS_FALSE,
						Reason:  "ModelHealthCheckTimeout",
						Message: fmt.Sprintf("Model %s health check timed out after 10 minutes", modelName),
					}, nil
				}

				a.logger.Error("Failed to check model status in Triton", zap.String("model", modelName), zap.Error(err))
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_FALSE,
					Reason:  "ModelHealthCheckError",
					Message: fmt.Sprintf("Error checking model %s readiness: %v", modelName, err),
				}, nil
			}

			if modelReady {
				a.logger.Info("New model is loaded and ready in Triton", zap.String("model", modelName))
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_TRUE,
					Reason:  "ModelSyncCompleted",
					Message: fmt.Sprintf("Model %s successfully loaded and ready in Triton", modelName),
				}, nil
			} else {
				a.logger.Info("New model is not yet ready in Triton, continuing to wait", zap.String("model", modelName))
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_FALSE,
					Reason:  "ModelNotReady",
					Message: fmt.Sprintf("Model %s is loading but not yet ready in Triton", modelName),
				}, nil
			}
		}
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ModelSyncPending",
		Message: "Model sync is in progress",
	}, nil
}

func (a *ModelSyncActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running model sync for deployment", zap.String("deployment", deployment.Name))

	// For OSS, this would sync the model from storage to the inference server
	if deployment.Spec.DesiredRevision != nil {
		modelName := deployment.Spec.DesiredRevision.Name
		inferenceServerName := deployment.Spec.GetInferenceServer().Name

		a.logger.Info("Syncing model to inference server",
			zap.String("model", modelName),
			zap.String("inference_server", inferenceServerName))

		// UCS CACHE PATTERN: Replicate Uber's exact UCS cache update pattern from rolling/actor.go:76
		// Original Uber code: err = a.ucsCache.UpdateDeployment(*deployment, constraints, nil, common.RoleTypeCandidate)
		if a.configMapProvider != nil {
			// Follow Uber's pattern exactly: UpdateDeployment with deployment, constraints, role
			// For OSS: constraints are empty (no hosts), but we track deployment-level model ownership
			if err := a.configMapProvider.UpdateDeploymentModel(ctx, inferenceServerName, deployment.Namespace, deployment.Name, modelName, "candidate"); err != nil {
				a.logger.Error("Failed to update deployment via ConfigMapProvider (UCS cache pattern)", zap.Error(err))
				return &apipb.Condition{
					Type:    a.GetType(),
					Status:  apipb.CONDITION_STATUS_FALSE,
					Reason:  "ConfigMapUpdateFailed",
					Message: fmt.Sprintf("Failed to update deployment: %v", err),
				}, nil
			}

			a.logger.Info("UCS cache pattern update completed successfully",
				zap.String("deployment", deployment.Name),
				zap.String("candidateModel", modelName),
				zap.String("roleType", "candidate"))
		} else {
			// Fallback to old gateway-based approach if ConfigMapProvider not available
			a.logger.Info("ConfigMapProvider not available, falling back to gateway approach")
			if a.gateway != nil {
				// Get current models from ConfigMap to preserve them during deployment
				currentModels, err := a.getCurrentModelsFromConfigMap(ctx, a.logger, inferenceServerName, deployment.Namespace)
				if err != nil {
					a.logger.Error("Failed to get current models from ConfigMap", zap.Error(err))
					// Continue with just the new model if we can't read existing ones
					currentModels = []configmap.ModelConfigEntry{}
				}

				// Check if new model already exists to avoid duplicates
				modelExists := false
				for _, model := range currentModels {
					if model.Name == modelName {
						modelExists = true
						break
					}
				}

				// Add the new model if it doesn't already exist
				if !modelExists {
					currentModels = append(currentModels, configmap.ModelConfigEntry{
						Name:   modelName,
						S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName),
					})
					a.logger.Info("Adding new model for zero-downtime deployment",
						zap.String("newModel", modelName), zap.Int("totalModels", len(currentModels)))
				} else {
					a.logger.Info("Model already exists in ConfigMap", zap.String("model", modelName))
				}

				updateRequest := configmap.ConfigMapRequest{
					InferenceServer: inferenceServerName,
					Namespace:       deployment.Namespace,
					BackendType:     v2pb.BACKEND_TYPE_TRITON, // Default to Triton for OSS
					ModelConfigs:    currentModels,
				}

				if err := a.configMapProvider.UpdateModelConfigMap(ctx, updateRequest); err != nil {
					a.logger.Error("Failed to update model config via gateway", zap.Error(err))
					return &apipb.Condition{
						Type:    a.GetType(),
						Status:  apipb.CONDITION_STATUS_FALSE,
						Reason:  "ModelConfigUpdateFailed",
						Message: fmt.Sprintf("Failed to update model config: %v", err),
					}, nil
				}

				a.logger.Info("Model configuration updated successfully for zero-downtime deployment",
					zap.String("model", modelName), zap.Int("totalModels", len(currentModels)))
			}
		}

		// DO NOT update HTTPRoute or CurrentRevision yet!
		// We only sync the model to ConfigMap here. HTTPRoute update and CurrentRevision
		// will be handled by ModelHealthCheckActor after verifying the new model is ready.
		a.logger.Info("Model sync to ConfigMap completed successfully - waiting for health check before switching traffic")
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}

// getCurrentModelsFromConfigMap retrieves current models from the inference server ConfigMap
// Following the correct pattern from PR #188: Get -> Parse with proper error handling
func (a *ModelSyncActor) getCurrentModelsFromConfigMap(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace string) ([]configmap.ModelConfigEntry, error) {
	configMapName := fmt.Sprintf("%s-model-config", inferenceServerName)

	// Get the ConfigMap using Kubernetes client
	configMap := &v1.ConfigMap{}
	key := client.ObjectKey{Name: configMapName, Namespace: namespace}

	if err := a.client.Get(ctx, key, configMap); err != nil {
		// If ConfigMap doesn't exist, return empty list (new deployment)
		if client.IgnoreNotFound(err) == nil {
			logger.Info("ConfigMap not found, starting with empty model list", zap.String("configMap", configMapName))
			return []configmap.ModelConfigEntry{}, nil
		}
		return nil, fmt.Errorf("failed to get ConfigMap %s: %w", configMapName, err)
	}

	// Parse the model-list.json from ConfigMap - following PR #188 pattern
	modelListJSON, exists := configMap.Data["model-list.json"]
	if !exists || modelListJSON == "" {
		logger.Info("model-list.json not found or empty in ConfigMap", zap.String("configMap", configMapName))
		return []configmap.ModelConfigEntry{}, nil
	}

	// Parse JSON to get current models with proper error handling
	var currentModels []configmap.ModelConfigEntry
	if err := json.Unmarshal([]byte(modelListJSON), &currentModels); err != nil {
		logger.Error("Failed to parse model-list.json from ConfigMap", zap.Error(err), zap.String("configMap", configMapName))
		// Return empty list on parse failure rather than nil to allow recovery
		return []configmap.ModelConfigEntry{}, nil
	}

	logger.Info("Retrieved current models from ConfigMap", zap.String("configMap", configMapName), zap.Int("modelCount", len(currentModels)))
	return currentModels, nil
}

// checkModelStatusWithTimeout implements retry logic with configurable timeout for model health checks
func (a *ModelSyncActor) checkModelStatusWithTimeout(ctx context.Context, logger *zap.Logger, modelStatusRequest gateways.CheckModelStatusRequest) (bool, error) {
	const (
		modelHealthCheckTimeout  = 10 * time.Minute // Configurable timeout for model health checks
		modelHealthCheckInterval = 30 * time.Second // Interval between health check retries
	)

	logger.Info("Starting model health check with timeout",
		zap.String("model", modelStatusRequest.ModelName),
		zap.Int("timeout", int(modelHealthCheckTimeout)),
		zap.Int("retryInterval", int(modelHealthCheckInterval)))

	// Create a context with timeout for the entire health check process
	timeoutCtx, cancel := context.WithTimeout(ctx, modelHealthCheckTimeout)
	defer cancel()

	ticker := time.NewTicker(modelHealthCheckInterval)
	defer ticker.Stop()

	// Try immediately first
	modelReady, err := a.gateway.CheckModelStatus(timeoutCtx, logger, modelStatusRequest)
	if err == nil && modelReady {
		logger.Info("Model health check succeeded immediately", zap.String("model", modelStatusRequest.ModelName))
		return true, nil
	}

	if err != nil {
		logger.Info("Initial model health check failed, will retry",
			zap.String("model", modelStatusRequest.ModelName),
			zap.Error(err))
	} else {
		logger.Info("Model not ready, will retry", zap.String("model", modelStatusRequest.ModelName))
	}

	// Start retry loop
	for {
		select {
		case <-timeoutCtx.Done():
			logger.Info("Model health check timed out",
				zap.String("model", modelStatusRequest.ModelName),
				zap.Int("timeout", int(modelHealthCheckTimeout)))
			return false, fmt.Errorf("health check timeout exceeded")

		case <-ticker.C:
			logger.Info("Retrying model health check", zap.String("model", modelStatusRequest.ModelName))

			modelReady, err := a.gateway.CheckModelStatus(timeoutCtx, logger, modelStatusRequest)
			if err == nil && modelReady {
				logger.Info("Model health check succeeded after retry", zap.String("model", modelStatusRequest.ModelName))
				return true, nil
			}

			if err != nil {
				logger.Info("Model health check retry failed, continuing to wait",
					zap.String("model", modelStatusRequest.ModelName),
					zap.Error(err))
			} else {
				logger.Info("Model still not ready, continuing to wait", zap.String("model", modelStatusRequest.ModelName))
			}
		}
	}
}
