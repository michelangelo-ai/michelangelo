//go:generate mamockgen EndpointRegistry

package endpointregistry

import (
	"context"

	"go.uber.org/zap"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ClusterEndpoint represents the desired and/or observed endpoint registration for a single cluster.
//
// In the "annotations-based discovery" approach, the EndpointRegistry implementation will use
// TargetCluster to connect to the target cluster and discover the concrete address/ports to register
// (e.g. east-west gateway LB address), based on annotations set by the backend on the created Service(s).
type ClusterEndpoint struct {
	ClusterID           string
	InferenceServerName string
	Namespace           string

	// TargetCluster is required input for EnsureRegisteredEndpoint. It may be nil on outputs from ListRegisteredEndpoints.
	TargetCluster *v2pb.ClusterTarget

	// Resolved fields (filled by registry implementation and/or read back from control-plane resources).
	Address string
	Ports   map[string]uint32
}

// EndpointRegistry provides an abstraction for managing inference server endpoints
// across multiple clusters. It handles the registration of cluster endpoints in the
// control plane, enabling service mesh routing and endpoint discovery.
//
// Implementations may use Istio ServiceEntry, ExternalName Services, or other
// mechanisms to register and discover endpoints.
type EndpointRegistry interface {
	// EnsureRegisteredEndpoint upserts the control-plane endpoint registration for a single target cluster.
	EnsureRegisteredEndpoint(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error

	// DeleteRegisteredEndpoint removes the control-plane registration for a single target cluster endpoint.
	DeleteRegisteredEndpoint(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace, clusterID string) error

	// ListRegisteredEndpoints retrieves all registered cluster endpoints for an inference server.
	// This is useful for pruning endpoints when clusterTargets change.
	ListRegisteredEndpoints(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace string) ([]ClusterEndpoint, error)
}
