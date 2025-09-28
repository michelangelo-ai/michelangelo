package strategies

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetBlastActors returns actors for blast rollout strategy (all-at-once deployment)
func GetBlastActors(params Params, deployment *v2pb.Deployment) []plugins.ConditionActor {
	return []plugins.ConditionActor{
		&BlastRolloutActor{
			client:  params.Client,
			gateway: params.Gateway,
			logger:  params.Logger,
		},
	}
}

// BlastRolloutActor implements all-at-once deployment strategy
type BlastRolloutActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  logr.Logger
}

func (a *BlastRolloutActor) GetType() string {
	return common.ActorTypeBlastRollout
}

func (a *BlastRolloutActor) GetLogger() logr.Logger {
	return a.logger
}

func (a *BlastRolloutActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if blast rollout is complete
	if resource.Status.CurrentRevision != nil &&
		resource.Spec.DesiredRevision != nil &&
		resource.Status.CurrentRevision.Name == resource.Spec.DesiredRevision.Name &&
		resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE {

		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "BlastRolloutCompleted",
			Message: "Blast rollout completed successfully",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "BlastRolloutPending",
		Message: "Blast rollout has not started yet",
	}, nil
}

func (a *BlastRolloutActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running blast rollout for deployment", "deployment", resource.Name)

	// Update deployment to placement stage
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_PLACEMENT
	resource.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING

	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name

		a.logger.Info("Starting blast rollout",
			"model", modelName,
			"inference_server", inferenceServerName)

		// Perform immediate 100% rollout - update all replicas at once
		updateRequest := gateways.ModelConfigUpdateRequest{
			InferenceServer: inferenceServerName,
			Namespace:       resource.Namespace,
			BackendType:     v2pb.BACKEND_TYPE_TRITON, // Default to Triton for OSS
			ModelConfigs: []gateways.ModelConfigEntry{
				{
					Name:   modelName,
					S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName),
				},
			},
		}

		if err := a.gateway.UpdateModelConfig(ctx, a.logger, updateRequest); err != nil {
			a.logger.Error(err, "Failed to update model config for blast rollout")
			return &apipb.Condition{
				Type:    a.GetType(),
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "BlastRolloutFailed",
				Message: fmt.Sprintf("Failed to update model config: %v", err),
			}, nil
		}

		// Simulate blast rollout completion
		resource.Status.CurrentRevision = resource.Spec.DesiredRevision
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		a.logger.Info("Blast rollout completed successfully", "model", modelName)
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}