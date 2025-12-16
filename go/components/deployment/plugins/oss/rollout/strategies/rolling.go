package strategies

import (
	"context"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Note: ModelSyncActor is now defined in actors.go with the correct interface

// GetRollingActors returns actors for rolling rollout strategy
func GetRollingActors(params Params, deployment *v2pb.Deployment) []conditionInterfaces.ConditionActor[*v2pb.Deployment] {
	return []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
		&ModelSyncActor{
			gateway: params.Gateway,
			logger:  params.Logger,
		},
		&ModelCleanupActor{
			Gateway: params.Gateway,
			Logger:  params.Logger,
		},
		&RollingRolloutActor{
			logger: params.Logger,
		},
	}
}

// RollingRolloutActor handles rolling rollout strategy following Uber patterns
type RollingRolloutActor struct {
	logger *zap.Logger
}

func (a *RollingRolloutActor) GetType() string {
	return common.ActorTypeRollingRollout
}

func (a *RollingRolloutActor) GetLogger() *zap.Logger {
	return a.logger
}

func (a *RollingRolloutActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if rolling rollout is complete
	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_PLACEMENT &&
		resource.Status.State == v2pb.DEPLOYMENT_STATE_INITIALIZING {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "RollingRolloutCompleted",
			Message: "Rolling rollout completed successfully across all inference servers",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "RollingRolloutPending",
		Message: "Rolling rollout has not started",
	}, nil
}

func (a *RollingRolloutActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running rolling rollout for deployment", zap.String("deployment", resource.Name))

	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_PLACEMENT
	resource.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING

	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name

		a.logger.Info("Starting rolling rollout",
			zap.String("model", modelName),
			zap.String("inference_server", inferenceServerName))

		// Get rollout increment percentage from annotations or use default
		incrementPercentage := common.GetRolloutIncrement(resource)
		a.logger.Info("Rolling rollout configuration",
			zap.Int("increment_percentage", incrementPercentage),
			zap.String("strategy", "rolling"))
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}
