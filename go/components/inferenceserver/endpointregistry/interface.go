//go:generate mamockgen EndpointRegistry

package endpointregistry

import (
	"context"

	"go.uber.org/zap"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// todo: ghosharitra: review comments here
// ClusterEndpoint represents the desired and/or observed endpoint registration for a single cluster.
//
// In the "annotations-based discovery" approach, the EndpointRegistry implementation will use
// TargetCluster to connect to the target cluster and discover the concrete address/ports to register
// (e.g. east-west gateway LB address), based on annotations set by the backend on the created Service(s).
type ClusterEndpoint struct {
	ClusterID           string
	InferenceServerName string
	Namespace           string

	// Resolved fields (filled by registry implementation and/or read back from control-plane resources).
	Address string
	Ports   map[string]uint32
}

// todo: ghosharitra: refine these comments according to the following block:
// The whole point of this interface is to provide means to "register" external endpoints (from differnet clusters). "Registering" could mean hooking up endpoints to a service mesh or a cloud's load balancer.
// On top of that, we also want to ensure that this interface creates some service within the control plane which is essentially a discovery mechanism for the external endpoints.

// EndpointRegistry provides an abstraction for managing inference server endpoints
// across multiple clusters. It handles the registration of cluster endpoints in the
// control plane, enabling service mesh routing and endpoint discovery.
//
// Implementations may use Istio ServiceEntry, ExternalName Services, or other
// mechanisms to register and discover endpoints.
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
