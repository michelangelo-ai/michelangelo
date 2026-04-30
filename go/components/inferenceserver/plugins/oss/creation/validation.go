package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &ValidationActor{}

// ValidationActor validates that inference server configuration meets requirements.
type ValidationActor struct {
	registry *backends.Registry
	logger   *zap.Logger
}

// NewValidationActor creates a condition actor for inference server configuration validation.
func NewValidationActor(registry *backends.Registry, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &ValidationActor{
		registry: registry,
		logger:   logger,
	}
}

// GetType returns the condition type identifier for validation.
func (a *ValidationActor) GetType() string {
	return common.ValidationConditionType
}

// Retrieve validates that the inference server configuration meets backend requirements.
func (a *ValidationActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving inference server validation condition")

	// Validate that the backend type is registered in the registry
	_, err := a.registry.GetBackend(resource.Spec.BackendType)
	if err != nil {
		return conditionUtils.GenerateFalseCondition(condition, "InvalidBackendType", fmt.Sprintf("unsupported backend type: %v", resource.Spec.BackendType)), nil
	}

	if len(resource.Spec.ClusterTargets) == 0 {
		return conditionUtils.GenerateFalseCondition(condition, "NoClusterTargets", "spec.cluster_targets must declare at least one cluster"), nil
	}

	// Validate cluster rollout strategy annotation before operational actors attempt multi-cluster iteration.
	if strategy := common.GetRolloutStrategy(resource); !common.IsKnownRolloutStrategy(strategy) {
		return conditionUtils.GenerateFalseCondition(condition, "InvalidRolloutStrategy",
			fmt.Sprintf("unknown cluster rollout strategy %q; supported: rolling", strategy)), nil
	}

	return conditionUtils.GenerateTrueCondition(condition), nil
}

// Run returns a failed condition since validation failures cannot be automatically fixed.
func (a *ValidationActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	// This method is only run when Retrieve() fails.
	// If Retrieve() failed, then there's nothing we can do here, simply return the condition.
	return condition, nil
}
