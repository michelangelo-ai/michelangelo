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

// AssetPreparationActor handles asset preparation following Uber patterns
type AssetPreparationActor struct {
	gateway gateways.Gateway
	logger  *zap.Logger
}

func (a *AssetPreparationActor) GetType() string {
	return common.ActorTypeAssetPreparation
}

func (a *AssetPreparationActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if assets are prepared for the desired model
	if deployment.Spec.DesiredRevision == nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "NoDesiredRevision",
			Message: "No desired revision specified for asset preparation",
		}, nil
	}

	// For OSS, we assume assets are available in MinIO/S3 storage
	// In Uber's implementation, this checks TerraBob and validates model assets
	modelName := deployment.Spec.DesiredRevision.Name

	// For OSS, assume assets are always available if the model name is valid
	// In a real implementation, this would check MinIO/S3 for model artifacts
	// TODO(GHOSH): update this to check if the model is available in the storage
	// DO LATER, MAKE ISSUE

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "AssetsAvailable",
		Message: fmt.Sprintf("Assets for model %s are available and prepared", modelName),
	}, nil
}

func (a *AssetPreparationActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running asset preparation for deployment", zap.String("deployment", resource.Name))

	if resource.Spec.DesiredRevision != nil {
		modelName := resource.Spec.DesiredRevision.Name
		a.logger.Info("Preparing assets for model", zap.String("model", modelName))
		// TODO(GHOSH): download the model from the storage and prepare the assets and do the necessary validations
		// DO LATER, MAKE ISSUE
		a.logger.Info("Asset preparation completed", zap.String("model", modelName))
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "Success",
		Message: "Operation completed successfully",
	}, nil
}
