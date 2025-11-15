package gateways

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/shared/configmap"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// gateway implements the Gateway interface
type gateway struct {
	httpClient *http.Client
	kubeClient client.Client

	registry          *registry
	configMapProvider configmap.ConfigMapProvider
	dynamicClient     dynamic.Interface
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
func NewGatewayWithClients(kubeClient client.Client, dynamicClient dynamic.Interface, logger *zap.Logger) Gateway {
	// Create configmap provider
	configMapProvider := configmap.NewDefaultConfigMapProvider(kubeClient, logger)

	return &gateway{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		kubeClient:        kubeClient,
		dynamicClient:     dynamicClient,
		registry:          newRegistry(),
		configMapProvider: configMapProvider,
	}
}

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
	logger.Info("Configuring proxy", zap.String("server", request.InferenceServer), zap.String("model", request.ModelName))
	// Use the same Istio logic from the deployment provider
	return g.configureIstioProxy(ctx, logger, request)
}

// GetProxyStatus checks the status of Istio VirtualService configuration
func (g *gateway) GetProxyStatus(ctx context.Context, logger *zap.Logger, request GetProxyStatusRequest) (*GetProxyStatusResponse, error) {
	logger.Info("Getting proxy status", zap.String("server", request.InferenceServer))
	return g.getIstioProxyStatus(ctx, logger, request)
}

// AddDeploymentSpecificRoute adds a deployment-specific route to the HTTPRoute
func (g *gateway) AddDeploymentRoute(ctx context.Context, logger *zap.Logger, request AddDeploymentRouteRequest) error {
	logger.Info("Adding deployment-specific route", zap.String("server", request.InferenceServer), zap.String("deployment", request.DeploymentName), zap.String("model", request.ModelName))

	return g.addDeploymentSpecificRoute(ctx, logger, request)
}

// DeleteHTTPRoute deletes an HTTPRoute by name and namespace
func (g *gateway) DeleteHTTPRoute(ctx context.Context, logger *zap.Logger, httpRouteName, namespace string) error {
	logger.Info("Deleting HTTPRoute", zap.String("httpRoute", httpRouteName), zap.String("namespace", namespace))
	if g.dynamicClient == nil {
		return fmt.Errorf("dynamicClient not available")
	}
	// Use the Gateway API HTTPRoute GroupVersionResource
	gvr := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}
	if err := g.dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, httpRouteName, metav1.DeleteOptions{}); err != nil {
		// Ignore not found errors as the HTTPRoute may already be deleted
		if errors.IsNotFound(err) {
			logger.Info("HTTPRoute not found, already deleted", zap.String("httpRoute", httpRouteName))
		} else {
			return fmt.Errorf("failed to delete HTTPRoute %s in namespace %s: %w", httpRouteName, namespace, err)
		}
	} else {
		logger.Info("HTTPRoute deleted successfully", zap.String("httpRoute", httpRouteName), zap.String("namespace", namespace))
	}

	return nil
}
