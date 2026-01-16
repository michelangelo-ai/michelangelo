package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &ValidationActor{}

// ValidationActor validates that inference server configuration meets Triton requirements.
type ValidationActor struct {
	backend backends.Backend
	logger  *zap.Logger
}

// NewValidationActor creates a condition actor for Triton configuration validation.
func NewValidationActor(backend backends.Backend, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &ValidationActor{
		backend: backend,
		logger:  logger,
	}
}

// GetType returns the condition type identifier for validation.
func (a *ValidationActor) GetType() string {
	return common.TritonValidationConditionType
}

// Retrieve validates that the inference server configuration meets Triton backend requirements.
func (a *ValidationActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Retrieving Triton validation condition")

	// Validate Triton-specific requirements
	if resource.Spec.BackendType != v2pb.BACKEND_TYPE_TRITON {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_FALSE,
			Reason:  "InvalidBackendType",
			Message: fmt.Sprintf("invalid backend type for Triton plugin: %v", resource.Spec.BackendType),
		}, nil
	}

	// todo: ghosharitra: add validation for the cluster targets

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "ValidationSucceeded",
		Message: "Triton configuration is valid",
	}, nil
}

// Run returns a failed condition since validation failures cannot be automatically fixed.
func (a *ValidationActor) Run(ctx context.Context, resource *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	// This method is only ran when Retrieve() fails.
	// If Retrieve() failed, then there's nothing we can do here, simply return false condition.
	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "ValidationFailed",
		Message: "Triton configuration is invalid",
	}, nil
}
