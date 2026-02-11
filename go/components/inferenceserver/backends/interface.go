//go:generate mamockgen Backend

package backends

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// ServerStatus represents the current state and health of inference server.
type ServerStatus struct {
	ClusterState v2pb.ClusterState
	Endpoint     string
}

type ResourceConstraints struct {
	Cpu      int32
	Gpu      int32
	Memory   string
	Replicas int32
}

// Backend defines the interface for inference server backend implementations (Triton, vLLM, etc.).
// Each backend provides platform-specific logic for server and model management.
type Backend interface {
	// CreateServer provisions backend-specific Kubernetes resources for an inference server. If targetCluster is nil, the inference server will be created in the control plane cluster.
	CreateServer(ctx context.Context, inferenceServerName, namespace string, resourceConstraints ResourceConstraints, targetCluster *v2pb.ClusterTarget) (*ServerStatus, error)
	// GetServerStatus queries the backend-specific server state. If targetCluster is nil, the inference server status will be returned for the control plane cluster.
	GetServerStatus(ctx context.Context, inferenceServerName, namespace string, targetCluster *v2pb.ClusterTarget) (*ServerStatus, error)
	// DeleteServer removes backend-specific Kubernetes resources for an inference server. If targetCluster is nil, the inference server will be deleted from the control plane cluster.
	DeleteServer(ctx context.Context, inferenceServerName, namespace string, targetCluster *v2pb.ClusterTarget) error
	// IsHealthy checks backend-specific health endpoints for an inference server. If targetCluster is nil, the inference server health will be checked for the control plane cluster.
	IsHealthy(ctx context.Context, inferenceServerName, namespace string, targetCluster *v2pb.ClusterTarget) (bool, error)
	// CheckModelStatus checks the status of a model on an inference server. If targetCluster is nil, the model status will be checked for the control plane cluster.
	CheckModelStatus(ctx context.Context, modelName string, inferenceServerName string, namespace string, targetCluster *v2pb.ClusterTarget) (bool, error)
}
