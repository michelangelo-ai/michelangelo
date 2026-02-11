//go:generate mamockgen RouteProvider

package route

import (
	"context"

	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
)

type RouteProvider interface {
	// EnsureDeploymentRoute creates or updates a deployment-specific route with model targeting.
	EnsureDeploymentRoute(ctx context.Context, logger *zap.Logger, client dynamic.Interface, deploymentName string, namespace string, inferenceServerName string, modelName string) error
	// CheckDeploymentRouteStatus validates that a deployment route is correctly configured.
	CheckDeploymentRouteStatus(ctx context.Context, logger *zap.Logger, client dynamic.Interface, deploymentName string, namespace string, inferenceServerName string, modelName string) (bool, error)
	// DeploymentRouteExists checks if a deployment-specific route has been created.
	DeploymentRouteExists(ctx context.Context, logger *zap.Logger, client dynamic.Interface, deploymentName string, namespace string) (bool, error)
	// DeleteDeploymentRoute removes a deployment-specific route.
	DeleteDeploymentRoute(ctx context.Context, logger *zap.Logger, client dynamic.Interface, deploymentName string, namespace string) error
}
