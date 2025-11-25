//go:generate mamockgen ProxyProvider

package proxy

import (
	"context"

	"go.uber.org/zap"
)

// Proxy Management Types
type EnsureInferenceServerRouteRequest struct {
	InferenceServer string
	Namespace       string
	ModelName       string
}

// EnsureDeploymentRouteRequest contains information needed to ensure a deployment-specific route is present
type EnsureDeploymentRouteRequest struct {
	DeploymentName  string
	Namespace       string
	ModelName       string
	InferenceServer string
}

// CheckDeploymentRouteStatusRequest contains information needed to check the status of a deployment-specific route
type CheckDeploymentRouteStatusRequest struct {
	DeploymentName  string
	Namespace       string
	InferenceServer string
	ModelName       string
}

// GetProxyStatusRequest contains information needed to get the proxy status
type GetProxyStatusRequest struct {
	InferenceServer string
	Namespace       string
}

// GetProxyStatusResponse contains information about the proxy status
type GetProxyStatusResponse struct {
	Status ProxyStatus
}

// ProxyStatus represents the status of the proxy
type ProxyStatus struct {
	Configured bool
	Routes     []ActiveRoute
	Message    string
}

// ActiveRoute represents an active route
type ActiveRoute struct {
	Path        string
	Destination string
	Rewrite     string
	Active      bool
}

// DeleteInferenceServerRouteRequest contains information needed to delete a inference server-specific route
type DeleteInferenceServerRouteRequest struct {
	InferenceServer string
	Namespace       string
}

// DeleteDeploymentRouteRequest contains information needed to delete a deployment-specific route
type DeleteDeploymentRouteRequest struct {
	DeploymentName string
	Namespace      string
}

// DeploymentRouteExistsRequest contains information needed to check if a deployment-specific route exists
type DeploymentRouteExistsRequest struct {
	DeploymentName string
	Namespace      string
}

// ProxyProvider interface defines the methods for managing network routes and proxies.
type ProxyProvider interface {
	EnsureInferenceServerRoute(ctx context.Context, logger *zap.Logger, request EnsureInferenceServerRouteRequest) error
	EnsureDeploymentRoute(ctx context.Context, logger *zap.Logger, request EnsureDeploymentRouteRequest) error
	GetProxyStatus(ctx context.Context, logger *zap.Logger, request GetProxyStatusRequest) (*GetProxyStatusResponse, error)
	CheckDeploymentRouteStatus(ctx context.Context, logger *zap.Logger, request CheckDeploymentRouteStatusRequest) (bool, error)
	DeploymentRouteExists(ctx context.Context, logger *zap.Logger, request DeploymentRouteExistsRequest) (bool, error)
	DeleteDeploymentRoute(ctx context.Context, logger *zap.Logger, request DeleteDeploymentRouteRequest) error
	DeleteInferenceServerRoute(ctx context.Context, logger *zap.Logger, request DeleteInferenceServerRouteRequest) error
}
