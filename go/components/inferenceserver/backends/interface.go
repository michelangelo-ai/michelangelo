//go:generate mamockgen Backend

package backends

import (
	"context"

	"go.uber.org/zap"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ServerStatus represents the current state and health of inference server.
type ServerStatus struct {
	State     v2pb.InferenceServerState
	Message   string
	Ready     bool
	Endpoints []string
}

// Backend defines the interface for inference server backend implementations (Triton, vLLM, etc.).
// Each backend provides platform-specific logic for server and model management.
type Backend interface {
	// CreateServer provisions backend-specific Kubernetes resources for an inference server.
	CreateServer(ctx context.Context, logger *zap.Logger, inferenceServer *v2pb.InferenceServer, connectionSpec *v2pb.ConnectionSpec) (*ServerStatus, error)
	// GetServerStatus queries the backend-specific server state.
	GetServerStatus(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string, connectionSpec *v2pb.ConnectionSpec) (*ServerStatus, error)
	// DeleteServer removes backend-specific Kubernetes resources for an inference server.
	DeleteServer(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string, connectionSpec *v2pb.ConnectionSpec) error
	// IsHealthy checks backend-specific health endpoints for an inference server.
	IsHealthy(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string, connectionSpec *v2pb.ConnectionSpec) (bool, error)
	// CheckModelStatus checks the status of a model on an inference server.
	CheckModelStatus(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, connectionSpec *v2pb.ConnectionSpec) (bool, error)
}
