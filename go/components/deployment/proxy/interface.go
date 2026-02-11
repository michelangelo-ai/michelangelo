//go:generate mamockgen ProxyProvider

package proxy

import (
	"context"

	"go.uber.org/zap"
)

// ProxyProvider manages HTTP routing configuration for model deployments.
// Implementations handle Kubernetes Gateway API HTTPRoute resources or alternative routing mechanisms.
type ProxyProvider interface {
	// EnsureDeploymentRoute creates or updates a deployment-specific route with model targeting.
	EnsureDeploymentRoute(ctx context.Context, logger *zap.Logger, deploymentName string, namespace string, inferenceServerName string, modelName, backendServiceName string) error
	// CheckDeploymentRouteStatus validates that a deployment route is correctly configured.
	CheckDeploymentRouteStatus(ctx context.Context, logger *zap.Logger, deploymentName string, namespace string, inferenceServerName string, modelName, backendServiceName string) (bool, error)
	// DeploymentRouteExists checks if a deployment-specific route has been created.
	DeploymentRouteExists(ctx context.Context, logger *zap.Logger, deploymentName string, namespace string) (bool, error)
	// DeleteDeploymentRoute removes a deployment-specific route.
	DeleteDeploymentRoute(ctx context.Context, logger *zap.Logger, deploymentName string, namespace string) error
}
