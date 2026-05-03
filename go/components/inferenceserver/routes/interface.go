//go:generate mamockgen RouteProvider

// Package routes manages the InferenceServer-owned routing resources that
// direct inference traffic. Each InferenceServer is exposed by two routes: a
// discovery route in the control plane that fans inbound requests across the
// hosting clusters, and a traffic route in each target cluster that forwards
// to the local inference Service.
package routes

import (
	"context"

	"k8s.io/client-go/dynamic"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// RouteProvider manages the InferenceServer-owned routes and the per-Deployment
// rules within them.
//
// Discovery methods operate on the control-plane route that selects a hosting
// cluster for an inbound request. Traffic methods operate on the per-cluster
// route that forwards a request to the local inference Service.
//
// Per-Deployment rule operations are idempotent on the deployment identity.
// Concurrent rule mutations on the same route are serialized.
type RouteProvider interface {
	// EnsureDiscoveryRoute creates the discovery route for the InferenceServer
	// if it does not exist, and ensures its default rule is present. Existing
	// per-Deployment rules are preserved.
	EnsureDiscoveryRoute(ctx context.Context, dynamicClient dynamic.Interface, server *v2pb.InferenceServer) error

	// DeleteDiscoveryRoute removes the discovery route. Tolerates not-found.
	DeleteDiscoveryRoute(ctx context.Context, dynamicClient dynamic.Interface, inferenceServerName string, namespace string) error

	// DiscoveryRouteExists reports whether the discovery route has been
	// provisioned for the InferenceServer.
	DiscoveryRouteExists(ctx context.Context, dynamicClient dynamic.Interface, inferenceServerName string, namespace string) (bool, error)

	// EnsureTrafficRoute creates the traffic route in the target cluster if it
	// does not exist, and ensures its default rule is present. Existing
	// per-Deployment rules are preserved.
	EnsureTrafficRoute(ctx context.Context, dynamicClient dynamic.Interface, clusterID string, inferenceServerName string, namespace string) error

	// DeleteTrafficRoute removes the traffic route from the target cluster.
	// Tolerates not-found.
	DeleteTrafficRoute(ctx context.Context, dynamicClient dynamic.Interface, clusterID string, inferenceServerName string, namespace string) error

	// TrafficRouteExists reports whether the traffic route has been
	// provisioned for the InferenceServer in the target cluster.
	TrafficRouteExists(ctx context.Context, dynamicClient dynamic.Interface, clusterID string, inferenceServerName string, namespace string) (bool, error)

	// UpsertDiscoveryRule adds or updates the rule on the discovery route that
	// exposes {deploymentName}'s {modelName}.
	UpsertDiscoveryRule(ctx context.Context, dynamicClient dynamic.Interface, inferenceServerName string, namespace string, deploymentName string, modelName string) error

	// RemoveDiscoveryRule removes the rule for {deploymentName} from the
	// discovery route. Tolerates a missing rule.
	RemoveDiscoveryRule(ctx context.Context, dynamicClient dynamic.Interface, inferenceServerName string, namespace string, deploymentName string) error

	// DeploymentDiscoveryRouteExists reports whether the discovery routing for
	// {deploymentName} is configured.
	DeploymentDiscoveryRouteExists(ctx context.Context, dynamicClient dynamic.Interface, inferenceServerName string, namespace string, deploymentName string) (bool, error)

	// UpsertTrafficRule adds or updates the rule on the traffic route in the
	// target cluster that forwards {deploymentName}'s requests to {modelName}
	// on the local inference Service.
	UpsertTrafficRule(ctx context.Context, dynamicClient dynamic.Interface, clusterID string, inferenceServerName string, namespace string, deploymentName string, modelName string) error

	// RemoveTrafficRule removes the rule for {deploymentName} from the traffic
	// route in the target cluster. Tolerates a missing rule.
	RemoveTrafficRule(ctx context.Context, dynamicClient dynamic.Interface, clusterID string, inferenceServerName string, namespace string, deploymentName string) error

	// DeploymentTrafficRouteExists reports whether the traffic routing for
	// {deploymentName} is present AND configured for {modelName} in the target
	// cluster. Returns false when the rule is missing or refers to a different
	// model, so the rollout actor reapplies on a desiredRevision change.
	DeploymentTrafficRouteExists(ctx context.Context, dynamicClient dynamic.Interface, clusterID string, inferenceServerName string, namespace string, deploymentName string, modelName string) (bool, error)
}
