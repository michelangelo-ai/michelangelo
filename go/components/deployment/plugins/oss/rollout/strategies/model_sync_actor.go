package strategies

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ModelSyncActor loads models to inference servers by updating ConfigMaps and verifying model readiness.
type ModelSyncActor struct {
	gateway gateways.Gateway
	logger  *zap.Logger
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

		// Implement retry logic with configurable timeout for health checks
		modelReady, err := a.checkModelStatusWithTimeout(ctx, a.logger, modelName, inferenceServerName, deployment.Namespace)
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
		// TODO(#696): ghosharitra: make the storage path configurable w.r.t storage client and storage location
		if err := a.gateway.LoadModel(ctx, a.logger, modelName, fmt.Sprintf("s3://deploy-models/%s/", modelName), inferenceServerName, deployment.Namespace); err != nil {
			a.logger.Error("Failed to initiate model loading", zap.Error(err), zap.String("operation", "load_model"), zap.String("model", modelName), zap.String("inferenceServerName", inferenceServerName), zap.String("namespace", deployment.Namespace), zap.String("backendType", v2pb.BACKEND_TYPE_TRITON.String()))
			return &apipb.Condition{
				Type:    a.GetType(),
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "ModelLoadingFailed",
				Message: fmt.Sprintf("Failed to update deployment: %v", err),
			}, nil
		}

		a.logger.Info("Successfully initiated model loading",
			zap.String("operation", "load_model"),
			zap.String("model", modelName),
			zap.String("inferenceServerName", inferenceServerName),
			zap.String("namespace", deployment.Namespace),
			zap.String("backendType", v2pb.BACKEND_TYPE_TRITON.String()))
	}

	// Return unknown so that the condition is only true when the model is truely ready and loaded in triton
	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_UNKNOWN, Reason: "ModelSyncPending", Message: "Model sync is in progress"}, nil
}

// checkModelStatusWithTimeout implements retry logic with configurable timeout for model health checks
func (a *ModelSyncActor) checkModelStatusWithTimeout(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string) (bool, error) {
	const (
		modelHealthCheckTimeout  = 10 * time.Minute // Configurable timeout for model health checks
		modelHealthCheckInterval = 30 * time.Second // Interval between health check retries
	)

	logger.Info("Starting model health check with timeout",
		zap.String("model", modelName),
		zap.String("inference_server", inferenceServerName),
		zap.String("namespace", namespace),
		zap.Int("timeout", int(modelHealthCheckTimeout)),
		zap.Int("retryInterval", int(modelHealthCheckInterval)))

	// Create a context with timeout for the entire health check process
	timeoutCtx, cancel := context.WithTimeout(ctx, modelHealthCheckTimeout)
	defer cancel()

	ticker := time.NewTicker(modelHealthCheckInterval)
	defer ticker.Stop()

	// Try immediately first
	modelReady, err := a.gateway.CheckModelStatus(timeoutCtx, logger, modelName, inferenceServerName, namespace, v2pb.BACKEND_TYPE_TRITON)
	if err == nil && modelReady {
		logger.Info("Model health check succeeded immediately", zap.String("model", modelName))
		return true, nil
	}

	if err != nil {
		logger.Info("Initial model health check failed, will retry",
			zap.String("model", modelName),
			zap.Error(err))
	} else {
		logger.Info("Model not ready, will retry", zap.String("model", modelName))
	}

	// Start retry loop
	for {
		select {
		case <-timeoutCtx.Done():
			logger.Info("Model health check timed out",
				zap.String("model", modelName),
				zap.Int("timeout", int(modelHealthCheckTimeout)))
			return false, fmt.Errorf("health check timeout exceeded")

		case <-ticker.C:
			logger.Info("Retrying model health check", zap.String("model", modelName))

			modelReady, err := a.gateway.CheckModelStatus(timeoutCtx, logger, modelName, inferenceServerName, namespace, v2pb.BACKEND_TYPE_TRITON)
			if err == nil && modelReady {
				logger.Info("Model health check succeeded after retry", zap.String("model", modelName))
				return true, nil
			}

			if err != nil {
				logger.Info("Model health check retry failed, continuing to wait",
					zap.String("model", modelName),
					zap.String("inference_server", inferenceServerName),
					zap.String("namespace", namespace),
					zap.String("backend_type", v2pb.BACKEND_TYPE_TRITON.String()),
					zap.Error(err))
			} else {
				logger.Info("Model still not ready, continuing to wait", zap.String("model", modelName), zap.String("inference_server", inferenceServerName), zap.String("namespace", namespace), zap.String("backend_type", v2pb.BACKEND_TYPE_TRITON.String()))
			}
		}
	}
}
