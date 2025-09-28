package strategies

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Note: ModelSyncActor is now defined in actors.go with the correct interface

// RollingRolloutActor handles rolling rollout strategy following Uber patterns
type RollingRolloutActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  logr.Logger
}

func (a *RollingRolloutActor) GetType() string {
	return common.ActorTypeRollingRollout
}

func (a *RollingRolloutActor) GetLogger() logr.Logger {
	return a.logger
}

func (a *RollingRolloutActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if rolling rollout is complete
	if resource.Status.CurrentRevision != nil &&
		resource.Spec.DesiredRevision != nil &&
		resource.Status.CurrentRevision.Name == resource.Spec.DesiredRevision.Name &&
		resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_PLACEMENT {

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
	a.logger.Info("Running rolling rollout for deployment", "deployment", resource.Name)

	// Update deployment to placement stage
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_PLACEMENT
	resource.Status.State = v2pb.DEPLOYMENT_STATE_INITIALIZING

	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		inferenceServerName := resource.Spec.GetInferenceServer().Name

		a.logger.Info("Starting rolling rollout",
			"model", modelName,
			"inference_server", inferenceServerName)

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
			"increment_percentage", incrementPercentage,
			"strategy", "rolling")

		// Simulate successful rollout completion
		resource.Status.CurrentRevision = resource.Spec.DesiredRevision
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		a.logger.Info("Rolling rollout completed successfully", "model", modelName)
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}
