//go:generate mamockgen ProxyProvider

package proxy

import (
	"context"

	"go.uber.org/zap"
)

// EnsureInferenceServerRouteRequest specifies parameters for creating baseline inference server routes.
type EnsureInferenceServerRouteRequest struct {
	InferenceServer string
	Namespace       string
	ModelName       string
}

// EnsureDeploymentRouteRequest specifies parameters for creating deployment-specific routes.
type EnsureDeploymentRouteRequest struct {
	DeploymentName  string
	Namespace       string
	ModelName       string
	InferenceServer string
}

// CheckDeploymentRouteStatusRequest specifies parameters for validating deployment route configuration.
type CheckDeploymentRouteStatusRequest struct {
	DeploymentName  string
	Namespace       string
	InferenceServer string
	ModelName       string
}

// GetProxyStatusRequest specifies parameters for querying proxy routing configuration.
type GetProxyStatusRequest struct {
	InferenceServer string
	Namespace       string
}

// GetProxyStatusResponse provides the proxy routing configuration and status.
type GetProxyStatusResponse struct {
	Status ProxyStatus
}

// ProxyStatus represents the current routing configuration and active routes.
type ProxyStatus struct {
	Configured bool
	Routes     []ActiveRoute
	Message    string
}

// ActiveRoute represents a configured HTTP route with path matching and rewriting rules.
type ActiveRoute struct {
	Path        string
	Destination string
	Rewrite     string
	Active      bool
}

// DeleteInferenceServerRouteRequest specifies parameters for removing inference server routes.
type DeleteInferenceServerRouteRequest struct {
	InferenceServer string
	Namespace       string
}

// DeleteDeploymentRouteRequest specifies parameters for removing deployment-specific routes.
type DeleteDeploymentRouteRequest struct {
	DeploymentName string
	Namespace      string
}

// DeploymentRouteExistsRequest specifies parameters for checking deployment route existence.
type DeploymentRouteExistsRequest struct {
	DeploymentName string
	Namespace      string
}

// ProxyProvider manages HTTP routing configuration for inference servers and deployments.
// Implementations handle Gateway API HTTPRoute resources or alternative routing mechanisms.
type ProxyProvider interface {
	// EnsureInferenceServerRoute creates or updates the baseline route for an inference server.
	EnsureInferenceServerRoute(ctx context.Context, logger *zap.Logger, request EnsureInferenceServerRouteRequest) error

	// EnsureDeploymentRoute creates or updates a deployment-specific route with model targeting.
	EnsureDeploymentRoute(ctx context.Context, logger *zap.Logger, request EnsureDeploymentRouteRequest) error

	// GetProxyStatus retrieves the current routing configuration and active routes.
	GetProxyStatus(ctx context.Context, logger *zap.Logger, request GetProxyStatusRequest) (*GetProxyStatusResponse, error)

	// CheckDeploymentRouteStatus validates that a deployment route is correctly configured.
	CheckDeploymentRouteStatus(ctx context.Context, logger *zap.Logger, request CheckDeploymentRouteStatusRequest) (bool, error)

	// DeploymentRouteExists checks if a deployment-specific route has been created.
	DeploymentRouteExists(ctx context.Context, logger *zap.Logger, request DeploymentRouteExistsRequest) (bool, error)

	// DeleteDeploymentRoute removes a deployment-specific route.
	DeleteDeploymentRoute(ctx context.Context, logger *zap.Logger, request DeleteDeploymentRouteRequest) error

	// DeleteInferenceServerRoute removes the baseline route for an inference server.
	DeleteInferenceServerRoute(ctx context.Context, logger *zap.Logger, request DeleteInferenceServerRouteRequest) error
}
