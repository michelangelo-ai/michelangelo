package rollback

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &RollbackActor{}

// RollbackActor restores deployment to the previous stable revision when rollout fails.
type RollbackActor struct {
	logger  *zap.Logger
	gateway gateways.Gateway
}

// GetType returns the condition type identifier for rollback.
func (a *RollbackActor) GetType() string {
	return common.ActorTypeRollback
}

// Retrieve checks if rollback is required by verifying whether CandidateRevision still exists.
func (a *RollbackActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	candidateModel := resource.Status.CandidateRevision.GetName()
	if candidateModel == "" {
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	a.logger.Info("Rollback in progress", zap.String("candidate_model", candidateModel))
	if exists, err := a.gateway.CheckModelExists(ctx, a.logger, candidateModel, resource.Spec.GetInferenceServer().GetName(), resource.GetNamespace(), v2pb.BACKEND_TYPE_TRITON); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "UnableToCheckModelExists", fmt.Sprintf("Unable to check if model %s exists in Inference Server: %v", candidateModel, err)), nil
	} else if exists {
		return conditionsutil.GenerateFalseCondition(condition, "ModelStillExistsInInferenceServer", fmt.Sprintf("Candidate Model %s still exists in Inference Server", candidateModel)), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run unloads the candidate model from the inference server.
func (a *RollbackActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	inferenceServerName := resource.Spec.GetInferenceServer().GetName()
	candidateModel := resource.Status.CandidateRevision.GetName()
	a.logger.Info("Starting deployment rollback",
		zap.String("candidate_model", candidateModel),
		zap.String("inference_server", inferenceServerName))

	if err := a.gateway.UnloadModel(ctx, a.logger, candidateModel, inferenceServerName, resource.Namespace, v2pb.BACKEND_TYPE_TRITON); err != nil {
		a.logger.Error("Failed to rollback deployment", zap.String("model", candidateModel), zap.Error(err))
		return conditionsutil.GenerateFalseCondition(condition, "RollbackFailed", fmt.Sprintf("Failed to rollback deployment: %v", err)), nil
	}

	return conditionsutil.GenerateTrueCondition(condition), nil
}
