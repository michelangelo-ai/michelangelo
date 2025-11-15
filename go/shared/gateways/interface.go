package gateways

import (
	"context"

	"go.uber.org/zap"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// CreateInfrastructureRequest contains information needed to create infrastructure
type CreateInfrastructureRequest struct {
	InferenceServer *v2pb.InferenceServer
	BackendType     v2pb.BackendType
	Namespace       string
	Resources       ResourceSpec
}

// CreateInfrastructureResponse contains information about the created infrastructure
type CreateInfrastructureResponse struct {
	State     v2pb.InferenceServerState
	Message   string
	Endpoints []string
	Details   map[string]interface{}
}

type ResourceSpec struct {
	CPU         string
	Memory      string
	GPU         int32
	Replicas    int32
	ImageTag    string
	ModelConfig map[string]string
}

// DeleteInfrastructureRequest contains information needed to delete infrastructure
type DeleteInfrastructureRequest struct {
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
}

// LoadModelRequest contains information needed to load a model
type LoadModelRequest struct {
	ModelName       string
	ModelVersion    string
	PackagePath     string
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
	Config          map[string]string
}

// UnloadModelRequest contains information needed to unload a model
type UnloadModelRequest struct {
	ModelName       string
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
}

// CheckModelStatusRequest contains information needed to check model status
type CheckModelStatusRequest struct {
	ModelName       string
	ModelVersion    string
	Namespace       string
	DeploymentName  string
	InferenceServer string
	BackendType     v2pb.BackendType
}

type CheckModelStatusResponse struct {
	Status ModelStatus
}

// ModelStatus represents the status of a model
type ModelStatus struct {
	State   v2pb.InferenceServerState
	Message string
	Ready   bool
}

// GetInfrastructureStatusRequest contains information needed to get infrastructure status
type GetInfrastructureStatusRequest struct {
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
}

// GetInfrastructureStatusResponse contains information about the infrastructure status
type GetInfrastructureStatusResponse struct {
	Status InfrastructureStatus
}

// InfrastructureStatus represents the status of the infrastructure
type InfrastructureStatus struct {
	State     v2pb.InferenceServerState
	Message   string
	Ready     bool
	Endpoints []string
}

// Proxy Management Types
type ConfigureProxyRequest struct {
	InferenceServer string
	Namespace       string
	ModelName       string
	DeploymentName  string
	BackendType     v2pb.BackendType
}

// AddDeploymentRouteRequest contains information needed to add a deployment-specific route
type AddDeploymentRouteRequest struct {
	ModelName       string
	InferenceServer string
	Namespace       string
	DeploymentName  string
	BackendType     v2pb.BackendType
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

// Gateway provides a unified interface for inference server operations across different providers
type Gateway interface {
	// Inference Server Backend Management
	RegisterBackend(backendType v2pb.BackendType, backend Backend)

	// Infrastructure Management
	CreateInfrastructure(ctx context.Context, logger *zap.Logger, request CreateInfrastructureRequest) (*CreateInfrastructureResponse, error)
	GetInfrastructureStatus(ctx context.Context, logger *zap.Logger, request GetInfrastructureStatusRequest) (*GetInfrastructureStatusResponse, error)
	DeleteInfrastructure(ctx context.Context, logger *zap.Logger, request DeleteInfrastructureRequest) error

	// Model Management
	LoadModel(ctx context.Context, logger *zap.Logger, request LoadModelRequest) error
	UnloadModel(ctx context.Context, logger *zap.Logger, request UnloadModelRequest) error
	CheckModelStatus(ctx context.Context, logger *zap.Logger, request CheckModelStatusRequest) (bool, error)

	// Health Check
	IsHealthy(ctx context.Context, logger *zap.Logger, serverName string, backendType v2pb.BackendType) (bool, error)

	// Proxy/Routing Management
	ConfigureProxy(ctx context.Context, logger *zap.Logger, request ConfigureProxyRequest) error
	GetProxyStatus(ctx context.Context, logger *zap.Logger, request GetProxyStatusRequest) (*GetProxyStatusResponse, error)
	AddDeploymentRoute(ctx context.Context, logger *zap.Logger, request AddDeploymentRouteRequest) error
	DeleteHTTPRoute(ctx context.Context, logger *zap.Logger, httpRouteName, namespace string) error
}

type Backend interface {
	// Infrastructure Management
	CreateInfrastructure(ctx context.Context, logger *zap.Logger, request CreateInfrastructureRequest) (*CreateInfrastructureResponse, error)
	GetInfrastructureStatus(ctx context.Context, logger *zap.Logger, request GetInfrastructureStatusRequest) (*GetInfrastructureStatusResponse, error)
	DeleteInfrastructure(ctx context.Context, logger *zap.Logger, request DeleteInfrastructureRequest) error

	// Health Check
	IsHealthy(ctx context.Context, logger *zap.Logger, serverName string) (bool, error)

	// Model Management
	LoadModel(ctx context.Context, logger *zap.Logger, request LoadModelRequest) error
	UnloadModel(ctx context.Context, logger *zap.Logger, request UnloadModelRequest) error
	CheckModelStatus(ctx context.Context, logger *zap.Logger, request CheckModelStatusRequest) (bool, error)
}
