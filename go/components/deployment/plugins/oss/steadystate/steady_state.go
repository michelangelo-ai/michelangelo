package steadystate

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// SteadyStateActor monitors deployment health and maintains stable operation after rollout completion.
type SteadyStateActor struct {
	gateway gateways.Gateway
	logger  *zap.Logger
}

// GetType returns the condition type identifier for steady state.
func (a *SteadyStateActor) GetType() string {
	return common.ActorTypeSteadyState
}

// Retrieve checks if deployment is in steady state (complete and healthy).
func (a *SteadyStateActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if deployment is in steady state (complete and healthy)
	a.logger.Info("Monitoring steady state for deployment", zap.String("deployment", resource.Name))
	deploymentTargetInfo, err := a.gateway.GetDeploymentTargetInfo(ctx, a.logger, resource.Spec.GetInferenceServer().Name, resource.Namespace)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "GetDeploymentTargetInfoFailed", fmt.Sprintf("Failed to get deployment target info: %v", err)), nil
	}

	// Check if the inference server is healthy for all target clusters
	for _, targetCluster := range deploymentTargetInfo.ClusterTargets {
		healthy, err := a.gateway.InferenceServerIsHealthy(ctx, a.logger, resource.Spec.GetInferenceServer().Name, resource.Namespace, targetCluster, deploymentTargetInfo.BackendType)
		if err != nil {
			a.logger.Error("failed to check health of inference server",
				zap.Error(err),
				zap.String("operation", "steady_state_health_check"),
				zap.String("namespace", resource.Namespace),
				zap.String("deployment", resource.Name),
				zap.String("inference_server", resource.Spec.GetInferenceServer().Name),
				zap.String("cluster_id", targetCluster.ClusterId))
			return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Failed to check health of inference server: %v", err)), nil
		}
		if !healthy {
			return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Inference server is not healthy in cluster %s", targetCluster.ClusterId)), nil
		}

		// Check if the desired model is ready in the target cluster
		modelReady, err := a.gateway.CheckModelStatus(ctx, a.logger, resource.Spec.DesiredRevision.Name, resource.Spec.GetInferenceServer().Name, resource.Namespace, targetCluster, deploymentTargetInfo.BackendType)
		if err != nil {
			a.logger.Error("failed to check model status",
				zap.Error(err),
				zap.String("operation", "steady_state_model_check"),
				zap.String("namespace", resource.Namespace),
				zap.String("deployment", resource.Name),
				zap.String("model", resource.Spec.DesiredRevision.Name),
				zap.String("cluster_id", targetCluster.ClusterId))
			return conditionsutil.GenerateFalseCondition(condition, "ModelHealthCheckFailed", fmt.Sprintf("Failed to check model status in cluster %s: %v", targetCluster.ClusterId, err)), nil
		}
		if !modelReady {
			return conditionsutil.GenerateFalseCondition(condition, "ModelHealthCheckFailed", fmt.Sprintf("Model is not ready in cluster %s", targetCluster.ClusterId)), nil
		}
	}

	a.logger.Info("Deployment is in steady state", zap.String("deployment", resource.Name))
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run continuously monitors inference server and model health to maintain steady state.
func (a *SteadyStateActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// nothing actionable for steady state, simply return the condition
	return condition, nil
}
