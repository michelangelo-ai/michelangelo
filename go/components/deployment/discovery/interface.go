//go:generate mamockgen ModelDiscoveryProvider

package discovery

import (
	"context"
)

// ModelDiscoveryProvider manages the control-plane route that exposes a deployment's model
// across every cluster hosting the inference server, so a single user-facing URL
// load-balances inference traffic across the multi-cluster fleet.
//
// All operations target the control-plane cluster.
//
// The route forwards to a Service named "{inferenceServerName}-endpoints" in the same
// namespace as the deployment. Callers must ensure that Service is being kept in sync
// with the inference server's hosting clusters; the route returns no traffic until it is.
//
// Implementations are idempotent. Repeated EnsureDiscoveryRoute calls with the same
// arguments converge to the same state. DeleteDiscoveryRoute tolerates not-found.
type ModelDiscoveryProvider interface {
	// EnsureDiscoveryRoute creates or updates the discovery route for the given deployment.
	EnsureDiscoveryRoute(ctx context.Context, deploymentName string, namespace string, inferenceServerName string, modelName string) error
	// CheckDiscoveryRouteStatus reports whether the discovery route is configured for the given deployment and model.
	CheckDiscoveryRouteStatus(ctx context.Context, deploymentName string, namespace string, inferenceServerName string, modelName string) (bool, error)
	// DeleteDiscoveryRoute removes the discovery route. Tolerates not-found.
	DeleteDiscoveryRoute(ctx context.Context, deploymentName string, namespace string) error
}
