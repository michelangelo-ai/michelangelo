package gateways

import (
	"context"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/shared/configmap"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ModelLoadRequest contains information needed to load a model
type ModelLoadRequest struct {
	ModelName       string
	ModelVersion    string
	PackagePath     string
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
	Config          map[string]string
}

// ModelUnloadRequest contains information needed to unload a model
type ModelUnloadRequest struct {
	ModelName       string
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
}

// ModelStatusRequest contains information needed to check model status
type ModelStatusRequest struct {
	ModelName       string
	ModelVersion    string
	InferenceServer string
	DeploymentName  string // Added for deployment-specific routing
	Namespace       string
	BackendType     v2pb.BackendType
}

// ModelStatus represents the status of a model
type ModelStatus struct {
	State   v2pb.InferenceServerState // Use proper enum type
	Message string
	Ready   bool
}

// ModelConfigUpdateRequest contains information for updating model configurations
type ModelConfigUpdateRequest struct {
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
	ModelConfigs    []configmap.ModelConfigEntry
}

// ModelConfigMapRequest contains information needed to create/update model ConfigMaps
type ModelConfigMapRequest struct {
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
	ModelConfigs    []configmap.ModelConfigEntry
	Labels          map[string]string
	Annotations     map[string]string
}

// Infrastructure Management Types
type InfrastructureRequest struct {
	InferenceServer *v2pb.InferenceServer
	BackendType     v2pb.BackendType
	Namespace       string
	Resources       ResourceSpec
}

type ResourceSpec struct {
	CPU         string
	Memory      string
	GPU         int32
	Replicas    int32
	ImageTag    string
	ModelConfig map[string]string
}

type InfrastructureResponse struct {
	State     v2pb.InferenceServerState
	Message   string
	Endpoints []string
	Details   map[string]interface{}
}

type InfrastructureStatusRequest struct {
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
}

type InfrastructureStatus struct {
	State     v2pb.InferenceServerState
	Message   string
	Ready     bool
	Endpoints []string
	Pods      []PodStatus
}

type PodStatus struct {
	Name  string
	Ready bool
	Phase string
}

type InfrastructureDeleteRequest struct {
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
}

// Proxy Management Types
type ProxyConfigRequest struct {
	InferenceServer string
	Namespace       string
	ModelName       string
	DeploymentName  string
	BackendType     v2pb.BackendType
	Routes          []RouteConfig
}

type RouteConfig struct {
	Path        string
	Destination string
	Rewrite     string
	Weight      int32
}

type ProxyStatusRequest struct {
	InferenceServer string
	Namespace       string
}

type ProxyStatus struct {
	Configured bool
	Routes     []ActiveRoute
	Message    string
}

type ActiveRoute struct {
	Path        string
	Destination string
	Rewrite     string
	Active      bool
}

// Health Check Types
type HealthCheckRequest struct {
	InferenceServer string
	Namespace       string
}

type HealthStatus struct {
	Healthy bool
	Message string
}

// Gateway provides a unified interface for inference server operations across different providers
type Gateway interface {
	// Inference Server Backend Management
	RegisterBackend(backendType v2pb.BackendType, backend Backend)

	// Infrastructure Management
	CreateInfrastructure(ctx context.Context, logger *zap.Logger, request InfrastructureRequest) (*InfrastructureResponse, error)
	GetInfrastructureStatus(ctx context.Context, logger *zap.Logger, request InfrastructureStatusRequest) (*InfrastructureStatus, error)
	DeleteInfrastructure(ctx context.Context, logger *zap.Logger, request InfrastructureDeleteRequest) error

	// Model Management
	LoadModel(ctx context.Context, logger *zap.Logger, request ModelLoadRequest) error
	UnloadModel(ctx context.Context, logger *zap.Logger, request ModelUnloadRequest) error
	CheckModelStatus(ctx context.Context, logger *zap.Logger, request ModelStatusRequest) (bool, error)

	// Health Check
	IsHealthy(ctx context.Context, logger *zap.Logger, serverName string, backendType v2pb.BackendType) (bool, error)

	// Proxy/Routing Management
	ConfigureProxy(ctx context.Context, logger *zap.Logger, request ProxyConfigRequest) error
	GetProxyStatus(ctx context.Context, logger *zap.Logger, request ProxyStatusRequest) (*ProxyStatus, error)
	AddDeploymentRoute(ctx context.Context, logger *zap.Logger, request ProxyConfigRequest) error
	DeleteHTTPRoute(ctx context.Context, logger *zap.Logger, httpRouteName, namespace string) error
}

type Backend interface {
	// Infrastructure Management
	CreateInfrastructure(ctx context.Context, logger *zap.Logger, request InfrastructureRequest) (*InfrastructureResponse, error)
	GetInfrastructureStatus(ctx context.Context, logger *zap.Logger, request InfrastructureStatusRequest) (*InfrastructureStatus, error)
	DeleteInfrastructure(ctx context.Context, logger *zap.Logger, request InfrastructureDeleteRequest) error

	// Health Check
	IsHealthy(ctx context.Context, logger *zap.Logger, serverName string) (bool, error)

	// Model Management
	LoadModel(ctx context.Context, logger *zap.Logger, request ModelLoadRequest) error
	UnloadModel(ctx context.Context, logger *zap.Logger, request ModelUnloadRequest) error
	CheckModelStatus(ctx context.Context, logger *zap.Logger, request ModelStatusRequest) (bool, error)
}
