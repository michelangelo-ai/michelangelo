package strategies

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ModelSyncActor loads models to inference servers by updating ConfigMaps and verifying model readiness.
type ModelSyncActor struct {
	gateway                gateways.Gateway
	modelConfigMapProvider configmap.ModelConfigMapProvider
	logger                 *zap.Logger
}

// GetType returns the condition type identifier for model sync.
func (a *ModelSyncActor) GetType() string {
	return common.ActorTypeModelSync
}

// GetLogger returns the logger instance for this actor.
func (a *ModelSyncActor) GetLogger() *zap.Logger {
	return a.logger
}

// Retrieve checks if the desired model is loaded and ready in Triton with retry timeout logic.
func (a *ModelSyncActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if the desired model is actually loaded and ready in Triton
	if deployment.Spec.DesiredRevision != nil {
		modelName := deployment.Spec.DesiredRevision.Name

		// Check if model is loaded in Triton using the gateway health check

		inferenceServerName := deployment.Spec.GetInferenceServer().Name

		// Check if the desired model is ready in Triton
		modelStatusRequest := gateways.CheckModelStatusRequest{
			ModelName:       modelName,
			InferenceServer: inferenceServerName,
			DeploymentName:  deployment.Name, // Include deployment name for deployment-specific routing
			Namespace:       deployment.Namespace,
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

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ModelSyncPending",
		Message: "Model sync is in progress",
	}, nil
}

// Run adds the model to the ConfigMap, triggering inference server to load it.
func (a *ModelSyncActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running model sync for deployment", zap.String("deployment", deployment.Name))

	// For OSS, this would sync the model from storage to the inference server
	if deployment.Spec.DesiredRevision != nil {
		modelName := deployment.Spec.DesiredRevision.Name
		inferenceServerName := deployment.Spec.GetInferenceServer().Name

		a.logger.Info("Syncing model to inference server",
			zap.String("model", modelName),
			zap.String("inference_server", inferenceServerName))

		// Update deployment model in ConfigMap
		if err := a.modelConfigMapProvider.AddModelToConfigMap(ctx, configmap.AddModelToConfigMapRequest{
			InferenceServer: inferenceServerName,
			Namespace:       deployment.Namespace,
			ModelConfig: configmap.ModelConfigEntry{
				Name:   modelName,
				S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName),
			},
		}); err != nil {
			a.logger.Error("Failed to update deployment via ConfigMapProvider", zap.Error(err))
			return &apipb.Condition{
				Type:    a.GetType(),
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "ConfigMapUpdateFailed",
				Message: fmt.Sprintf("Failed to update deployment: %v", err),
			}, nil
		}

		a.logger.Info("Updated ConfigMap with candidate model",
			zap.String("deployment", deployment.Name),
			zap.String("candidateModel", modelName),
			zap.String("roleType", "candidate"))
	}

	// Return unknown so that the condition is only true when the model is truely ready and loaded in triton
	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_UNKNOWN, Reason: "ModelSyncPending", Message: "Model sync is in progress"}, nil
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
