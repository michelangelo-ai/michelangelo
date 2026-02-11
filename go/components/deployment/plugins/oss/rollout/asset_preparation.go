package rollout

import (
	"context"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &AssetPreparationActor{}

// AssetPreparationActor verifies model artifacts are available in storage before deployment.
type AssetPreparationActor struct {
	logger *zap.Logger
}

// GetType returns the condition type identifier for asset preparation.
func (a *AssetPreparationActor) GetType() string {
	return common.ActorTypeAssetPreparation
}

// Retrieve checks if model assets are available in storage (MinIO/S3).
func (a *AssetPreparationActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if assets are prepared for the desired model
	if deployment.Spec.DesiredRevision == nil {
		return conditionsutil.GenerateFalseCondition(condition, "NoDesiredRevision", "No desired revision specified for asset preparation"), nil
	}

	// TODO(#619): ghosharitra: update this to check if the model is available in the storage
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run prepares model assets for deployment (placeholder for future implementation).
func (a *AssetPreparationActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// nothing actionable for asset preparation, simply return the condition
	return condition, nil
}
