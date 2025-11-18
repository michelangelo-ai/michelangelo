package gateways

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/shared/configmap"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// gateway implements the Gateway interface
type gateway struct {
	httpClient    *http.Client
	kubeClient    client.Client
	dynamicClient dynamic.Interface

	registry         *registry
	httpRouteManager RouteManager

	modelConfigMapProvider configmap.ModelConfigMapProvider
}

type Params struct {
	KubeClient             client.Client
	DynamicClient          dynamic.Interface
	HttpRouteManager       RouteManager
	ModelConfigMapProvider configmap.ModelConfigMapProvider
}

// NewGatewayWithClients creates a new inference server gateway with Kubernetes clients
func NewGatewayWithClients(p Params) Gateway {
	return &gateway{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		kubeClient:    p.KubeClient,
		dynamicClient: p.DynamicClient,

		registry: newRegistry(),

		httpRouteManager:       p.HttpRouteManager,
		modelConfigMapProvider: p.ModelConfigMapProvider,
	}
}

// Inference Server Backend Management Methods

// RegisterBackend registers a backend for a specific backend type
func (g *gateway) RegisterBackend(backendType v2pb.BackendType, backend Backend) {
	g.registry.registerBackend(backendType, backend)
}

// CreateInfrastructure dispatches infrastructure creation based on backend type
func (g *gateway) CreateInfrastructure(ctx context.Context, logger *zap.Logger, request CreateInfrastructureRequest) (*CreateInfrastructureResponse, error) {
	logger.Info("Creating infrastructure", zap.String("server", request.InferenceServer.Name), zap.String("backend", request.BackendType.String()))
	backend, err := g.registry.getBackend(request.BackendType)
	if err != nil {
		return nil, err
	}
	return backend.CreateInfrastructure(ctx, logger, request)
}

// GetInfrastructureStatus dispatches infrastructure status checking based on backend type
func (g *gateway) GetInfrastructureStatus(ctx context.Context, logger *zap.Logger, request GetInfrastructureStatusRequest) (*GetInfrastructureStatusResponse, error) {
	logger.Info("Getting infrastructure status", zap.String("server", request.InferenceServer), zap.String("backend", request.BackendType.String()))
	backend, err := g.registry.getBackend(request.BackendType)
	if err != nil {
		return nil, err
	}
	return backend.GetInfrastructureStatus(ctx, logger, request)
}

// DeleteInfrastructure dispatches infrastructure deletion based on backend type
func (g *gateway) DeleteInfrastructure(ctx context.Context, logger *zap.Logger, request DeleteInfrastructureRequest) error {
	logger.Info("Deleting infrastructure", zap.String("server", request.InferenceServer), zap.String("backend", request.BackendType.String()))
	backend, err := g.registry.getBackend(request.BackendType)
	if err != nil {
		return err
	}
	return backend.DeleteInfrastructure(ctx, logger, request)
}

// IsHealthy dispatches health checking based on backend type
func (g *gateway) IsHealthy(ctx context.Context, logger *zap.Logger, request HealthCheckRequest) (bool, error) {
	logger.Info("Checking server health", zap.String("server", request.InferenceServer), zap.String("backend", request.BackendType.String()))
	backend, err := g.registry.getBackend(request.BackendType)
	if err != nil {
		return false, err
	}
	return backend.IsHealthy(ctx, logger, request)
}

// Model Management Methods

// LoadModel dispatches model loading based on backend type
func (g *gateway) LoadModel(ctx context.Context, logger *zap.Logger, request LoadModelRequest) error {
	logger.Info("Loading model", zap.String("model", request.ModelName), zap.String("backend", request.BackendType.String()))
	backend, err := g.registry.getBackend(request.BackendType)
	if err != nil {
		return err
	}
	return backend.LoadModel(ctx, logger, request)
}

// UnloadModel dispatches model unloading based on backend type
func (g *gateway) UnloadModel(ctx context.Context, logger *zap.Logger, request UnloadModelRequest) error {
	logger.Info("Unloading model", zap.String("model", request.ModelName), zap.String("backend", request.BackendType.String()))
	backend, err := g.registry.getBackend(request.BackendType)
	if err != nil {
		return err
	}
	return backend.UnloadModel(ctx, logger, request)
}

// CheckModelStatus dispatches model status checking based on backend type
func (g *gateway) CheckModelStatus(ctx context.Context, logger *zap.Logger, request CheckModelStatusRequest) (bool, error) {
	logger.Info("Checking model status", zap.String("model", request.ModelName), zap.String("backend", request.BackendType.String()))
	backend, err := g.registry.getBackend(request.BackendType)
	if err != nil {
		return false, err
	}
	return backend.CheckModelStatus(ctx, logger, request)
}

// Proxy Management Methods

// ConfigureProxy sets up Istio VirtualService routing
func (g *gateway) ConfigureProxy(ctx context.Context, logger *zap.Logger, request ConfigureProxyRequest) error {
	logger.Info("Configuring proxy for inference server", zap.String("server", request.InferenceServer), zap.String("model", request.ModelName))
	return g.httpRouteManager.ConfigureProxy(ctx, logger, request)
}

// GetProxyStatus checks the status of Istio VirtualService configuration
func (g *gateway) GetProxyStatus(ctx context.Context, logger *zap.Logger, request GetProxyStatusRequest) (*GetProxyStatusResponse, error) {
	logger.Info("Getting proxy status for inference server", zap.String("server", request.InferenceServer))
	return g.httpRouteManager.GetProxyStatus(ctx, logger, request)
}

// AddDeploymentSpecificRoute adds a deployment-specific route to the HTTPRoute
func (g *gateway) AddDeploymentRoute(ctx context.Context, logger *zap.Logger, request AddDeploymentRouteRequest) error {
	logger.Info("Adding deployment-specific route for inference server", zap.String("server", request.InferenceServer), zap.String("deployment", request.DeploymentName), zap.String("model", request.ModelName))
	return g.httpRouteManager.AddDeploymentRoute(ctx, logger, request)
}

// DeleteRoute deletes a network route by name and namespace
func (g *gateway) DeleteRoute(ctx context.Context, logger *zap.Logger, request DeleteRouteRequest) error {
	logger.Info("Deleting network route for inference server", zap.String("server", request.InferenceServer), zap.String("namespace", request.Namespace))
	return g.httpRouteManager.DeleteRoute(ctx, logger, request)
}
