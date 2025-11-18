package gateways

import (
	"context"
	"fmt"
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
	httpRouteManager *httpRouteManager

	modelConfigMapProvider configmap.ModelConfigMapProvider
}

// NewGatewayWithClients creates a new inference server gateway with Kubernetes clients
func NewGatewayWithClients(kubeClient client.Client, dynamicClient dynamic.Interface, logger *zap.Logger) Gateway {
	return &gateway{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		kubeClient:    kubeClient,
		dynamicClient: dynamicClient,

		registry:         newRegistry(),
		httpRouteManager: newHTTPRouteManager(dynamicClient, logger),

		modelConfigMapProvider: configmap.NewDefaultModelConfigMapProvider(kubeClient, logger),
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
func (g *gateway) IsHealthy(ctx context.Context, logger *zap.Logger, serverName string, backendType v2pb.BackendType) (bool, error) {
	logger.Info("Checking server health", zap.String("server", serverName), zap.String("backend", backendType.String()))
	backend, err := g.registry.getBackend(backendType)
	if err != nil {
		return false, err
	}
	return backend.IsHealthy(ctx, logger, serverName)
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
	logger.Info("Configuring proxy with Gateway API HTTPRoute", zap.String("server", request.InferenceServer), zap.String("model", request.ModelName))
	httpRoute, err := g.httpRouteManager.getOrCreateHTTPRoute(ctx, logger, request)
	if err != nil {
		return fmt.Errorf("failed to get or create HTTPRoute: %w", err)
	}
	return g.httpRouteManager.updateProductionRoute(ctx, logger, httpRoute, request)
}

// GetProxyStatus checks the status of Istio VirtualService configuration
func (g *gateway) GetProxyStatus(ctx context.Context, logger *zap.Logger, request GetProxyStatusRequest) (*GetProxyStatusResponse, error) {
	logger.Info("Getting proxy status", zap.String("server", request.InferenceServer))
	return g.httpRouteManager.getProxyStatus(ctx, logger, request)
}

// AddDeploymentSpecificRoute adds a deployment-specific route to the HTTPRoute
func (g *gateway) AddDeploymentRoute(ctx context.Context, logger *zap.Logger, request AddDeploymentRouteRequest) error {
	logger.Info("Adding deployment-specific route", zap.String("server", request.InferenceServer), zap.String("deployment", request.DeploymentName), zap.String("model", request.ModelName))
	return g.httpRouteManager.addDeploymentRoute(ctx, logger, request)
}

// DeleteHTTPRoute deletes an HTTPRoute by name and namespace
func (g *gateway) DeleteHTTPRoute(ctx context.Context, logger *zap.Logger, httpRouteName, namespace string) error {
	logger.Info("Deleting HTTPRoute", zap.String("httpRoute", httpRouteName), zap.String("namespace", namespace))
	return g.httpRouteManager.deleteHTTPRoute(ctx, logger, httpRouteName, namespace)
}
