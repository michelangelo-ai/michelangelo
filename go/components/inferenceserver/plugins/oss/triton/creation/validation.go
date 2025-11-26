package creation

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &ValidationActor{}

// ValidationActor validates Triton-specific configuration
type ValidationActor struct {
	gateway       gateways.Gateway
	proxyProvider proxy.ProxyProvider
	logger        *zap.Logger
}

func NewValidationActor(gateway gateways.Gateway, logger *zap.Logger, proxyProvider proxy.ProxyProvider) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &ValidationActor{
		gateway:       gateway,
		logger:        logger,
		proxyProvider: proxyProvider,
	}
}

func (a *ValidationActor) GetType() string {
	return "TritonValidation"
}

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

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "ValidationSucceeded",
		Message: "Triton configuration is valid",
	}, nil
}

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
