package rollout

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// RolloutCompletionActor finalizes deployment by updating CurrentRevision and cleaning up rollout metadata.
type RolloutCompletionActor struct {
	gateway gateways.Gateway
	logger  *zap.Logger
}

// GetType returns the condition type identifier for rollout completion.
func (a *RolloutCompletionActor) GetType() string {
	return common.ActorTypeRolloutCompletion
}

// Retrieve checks if the deployment has reached rollout complete stage with healthy state.
func (a *RolloutCompletionActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE &&
		resource.Status.State == v2pb.DEPLOYMENT_STATE_HEALTHY {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "CompletionTasksFinished",
			Message: "All rollout completion tasks have been successfully executed",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "CompletionTasksPending",
		Message: "Rollout completion tasks are pending",
	}, nil
}

// Run updates CurrentRevision to DesiredRevision and removes temporary rollout annotations.
func (a *RolloutCompletionActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running rollout completion tasks for deployment", zap.String("deployment", deployment.Name))

	if deployment.Spec.DesiredRevision != nil {
		// Now we can safely update CurrentRevision since traffic has been switched
		modelName := deployment.Spec.DesiredRevision.Name
		deployment.Status.CurrentRevision = deployment.Spec.DesiredRevision
		a.logger.Info("CurrentRevision updated after successful traffic switch", zap.String("model", modelName))

		deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		deployment.Status.Message = fmt.Sprintf("Rollout completed successfully for model %s", modelName)

		// Clean up any temporary annotations or metadata
		if deployment.Annotations != nil {
			// Remove rollout-specific annotations
			delete(deployment.Annotations, "rollout.michelangelo.ai/in-progress")
			delete(deployment.Annotations, "rollout.michelangelo.ai/start-time")
		}

		a.logger.Info("Rollout completion tasks finished successfully", zap.String("model", modelName))
	}

	return &apipb.Condition{Type: a.GetType(), Status: apipb.CONDITION_STATUS_TRUE, Reason: "Success", Message: "Operation completed successfully"}, nil
}
