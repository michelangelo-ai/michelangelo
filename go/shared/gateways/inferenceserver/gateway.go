package inferenceserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Gateway provides a unified interface for inference server operations across different providers
type Gateway interface {
	// Infrastructure Management
	CreateInfrastructure(ctx context.Context, logger logr.Logger, request InfrastructureRequest) (*InfrastructureResponse, error)
	GetInfrastructureStatus(ctx context.Context, logger logr.Logger, request InfrastructureStatusRequest) (*InfrastructureStatus, error)
	DeleteInfrastructure(ctx context.Context, logger logr.Logger, request InfrastructureDeleteRequest) error

	// Proxy/Routing Management
	ConfigureProxy(ctx context.Context, logger logr.Logger, request ProxyConfigRequest) error
	GetProxyStatus(ctx context.Context, logger logr.Logger, request ProxyStatusRequest) (*ProxyStatus, error)

	// Model Management
	LoadModel(ctx context.Context, logger logr.Logger, request ModelLoadRequest) error
	CheckModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (bool, error)
	GetModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (*ModelStatus, error)

	// Health Checking
	IsHealthy(ctx context.Context, logger logr.Logger, serverName string, backendType v2pb.BackendType) (bool, error)
}

// ModelLoadRequest contains information needed to load a model
type ModelLoadRequest struct {
	ModelName        string
	ModelVersion     string
	PackagePath      string
	InferenceServer  string
	BackendType      v2pb.BackendType
	Config           map[string]string
}

// ModelStatusRequest contains information needed to check model status
type ModelStatusRequest struct {
	ModelName       string
	ModelVersion    string
	InferenceServer string
	BackendType     v2pb.BackendType
}

// ModelStatus represents the status of a model
type ModelStatus struct {
	State   string // LOADING, LOADED, FAILED, NOT_FOUND
	Message string
	Ready   bool
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

// gateway implements the Gateway interface
type gateway struct {
	httpClient    *http.Client
	kubeClient    client.Client
	dynamicClient dynamic.Interface
}

// NewGateway creates a new inference server gateway
func NewGateway() Gateway {
	return &gateway{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewGatewayWithClients creates a new inference server gateway with Kubernetes clients
func NewGatewayWithClients(kubeClient client.Client, dynamicClient dynamic.Interface) Gateway {
	return &gateway{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		kubeClient:    kubeClient,
		dynamicClient: dynamicClient,
	}
}

// LoadModel dispatches model loading based on backend type
func (g *gateway) LoadModel(ctx context.Context, logger logr.Logger, request ModelLoadRequest) error {
	logger.Info("Loading model", "model", request.ModelName, "backend", request.BackendType)

	switch request.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return g.loadTritonModel(ctx, logger, request)
	case v2pb.BACKEND_TYPE_LLM_D:
		return g.loadLLMDModel(ctx, logger, request)
	case v2pb.BACKEND_TYPE_DYNAMO:
		return g.loadDynamoModel(ctx, logger, request)
	default:
		return fmt.Errorf("unsupported backend type: %v", request.BackendType)
	}
}

// CheckModelStatus dispatches model status checking based on backend type
func (g *gateway) CheckModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (bool, error) {
	logger.Info("Checking model status", "model", request.ModelName, "backend", request.BackendType)

	switch request.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return g.checkTritonModelStatus(ctx, logger, request)
	case v2pb.BACKEND_TYPE_LLM_D:
		return g.checkLLMDModelStatus(ctx, logger, request)
	case v2pb.BACKEND_TYPE_DYNAMO:
		return g.checkDynamoModelStatus(ctx, logger, request)
	default:
		return false, fmt.Errorf("unsupported backend type: %v", request.BackendType)
	}
}

// GetModelStatus dispatches detailed model status retrieval based on backend type
func (g *gateway) GetModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (*ModelStatus, error) {
	logger.Info("Getting model status", "model", request.ModelName, "backend", request.BackendType)

	switch request.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return g.getTritonModelStatus(ctx, logger, request)
	case v2pb.BACKEND_TYPE_LLM_D:
		return g.getLLMDModelStatus(ctx, logger, request)
	case v2pb.BACKEND_TYPE_DYNAMO:
		return g.getDynamoModelStatus(ctx, logger, request)
	default:
		return nil, fmt.Errorf("unsupported backend type: %v", request.BackendType)
	}
}

// IsHealthy dispatches health checking based on backend type
func (g *gateway) IsHealthy(ctx context.Context, logger logr.Logger, serverName string, backendType v2pb.BackendType) (bool, error) {
	logger.Info("Checking server health", "server", serverName, "backend", backendType)

	switch backendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return g.isTritonHealthy(ctx, logger, serverName)
	case v2pb.BACKEND_TYPE_LLM_D:
		return g.isLLMDHealthy(ctx, logger, serverName)
	case v2pb.BACKEND_TYPE_DYNAMO:
		return g.isDynamoHealthy(ctx, logger, serverName)
	default:
		return false, fmt.Errorf("unsupported backend type: %v", backendType)
	}
}

// Infrastructure Management Methods

// CreateInfrastructure dispatches infrastructure creation based on backend type
func (g *gateway) CreateInfrastructure(ctx context.Context, logger logr.Logger, request InfrastructureRequest) (*InfrastructureResponse, error) {
	logger.Info("Creating infrastructure", "server", request.InferenceServer.Name, "backend", request.BackendType)

	switch request.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return g.createTritonInfrastructure(ctx, logger, request)
	case v2pb.BACKEND_TYPE_LLM_D:
		return g.createLLMDInfrastructure(ctx, logger, request)
	case v2pb.BACKEND_TYPE_DYNAMO:
		return g.createDynamoInfrastructure(ctx, logger, request)
	default:
		return nil, fmt.Errorf("unsupported backend type: %v", request.BackendType)
	}
}

// GetInfrastructureStatus dispatches infrastructure status checking based on backend type
func (g *gateway) GetInfrastructureStatus(ctx context.Context, logger logr.Logger, request InfrastructureStatusRequest) (*InfrastructureStatus, error) {
	logger.Info("Getting infrastructure status", "server", request.InferenceServer, "backend", request.BackendType)

	switch request.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return g.getTritonInfrastructureStatus(ctx, logger, request)
	case v2pb.BACKEND_TYPE_LLM_D:
		return g.getLLMDInfrastructureStatus(ctx, logger, request)
	case v2pb.BACKEND_TYPE_DYNAMO:
		return g.getDynamoInfrastructureStatus(ctx, logger, request)
	default:
		return nil, fmt.Errorf("unsupported backend type: %v", request.BackendType)
	}
}

// DeleteInfrastructure dispatches infrastructure deletion based on backend type
func (g *gateway) DeleteInfrastructure(ctx context.Context, logger logr.Logger, request InfrastructureDeleteRequest) error {
	logger.Info("Deleting infrastructure", "server", request.InferenceServer, "backend", request.BackendType)

	switch request.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return g.deleteTritonInfrastructure(ctx, logger, request)
	case v2pb.BACKEND_TYPE_LLM_D:
		return g.deleteLLMDInfrastructure(ctx, logger, request)
	case v2pb.BACKEND_TYPE_DYNAMO:
		return g.deleteDynamoInfrastructure(ctx, logger, request)
	default:
		return fmt.Errorf("unsupported backend type: %v", request.BackendType)
	}
}

// Proxy Management Methods

// ConfigureProxy sets up Istio VirtualService routing
func (g *gateway) ConfigureProxy(ctx context.Context, logger logr.Logger, request ProxyConfigRequest) error {
	logger.Info("Configuring proxy", "server", request.InferenceServer, "model", request.ModelName)
	
	// Use the same Istio logic from the deployment provider
	return g.configureIstioProxy(ctx, logger, request)
}

// GetProxyStatus checks the status of Istio VirtualService configuration
func (g *gateway) GetProxyStatus(ctx context.Context, logger logr.Logger, request ProxyStatusRequest) (*ProxyStatus, error) {
	logger.Info("Getting proxy status", "server", request.InferenceServer)
	
	return g.getIstioProxyStatus(ctx, logger, request)
}