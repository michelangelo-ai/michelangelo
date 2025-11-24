package proxy

import (
	"context"

	"go.uber.org/zap"
)

// Proxy Management Types
type ConfigureProxyRequest struct {
	InferenceServer string
	Namespace       string
	ModelName       string
	DeploymentName  string
}

// AddDeploymentRouteRequest contains information needed to add a deployment-specific route
type AddDeploymentRouteRequest struct {
	ModelName       string
	InferenceServer string
	Namespace       string
	DeploymentName  string
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

// DeleteRouteRequest contains information needed to delete a network route
type DeleteRouteRequest struct {
	InferenceServer string
	Namespace       string
}

// ProxyProvider interface defines the methods for managing network routes and proxies.
type ProxyProvider interface {
	ConfigureProxy(ctx context.Context, logger *zap.Logger, request ConfigureProxyRequest) error
	GetProxyStatus(ctx context.Context, logger *zap.Logger, request GetProxyStatusRequest) (*GetProxyStatusResponse, error)
	AddDeploymentRoute(ctx context.Context, logger *zap.Logger, request AddDeploymentRouteRequest) error
	DeleteRoute(ctx context.Context, logger *zap.Logger, request DeleteRouteRequest) error
}
