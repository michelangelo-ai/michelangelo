package steadystate

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// SteadyStateActor monitors deployment health and maintains stable operation after rollout completion.
type SteadyStateActor struct {
	httpClient *http.Client
	gateway    gateways.Gateway
	logger     *zap.Logger
	client     client.Client
}

// GetType returns the condition type identifier for steady state.
func (a *SteadyStateActor) GetType() string {
	return common.ActorTypeSteadyState
}

// Retrieve checks if deployment is in steady state (complete and healthy).
func (a *SteadyStateActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if deployment is in steady state (complete and healthy)
	a.logger.Info("Monitoring steady state for deployment", zap.String("deployment", resource.Name))

	// Check if the inference server is healthy
	healthy, err := a.gateway.InferenceServerIsHealthy(ctx, a.logger, a.client, resource.Spec.GetInferenceServer().Name, resource.Namespace, v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		a.logger.Error("failed to check health of inference server",
			zap.Error(err),
			zap.String("operation", "steady_state_health_check"),
			zap.String("namespace", resource.Namespace),
			zap.String("deployment", resource.Name),
			zap.String("inference_server", resource.Spec.GetInferenceServer().Name))
		return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", fmt.Sprintf("Failed to check health of inference server: %v", err)), nil
	}
	if !healthy {
		return conditionsutil.GenerateFalseCondition(condition, "HealthCheckFailed", "Inference server is not healthy"), nil
	}

	// Check if the desired model is ready in Triton
	modelReady, err := a.gateway.CheckModelStatus(ctx, a.logger, a.client, a.httpClient, resource.Spec.DesiredRevision.Name, resource.Spec.GetInferenceServer().Name, resource.Namespace, v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		a.logger.Error("failed to check model status",
			zap.Error(err),
			zap.String("operation", "steady_state_model_check"),
			zap.String("namespace", resource.Namespace),
			zap.String("deployment", resource.Name),
			zap.String("model", resource.Spec.DesiredRevision.Name))
		return conditionsutil.GenerateFalseCondition(condition, "ModelHealthCheckFailed", fmt.Sprintf("Failed to check model status: %v", err)), nil
	}
	if !modelReady {
		return conditionsutil.GenerateFalseCondition(condition, "ModelHealthCheckFailed", "Model is not ready"), nil
	}

	a.logger.Info("Deployment is in steady state", zap.String("deployment", resource.Name))
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run continuously monitors inference server and model health to maintain steady state.
func (a *SteadyStateActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// nothing actionable for steady state, simply return the condition
	return condition, nil
}
