package strategies

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gogo/protobuf/types"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	strategiesCommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/rollout/strategies/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	modelconfig "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// getRollingActors returns actors for rolling rollout strategy
func getRollingActors(params Params) []conditionInterfaces.ConditionActor[*v2pb.Deployment] {
	return []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
		&RollingRolloutActor{
			client:              params.Client,
			HTTPClient:          params.HTTPClient,
			gateway:             params.Gateway,
			modelConfigProvider: params.ModelConfigProvider,
			logger:              params.Logger,
		},
		&strategiesCommon.TrafficRoutingActor{
			DynamicClient: params.DynamicClient,
			RouteProvider: params.RouteProvider,
			Logger:        params.Logger,
		},
		&ModelCleanupActor{
			Client:              params.Client,
			HTTPClient:          params.HTTPClient,
			Gateway:             params.Gateway,
			ModelConfigProvider: params.ModelConfigProvider,
			Logger:              params.Logger,
		},
	}
}

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &RollingRolloutActor{}

// RollingRolloutActor loads models into the inference servers via a rolling rollout strategy.
// The strategy involves loading the model into one target cluster at a time and verifying it is ready.
type RollingRolloutActor struct {
	client              client.Client
	HTTPClient          *http.Client
	gateway             gateways.Gateway
	modelConfigProvider modelconfig.ModelConfigProvider
	logger              *zap.Logger
}

// GetType returns the condition type identifier for rolling rollout.
func (a *RollingRolloutActor) GetType() string {
	return common.ActorTypeRollingRollout
}

// GetLogger returns the logger instance for this actor.
func (a *RollingRolloutActor) GetLogger() *zap.Logger {
	return a.logger
}

// Retrieve checks if the desired model is loaded and ready in Triton with retry timeout logic.
func (a *RollingRolloutActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	rolloutstarted := &types.BoolValue{}
	_ = types.UnmarshalAny(condition.Metadata, rolloutstarted)
	if !rolloutstarted.Value {
		return conditionUtils.GenerateFalseCondition(condition, "RollingRolloutNotStarted", "Rolling rollout has not started"), nil
	}

	// Check if the desired model is actually loaded and ready in Triton
	if deployment.Spec.DesiredRevision != nil {
		modelName := deployment.Spec.DesiredRevision.Name

		// Check if model is loaded in inference server
		inferenceServerName := deployment.Spec.GetInferenceServer().Name
		modelReady, err := a.checkModelStatusWithTimeout(ctx, a.logger, modelName, inferenceServerName, deployment.Namespace)
		if err != nil {
			if err.Error() == "health check timeout exceeded" {
				a.logger.Info("Model health check timed out after 10 minutes", zap.String("model", modelName))
				return conditionUtils.GenerateFalseCondition(condition, "ModelHealthCheckTimeout", fmt.Sprintf("Model %s health check timed out after 10 minutes", modelName)), nil
			}
			a.logger.Error("Failed to check model status in Triton", zap.String("model", modelName), zap.Error(err))
			return conditionUtils.GenerateFalseCondition(condition, "ModelHealthCheckError", fmt.Sprintf("Error checking model %s readiness: %v", modelName, err)), nil
		}

		if modelReady {
			a.logger.Info("New model is loaded and ready in Triton", zap.String("model", modelName))
			return conditionUtils.GenerateTrueCondition(condition), nil
		} else {
			a.logger.Info("New model is not yet ready in Triton, continuing to wait", zap.String("model", modelName))
			return conditionUtils.GenerateFalseCondition(condition, "ModelNotReady", fmt.Sprintf("Model %s is loading but not yet ready in Triton", modelName)), nil
		}
	}

	return conditionUtils.GenerateFalseCondition(condition, "RollingRolloutPending", "Rolling rollout is in progress"), nil
}

// Run adds the model to the ConfigMap, triggering inference server to load it.
func (a *RollingRolloutActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running rolling rollout for deployment", zap.String("deployment", deployment.Name))

	if deployment.Spec.DesiredRevision != nil {
		modelName := deployment.Spec.DesiredRevision.Name
		inferenceServerName := deployment.Spec.GetInferenceServer().Name

		a.logger.Info("Syncing model to inference server",
			zap.String("model", modelName),
			zap.String("inference_server", inferenceServerName))

		var err error
		// TODO(#696): ghosharitra: make the storage path configurable w.r.t storage client and storage location
		if err = a.modelConfigProvider.AddModelToConfig(ctx, a.logger, a.client, inferenceServerName, deployment.Namespace, modelconfig.ModelConfigEntry{Name: modelName, StoragePath: fmt.Sprintf("s3://deploy-models/%s/", modelName)}); err != nil {
			a.logger.Error("Failed to initiate model loading", zap.Error(err), zap.String("operation", "load_model"), zap.String("model", modelName), zap.String("inferenceServerName", inferenceServerName), zap.String("namespace", deployment.Namespace), zap.String("backendType", v2pb.BACKEND_TYPE_TRITON.String()))
			return conditionUtils.GenerateFalseCondition(condition, "ModelLoadingFailed", fmt.Sprintf("Failed to update deployment: %v", err)), nil
		}
		rolloutstarted := &types.BoolValue{Value: true}
		condition.Metadata, err = types.MarshalAny(rolloutstarted)
		if err != nil {
			return condition, fmt.Errorf("failed to marshal rolloutstarted condition: %w", err)
		}

		a.logger.Info("Successfully initiated model loading",
			zap.String("operation", "load_model"),
			zap.String("model", modelName),
			zap.String("inferenceServerName", inferenceServerName),
			zap.String("namespace", deployment.Namespace),
			zap.String("backendType", v2pb.BACKEND_TYPE_TRITON.String()))
	}

	// Return unknown so that the condition is only true when the model is truely ready and loaded in triton
	return conditionUtils.GenerateUnknownCondition(condition, "RollingRolloutPending", "Rolling rollout is in progress"), nil
}

// checkModelStatusWithTimeout implements retry logic with configurable timeout for model health checks
func (a *RollingRolloutActor) checkModelStatusWithTimeout(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string) (bool, error) {
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
	modelReady, err := a.gateway.CheckModelStatus(timeoutCtx, logger, a.client, a.HTTPClient, modelName, inferenceServerName, namespace, v2pb.BACKEND_TYPE_TRITON)
	if err == nil && modelReady {
		logger.Info("Model health check succeeded immediately", zap.String("model", modelName))
		return true, nil
	}
	if err != nil {
		logger.Info("Initial model health check failed",
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

			modelReady, err := a.gateway.CheckModelStatus(timeoutCtx, logger, a.client, a.HTTPClient, modelName, inferenceServerName, namespace, v2pb.BACKEND_TYPE_TRITON)
			if err == nil && modelReady {
				logger.Info("Model health check succeeded after retry", zap.String("model", modelName))
				return true, nil
			}

			if err != nil {
				logger.Info("Model health check retry failed, continuing to wait",
					zap.String("model", modelName),
					zap.String("inference_server", inferenceServerName),
					zap.String("namespace", namespace),
					zap.Error(err))
			} else {
				logger.Info("Model still not ready, continuing to wait", zap.String("model", modelName), zap.String("inference_server", inferenceServerName), zap.String("namespace", namespace))
			}
		}
	}
}
