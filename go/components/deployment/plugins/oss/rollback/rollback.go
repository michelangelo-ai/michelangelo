package rollback

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &RollbackActor{}

// RollbackActor restores deployment to the previous stable revision when rollout fails.
type RollbackActor struct {
	client              client.Client
	modelConfigProvider modelconfig.ModelConfigProvider
	logger              *zap.Logger
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

	a.logger.Info("Checking if rollback is required", zap.String("candidate_model", candidateModel))
	// Use the model config as the source of truth for model existence. A model removed from
	// the config may briefly remain in the inference server until the sidecar unloads it.
	if exists, err := common.CheckModelExists(ctx, a.logger, a.modelConfigProvider, a.client, candidateModel, resource.Spec.GetInferenceServer().GetName(), resource.GetNamespace()); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "UnableToCheckModelExistsInModelConfig", fmt.Sprintf("Unable to check if model %s exists in model config: %v", candidateModel, err)), nil
	} else if exists {
		return conditionsutil.GenerateFalseCondition(condition, "ModelStillExistsInModelConfig", fmt.Sprintf("Candidate Model %s still exists in model config", candidateModel)), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run removes the candidate model from the model config.
func (a *RollbackActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	inferenceServerName := resource.Spec.GetInferenceServer().GetName()
	candidateModel := resource.Status.CandidateRevision.GetName()
	a.logger.Info("Removing candidate model from model config",
		zap.String("candidate_model", candidateModel),
		zap.String("inference_server", inferenceServerName))

	if err := a.modelConfigProvider.RemoveModelFromConfig(ctx, a.logger, a.client, inferenceServerName, resource.Namespace, candidateModel); err != nil {
		a.logger.Error("Failed to remove candidate model from model config", zap.String("model", candidateModel), zap.Error(err))
		return conditionsutil.GenerateFalseCondition(condition, "RemoveCandidateModelFromModelConfigFailed", fmt.Sprintf("Failed to remove candidate model %s from model config: %v", candidateModel, err)), nil
	}

	return conditionsutil.GenerateTrueCondition(condition), nil
}
