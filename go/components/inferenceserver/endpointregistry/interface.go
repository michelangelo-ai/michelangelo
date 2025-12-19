//go:generate mamockgen EndpointRegistry

package endpointregistry

import (
	"context"

	"go.uber.org/zap"
)

// ClusterEndpoint represents a registered cluster endpoint for an inference server.
// It contains all the information needed to connect to and route traffic to the cluster.
type ClusterEndpoint struct {
	// ClusterID is the unique identifier for the cluster.
	ClusterID string
	// InferenceServerName is the name of the inference server this endpoint belongs to.
	InferenceServerName string
	// Namespace is the Kubernetes namespace.
	Namespace string
	// Host is the Kubernetes API server host for the cluster.
	Host string
	// Port is the Kubernetes API server port for the cluster.
	Port string
	// ServiceHost is the internal service hostname for the inference server in the cluster.
	ServiceHost string
	// ServicePort is the port the inference service listens on.
	ServicePort uint32
	// TokenSecretRef is the reference to the secret containing the cluster auth token.
	TokenSecretRef string
	// CASecretRef is the reference to the secret containing the cluster CA certificate.
	CASecretRef string
}

// EndpointRegistry provides an abstraction for managing inference server endpoints
// across multiple clusters. It handles the registration of cluster endpoints in the
// control plane, enabling service mesh routing and endpoint discovery.
//
// Implementations may use Istio ServiceEntry, ExternalName Services, or other
// mechanisms to register and discover endpoints.
type EndpointRegistry interface {
	// RegisterEndpoint registers a cluster as an endpoint for an inference server.
	// This creates the necessary service mesh resources (e.g., ServiceEntry) in the control plane.
	RegisterEndpoint(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error

	// UnregisterEndpoint removes a cluster endpoint registration for an inference server.
	UnregisterEndpoint(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace, clusterID string) error

	// GetEndpoints retrieves all registered cluster endpoints for an inference server.
	GetEndpoints(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace string) ([]ClusterEndpoint, error)

	// GetEndpoint retrieves a specific cluster endpoint for an inference server.
	GetEndpoint(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace, clusterID string) (*ClusterEndpoint, error)

	// UpdateEndpoint updates an existing cluster endpoint registration.
	UpdateEndpoint(ctx context.Context, logger *zap.Logger, endpoint ClusterEndpoint) error
}
