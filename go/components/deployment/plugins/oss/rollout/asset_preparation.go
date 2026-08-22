package rollout

import (
	"context"
	"errors"

	"go.uber.org/zap"

	goapi "github.com/michelangelo-ai/michelangelo/go/api"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &AssetPreparationActor{}

// AssetPreparationActor verifies model artifacts are available in storage before deployment.
type AssetPreparationActor struct {
	apiHandler goapi.Handler
	logger     *zap.Logger
}

// GetType returns the condition type identifier for asset preparation.
func (a *AssetPreparationActor) GetType() string {
	return common.ActorTypeAssetPreparation
}

// Retrieve resolves the desired revision to a Model CR and checks that it points at
// artifacts the inference server can actually load. Failing here keeps a misregistered
// model from reaching the per-cluster rollout, where it would surface only as a timeout.
func (a *AssetPreparationActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if deployment.Spec.DesiredRevision == nil {
		return conditionsutil.GenerateFalseCondition(condition, "NoDesiredRevision", "No desired revision specified for asset preparation"), nil
	}

	// TODO(#619): also confirm the artifacts exist at the resolved path via the blobstore.
	storagePath, err := common.ResolveDeploymentModelStoragePath(ctx, a.apiHandler, deployment)
	if err != nil {
		var resolutionErr *common.ModelResolutionError
		if errors.As(err, &resolutionErr) {
			return conditionsutil.GenerateFalseCondition(condition, resolutionErr.Reason, resolutionErr.Message), nil
		}
		return conditionsutil.GenerateFalseCondition(condition, "ModelResolutionFailed", err.Error()), nil
	}

	a.logger.Info("Resolved model storage path",
		zap.String("model", deployment.Spec.GetDesiredRevision().GetName()),
		zap.String("storagePath", storagePath))
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run prepares model assets for deployment (placeholder for future implementation).
func (a *AssetPreparationActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// nothing actionable for asset preparation, simply return the condition
	return condition, nil
}
