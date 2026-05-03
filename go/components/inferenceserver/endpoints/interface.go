// Package endpoints defines the abstractions the InferenceServer controller
// uses to publish per-cluster service-discovery information about an
// InferenceServer. EndpointSource resolves the network address at which one
// cluster admits traffic for an InferenceServer. EndpointPublisher maintains
// a per-server cluster ID to Endpoint map readable by other components.
package endpoints

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Endpoint is the network address (host, port, scheme) at which one cluster
// admits traffic for an InferenceServer.
type Endpoint struct {
	Host   string
	Port   int32
	Scheme string // "http" | "https"
}

// EndpointSource resolves the ingress endpoint for one ClusterTarget.
// Implementations abstract away the per-environment differences in how a
// cluster's ingress address is discovered. Implementations bind via fx;
// callers do not branch on environment.
type EndpointSource interface {
	Resolve(ctx context.Context, target *v2pb.ClusterTarget) (Endpoint, error)
}

// EndpointPublisher maintains the published cluster ID to Endpoint map for one
// InferenceServer. Implementations decide how the map is stored and how other
// components observe it.
//
// Each InferenceServer is exposed at a single discovery endpoint backed by a
// Service named "{inferenceServerName}-endpoints" in the InferenceServer's
// namespace. Downstream consumers reach the InferenceServer through this
// Service, so implementations must preserve both the name and the namespace.
//
// Sync is idempotent. Cluster IDs in the desired map are upserted, and cluster
// IDs previously published but absent from the desired map are removed. After
// Delete returns, the discovery Service for the server no longer exists.
type EndpointPublisher interface {
	// Sync reconciles the published map to match `endpoints`. Idempotent.
	// Cluster IDs in `endpoints` are upserted. Cluster IDs previously published
	// but absent from `endpoints` are removed.
	Sync(ctx context.Context, server *v2pb.InferenceServer, endpoints map[string]Endpoint) error

	// Get returns the currently published cluster ID to Endpoint map for the
	// server. The map is empty when nothing has been published yet.
	Get(ctx context.Context, server *v2pb.InferenceServer) (map[string]Endpoint, error)

	// Delete removes everything the publisher has created for the server.
	Delete(ctx context.Context, server *v2pb.InferenceServer) error
}
