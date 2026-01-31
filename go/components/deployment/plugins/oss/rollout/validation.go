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

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &ValidationActor{}

// ValidationActor validates deployment configuration before rollout begins.
type ValidationActor struct {
	logger *zap.Logger
}

// GetType returns the condition type identifier for validation.
func (a *ValidationActor) GetType() string {
	return common.ActorTypeValidation
}

// Retrieve checks if deployment configuration has valid model and inference server references.
func (a *ValidationActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Validate deployment configuration
	if resource.Spec.GetInferenceServer() == nil {
		return conditionsutil.GenerateFalseCondition(condition, "NoInferenceServer", "Inference server is not specified for deployment"), nil
	}
	if resource.Spec.DesiredRevision == nil {
		return conditionsutil.GenerateFalseCondition(condition, "NoDesiredRevision", "Desired revision is not specified for deployment"), nil
	}
	if resource.Spec.DesiredRevision.Name == "" {
		return conditionsutil.GenerateFalseCondition(condition, "InvalidModelName", "Desired revision name cannot be empty"), nil
	}
	a.logger.Info("Validation completed successfully", zap.String("deployment", resource.Name))
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run performs comprehensive validation and updates deployment stage and state.
func (a *ValidationActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// nothing actionable for validation, simply return the condition
	return condition, nil
}
