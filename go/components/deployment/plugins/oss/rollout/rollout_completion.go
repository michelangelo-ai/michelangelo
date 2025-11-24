package rollout

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// RolloutCompletionActor handles post-rollout completion tasks
type RolloutCompletionActor struct {
	client                 client.Client
	gateway                gateways.Gateway
	modelConfigMapProvider configmap.ModelConfigMapProvider
	logger                 *zap.Logger
}

func (a *RolloutCompletionActor) GetType() string {
	return common.ActorTypeRolloutCompletion
}

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

func (a *RolloutCompletionActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running rollout completion tasks for deployment", zap.String("deployment", deployment.Name))

	if deployment.Spec.DesiredRevision != nil {
		// NOW we can safely update CurrentRevision since traffic has been switched
		modelName := deployment.Spec.DesiredRevision.Name
		deployment.Status.CurrentRevision = deployment.Spec.DesiredRevision
		a.logger.Info("CurrentRevision updated after successful traffic switch", zap.String("model", modelName))

		// TODO(GHOSH): SHOULD WE DIRECTLY SET THE STAGE TO ROLLOUT_COMPLETE HERE?
		// OR SHOULD WE LET PARSESTAGE HANDLE THIS?
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
