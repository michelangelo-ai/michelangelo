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
//
// Triton artifacts are already deployment-ready when the packaging step uploads them, so
// validating the reference is the whole job; there is nothing to stage or compile. The
// internal controller reaches the same conclusion from the other direction: it returns
// TRUE immediately for every non-Spark package type, because its download-compile-upload
// path exists only to package Spark pipeline models.
func (a *AssetPreparationActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if deployment.Spec.DesiredRevision == nil {
		return conditionsutil.GenerateFalseCondition(condition, "NoDesiredRevision", "No desired revision specified for asset preparation"), nil
	}

	// TODO(#619): confirm the artifact is present and well-formed at the resolved path.
	// This needs an existence primitive the blobstore does not have yet: BlobStoreClient
	// exposes only Get and Scheme, and a resolved path may be a prefix rather than a single
	// object, so a stat alone would not cover both shapes. Probing for config.pbtxt beneath
	// the prefix would establish presence and layout together, turning a malformed model
	// into a condition here rather than an opaque Triton "failed to poll from model
	// repository" midway through the rollout.
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

// Run is a no-op. The artifact is produced and uploaded by the packaging step long before
// a Deployment references it, so this actor is purely a validation gate: anything it could
// discover belongs in Retrieve, and there is no state for it to advance.
func (a *AssetPreparationActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	return condition, nil
}
