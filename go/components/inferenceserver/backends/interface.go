//go:generate mamockgen Backend

package backends

import (
	"context"

	"go.uber.org/zap"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// InfrastructureStatus represents the current state and health of inference server infrastructure.
type InfrastructureStatus struct {
	State     v2pb.InferenceServerState
	Message   string
	Ready     bool
	Endpoints []string
}

// Backend defines the interface for inference server backend implementations (Triton, vLLM, etc.).
// Each backend provides platform-specific logic for infrastructure and model management.
type Backend interface {
	// CreateInfrastructure provisions backend-specific Kubernetes resources.
	CreateInfrastructure(ctx context.Context, logger *zap.Logger, inferenceServer *v2pb.InferenceServer) (*InfrastructureStatus, error)

	// GetInfrastructureStatus queries the backend-specific infrastructure state.
	GetInfrastructureStatus(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) (*InfrastructureStatus, error)

	// DeleteInfrastructure removes backend-specific Kubernetes resources.
	DeleteInfrastructure(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) error

	// IsHealthy checks backend-specific health endpoints.
	IsHealthy(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) (bool, error)

	// CheckModelStatus checks the status of a model.
	CheckModelStatus(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string) (bool, error)
}
