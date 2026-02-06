//go:generate mamockgen EndpointRegistry

package endpointregistry

import (
	"context"

	"go.uber.org/zap"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// ClusterEndpoint represents an endpoint registration for a single cluster.
type ClusterEndpoint struct {
	ClusterID           string
	InferenceServerName string
	Namespace           string

	Address string
	Ports   map[string]uint32
}

// EndpointRegistry provides an abstraction for managing inference server endpoints across multiple clusters.
// It handles the registration of cluster endpoints in the control plane, enabling service mesh routing and endpoint discovery.
// Implementations may use Istio ServiceEntry, ExternalName Services, or other mechanisms to register and discover endpoints.
type EndpointRegistry interface {
	// EnsureRegisteredEndpoint upserts the control-plane endpoint registration for a single target cluster.
	EnsureRegisteredEndpoint(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint, targetCluster *v2pb.ClusterTarget) error
	// DeleteRegisteredEndpoint removes the control-plane registration for a single target cluster endpoint.
	DeleteRegisteredEndpoint(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace, clusterID string) error
	// ListRegisteredEndpoints retrieves all registered cluster endpoints for an inference server.
	ListRegisteredEndpoints(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace string) ([]ClusterEndpoint, error)
	// GetControlPlaneServiceName returns the name of the control plane service for an inference server.
	GetControlPlaneServiceName(inferenceServerName string) string
}
