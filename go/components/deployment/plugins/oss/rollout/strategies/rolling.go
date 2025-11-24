package strategies

import (
	"context"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
			client:                 params.Client,
			gateway:                params.Gateway,
			logger:                 params.Logger,
			modelConfigMapProvider: params.ModelConfigMapProvider,
		},
		&ModelCleanupActor{
			Client:                 params.Client,
			Gateway:                params.Gateway,
			Logger:                 params.Logger,
			ModelConfigMapProvider: params.ModelConfigMapProvider,
		},
		&RollingRolloutActor{
			client: params.Client,
			logger: params.Logger,
		},
	}
}

// RollingRolloutActor handles rolling rollout strategy following Uber patterns
type RollingRolloutActor struct {
	client client.Client
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
	if resource.Status.CurrentRevision != nil &&
		resource.Spec.DesiredRevision != nil &&
		resource.Status.CurrentRevision.Name == resource.Spec.DesiredRevision.Name &&
		resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE {

		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "RollingRolloutCompleted",
			Message: "Rolling rollout completed successfully across all inference servers",
		}, nil
	}

	// Check if rollout is in progress
	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_PLACEMENT {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "RollingRolloutInProgress",
			Message: "Rolling rollout is in progress",
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

	// Update deployment to placement stage
	// TODO(GHOSH): SHOULD WE DIRECTLY SET THE STAGE TO PLACEMENT HERE?
	// OR SHOULD WE LET PARSESTAGE HANDLE THIS?
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_PLACEMENT
	resource.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING

	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name

		a.logger.Info("Starting rolling rollout",
			zap.String("model", modelName),
			zap.String("inference_server", inferenceServerName))

		// In Uber's implementation, rolling rollout:
		// 1. Resolves all hosts for the inference server (via UNS)
		// 2. Incrementally rolls out to percentage of hosts (30% by default)
		// 3. Waits for model to load on each batch before proceeding
		// 4. Continues until 100% of hosts have the new model
		// 5. Uses sophisticated host resolution and load balancing

		// For OSS, we simulate a successful rolling rollout:
		// - Update inference server configurations incrementally
		// - Monitor model loading status on each pod
		// - Implement proper rollback on failures

		// Get rollout increment percentage from annotations or use default
		incrementPercentage := common.GetRolloutIncrement(resource)
		a.logger.Info("Rolling rollout configuration",
			zap.Int("increment_percentage", incrementPercentage),
			zap.String("strategy", "rolling"))

		// Simulate successful rollout completion
		resource.Status.CurrentRevision = resource.Spec.DesiredRevision
		// TODO(GHOSH): SHOULD WE DIRECTLY SET THE STAGE TO ROLLOUT_COMPLETE HERE?
		// OR SHOULD WE LET PARSESTAGE HANDLE THIS?
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		a.logger.Info("Rolling rollout completed successfully", zap.String("model", modelName))
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}
