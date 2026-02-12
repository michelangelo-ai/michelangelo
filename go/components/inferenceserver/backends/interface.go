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

	// LoadModel loads a LoRA adapter onto the inference server.
	// For Dynamo backends, this creates a DynamoModel CR.
	// modelName is the identifier used in inference requests, sourcePath is the model location (e.g., "s3://bucket/path", "hf://org/model").
	LoadModel(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string, modelName string, sourcePath string) error
	// UnloadModel removes a LoRA adapter from the inference server.
	// For Dynamo backends, this deletes the DynamoModel CR.
	UnloadModel(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string, modelName string) error
	// GetFrontEndSvc returns the frontend service name for routing traffic.
	GetFrontEndSvc(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) (string, error)
	// GetFrontEndSvc ---> should return the service that we link to the deployment route.
	// LoadModel/UnloadModel is also needed for direct loading/unloading of models.
}
