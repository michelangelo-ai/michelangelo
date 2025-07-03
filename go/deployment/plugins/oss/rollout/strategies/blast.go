package strategies

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetBlastActors returns actors for blast rollout strategy (all-at-once deployment)
func GetBlastActors(params Params, deployment *v2pb.Deployment) []plugins.ConditionActor {
	return []plugins.ConditionActor{
		&ModelSyncActor{
			client:  params.Client,
			gateway: params.Gateway,
			logger:  params.Logger,
		},
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
	gateway inferenceserver.Gateway
	logger  logr.Logger
}

func (a *BlastRolloutActor) GetType() string {
	return common.ActorTypeBlastRollout
}

func (a *BlastRolloutActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, deployment *v2pb.Deployment, existingCondition *apipb.Condition) (*apipb.Condition, error) {
	condition := &apipb.Condition{
		Type:   "BlastRollout",
		Status: apipb.CONDITION_STATUS_FALSE,
		Reason: "BlastRolloutInProgress",
	}

	if existingCondition != nil {
		condition = existingCondition
	}

	a.logger.Info("Retrieved blast rollout condition", "status", condition.Status, "reason", condition.Reason)
	return condition, nil
}

func (a *BlastRolloutActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, deployment *v2pb.Deployment, condition *apipb.Condition) error {
	a.logger.Info("Starting blast rollout", "deployment", deployment.Name, "inferenceServer", deployment.Spec.GetInferenceServer().Name)

	// Get model information
	modelName := deployment.Spec.DesiredRevision.Name
	if modelName == "" {
		modelName = deployment.Name
	}

	// Get the inference server
	inferenceServerName := deployment.Spec.GetInferenceServer().Name
	namespace := deployment.Namespace

	// Perform immediate 100% rollout - update all replicas at once
	updateRequest := inferenceserver.ModelConfigUpdateRequest{
		InferenceServer: inferenceServerName,
		Namespace:       namespace,
		BackendType:     v2pb.BACKEND_TYPE_TRITON, // Default to Triton for OSS
		ModelConfigs: []inferenceserver.ModelConfigEntry{
			{
				Name:   modelName,
				S3Path: fmt.Sprintf("s3://deploy-models/%s/", modelName),
			},
		},
	}

	if err := a.gateway.UpdateModelConfig(ctx, a.logger, updateRequest); err != nil {
		condition.Status = apipb.CONDITION_STATUS_FALSE
		condition.Reason = "BlastRolloutFailed"
		condition.Message = fmt.Sprintf("Failed to update model config: %v", err)
		return err
	}

	// Update deployment status to indicate blast rollout completion
	condition.Status = apipb.CONDITION_STATUS_TRUE
	condition.Reason = "BlastRolloutCompleted"
	condition.Message = "Model deployed to all replicas simultaneously"

	a.logger.Info("Blast rollout completed successfully", "model", modelName, "inferenceServer", inferenceServerName)
	return nil
}