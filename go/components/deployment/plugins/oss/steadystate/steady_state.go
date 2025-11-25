package steadystate

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// SteadyStateActor handles steady state monitoring following Uber patterns
type SteadyStateActor struct {
	gateway gateways.Gateway
	logger  *zap.Logger
}

func (a *SteadyStateActor) GetType() string {
	return common.ActorTypeSteadyState
}

func (a *SteadyStateActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if deployment is in steady state (complete and healthy)
	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE &&
		resource.Status.State == v2pb.DEPLOYMENT_STATE_HEALTHY {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "SteadyStateReached",
			Message: "Deployment is in steady state",
		}, nil
	}
	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "SteadyStateRestored",
			Message: "Deployment has been restored to steady state",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "NotInSteadyState",
		Message: "Deployment not yet in steady state",
	}, nil
}

func (a *SteadyStateActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// steady state plugin will only run if the deployment is in rollout/rollback complete stage.
	a.logger.Info("Monitoring steady state for deployment", zap.String("deployment", resource.Name))

	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE {
		// In Uber's implementation, steady state monitoring involves:
		// 1. Continuous health monitoring of inference servers
		// 2. Model performance metrics validation
		// 3. Resource utilization monitoring
		// 4. Automatic drift detection and correction
		// 5. SLA compliance monitoring
		// 6. Integration with MES for model lifecycle management

		// For OSS, actively monitor and maintain steady state
		if resource.Status.State != v2pb.DEPLOYMENT_STATE_HEALTHY {
			a.logger.Info("Deployment not healthy, investigating", zap.String("state", resource.Status.State.String()))
			// Check if the inference server is healthy
			if healthy, err := a.gateway.IsHealthy(ctx, a.logger, gateways.HealthCheckRequest{
				InferenceServer: resource.Spec.GetInferenceServer().Name,
				Namespace:       resource.Namespace,
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			}); err != nil {
				return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "HealthCheckFailed", Message: fmt.Sprintf("Failed to check health of inference server: %v", err)}, nil
			} else if !healthy {
				return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "HealthCheckFailed", Message: "Inference server is not healthy"}, nil
			}

			// Check if the desired model is ready in Triton
			modelStatusRequest := gateways.CheckModelStatusRequest{
				ModelName:       resource.Spec.DesiredRevision.Name,
				InferenceServer: resource.Spec.GetInferenceServer().Name,
				DeploymentName:  resource.Name,
				Namespace:       resource.Namespace,
				BackendType:     v2pb.BACKEND_TYPE_TRITON,
			}
			fmt.Printf("MADE IT TO HERE: modelStatusRequest: %+v\n", modelStatusRequest)
			modelReady, err := a.gateway.CheckModelStatus(ctx, a.logger, modelStatusRequest)
			if err != nil {
				return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "ModelHealthCheckFailed", Message: fmt.Sprintf("Failed to check model status: %v", err)}, nil
			}
			if !modelReady {
				return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_FALSE, Reason: "ModelHealthCheckFailed", Message: "Model is not ready"}, nil
			}
			resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		}

		// Ensure current revision matches desired revision
		if resource.Status.CurrentRevision != nil && resource.Spec.DesiredRevision != nil {
			if resource.Status.CurrentRevision.Name != resource.Spec.DesiredRevision.Name {
				a.logger.Info("Revision mismatch detected, needs reconciliation",
					zap.String("current", resource.Status.CurrentRevision.Name),
					zap.String("desired", resource.Spec.DesiredRevision.Name))
			}
		}

	}

	a.logger.Info("Deployment is in steady state", zap.String("deployment", resource.Name))
	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "SteadyStateReached",
		Message: "Deployment is in steady state",
	}, nil
}
