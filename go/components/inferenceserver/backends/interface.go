//go:generate mamockgen Backend

package backends

import (
	"context"
	"net/http"

	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// ServerStatus communicates inference server state back to the controller.
type ServerStatus struct {
	State     v2pb.InferenceServerState
	Endpoints []string
}

// Backend abstracts inference server provisioning for different frameworks (Triton, vLLM, etc.).
// All methods must be idempotent.
type Backend interface {
	// CreateServer provisions infrastructure for an inference server and returns the current state.
	// Calling it for an existing server reconciles the server's pod template with what the
	// backend would create today, so probe and argument changes reach running servers.
	CreateServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServer *v2pb.InferenceServer) (*ServerStatus, error)
	// GetServerStatus returns the current state of an inference server.
	GetServerStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (*ServerStatus, error)
	// DeleteServer removes all resources for an inference server.
	DeleteServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) error
	// IsHealthy reports whether the inference server can accept requests.
	IsHealthy(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (bool, error)
	// GetModelStatus reports, replica by replica, whether a model is loaded and ready for
	// inference. The probe goes through the cluster's API server so it works for remote
	// clusters the controller has no network path into.
	GetModelStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, httpClient *http.Client, apiServerURL string, inferenceServerName string, namespace string, modelName string) (*ModelStatus, error)
}
