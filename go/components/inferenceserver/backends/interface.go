//go:generate mamockgen Backend

package backends

import (
	"context"
	"net/http"

	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// ServerStatus represents the current state and health of inference server.
type ServerStatus struct {
	State     v2pb.InferenceServerState
	Endpoints []string
}

// Backend defines the interface for inference server backend implementations (Triton, vLLM, etc.).
// Each backend provides platform-specific logic for server and model management.
type Backend interface {
	// CreateServer provisions backend-specific Kubernetes resources for an inference server.
	CreateServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServer *v2pb.InferenceServer) (*ServerStatus, error)
	// GetServerStatus queries the backend-specific server state.
	GetServerStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (*ServerStatus, error)
	// DeleteServer removes backend-specific Kubernetes resources for an inference server.
	DeleteServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) error
	// IsHealthy checks backend-specific health endpoints for an inference server.
	IsHealthy(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (bool, error)
	// CheckModelStatus checks the status of a model on an inference server.
	CheckModelStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, httpClient *http.Client, inferenceServerName string, namespace string, modelName string) (bool, error)

	// GetFrontEndSvc ---> should return the service that we link to the deployment route.
	// LoadModel/UnloadModel is also needed for direct loading/unloading of models.
}
