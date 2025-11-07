package gateways

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Gateway provides a unified interface for inference server operations across different providers
type Gateway interface {
	// Infrastructure Management
	CreateInfrastructure(ctx context.Context, logger *zap.Logger, request InfrastructureRequest) (*InfrastructureResponse, error)
	GetInfrastructureStatus(ctx context.Context, logger *zap.Logger, request InfrastructureStatusRequest) (*InfrastructureStatus, error)
	DeleteInfrastructure(ctx context.Context, logger *zap.Logger, request InfrastructureDeleteRequest) error

	// Proxy/Routing Management
	ConfigureProxy(ctx context.Context, logger *zap.Logger, request ProxyConfigRequest) error
	GetProxyStatus(ctx context.Context, logger *zap.Logger, request ProxyStatusRequest) (*ProxyStatus, error)
	AddDeploymentSpecificRoute(ctx context.Context, logger *zap.Logger, request ProxyConfigRequest) error

	// Model Management
	LoadModel(ctx context.Context, logger *zap.Logger, request ModelLoadRequest) error
	CheckModelStatus(ctx context.Context, logger *zap.Logger, request ModelStatusRequest) (bool, error)
	GetModelStatus(ctx context.Context, logger *zap.Logger, request ModelStatusRequest) (*ModelStatus, error)

	// Health Checking
	IsHealthy(ctx context.Context, logger *zap.Logger, serverName string, backendType v2pb.BackendType) (bool, error)

	// Model Configuration Updates (for rolling out new models)
	UpdateModelConfig(ctx context.Context, logger *zap.Logger, request ModelConfigUpdateRequest) error

	// ConfigMap Management
	CreateModelConfigMap(ctx context.Context, logger *zap.Logger, request ModelConfigMapRequest) error
	UpdateModelConfigMap(ctx context.Context, logger *zap.Logger, request ModelConfigMapRequest) error
	DeleteModelConfigMap(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace string) error
	DeleteConfigMap(ctx context.Context, logger *zap.Logger, configMapName, namespace string) error

	// HTTPRoute Management
	DeleteHTTPRoute(ctx context.Context, logger *zap.Logger, httpRouteName, namespace string) error
}

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
	ModelConfigs    []ModelConfigEntry
}

// ModelConfigMapRequest contains information needed to create/update model ConfigMaps
type ModelConfigMapRequest struct {
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
	ModelConfigs    []ModelConfigEntry
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

// gateway implements the Gateway interface
type gateway struct {
	httpClient        *http.Client
	kubeClient        client.Client
	dynamicClient     dynamic.Interface
	configMapProvider *ConfigMapProvider
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
	return &gateway{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		kubeClient:        kubeClient,
		dynamicClient:     dynamicClient,
		configMapProvider: NewConfigMapProvider(kubeClient, logger),
	}
}

// LoadModel dispatches model loading based on backend type
func (g *gateway) LoadModel(ctx context.Context, logger *zap.Logger, request ModelLoadRequest) error {
	logger.Info("Loading model", zap.String("model", request.ModelName), zap.String("backend", request.BackendType.String()))

	switch request.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return g.loadTritonModel(ctx, logger, request)
	// TODO: Implement other backend types: LLMD, Dynamo, TorchServe
	default:
		return fmt.Errorf("unsupported backend type: %v", request.BackendType)
	}
}

// CheckModelStatus dispatches model status checking based on backend type
func (g *gateway) CheckModelStatus(ctx context.Context, logger *zap.Logger, request ModelStatusRequest) (bool, error) {
	logger.Info("Checking model status", zap.String("model", request.ModelName), zap.String("backend", request.BackendType.String()))

	switch request.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return g.checkTritonModelStatus(ctx, logger, request)
	// TODO: Implement other backend types: LLMD, Dynamo, TorchServe
	default:
		return false, fmt.Errorf("unsupported backend type: %v", request.BackendType)
	}
}

// GetModelStatus dispatches detailed model status retrieval based on backend type
func (g *gateway) GetModelStatus(ctx context.Context, logger *zap.Logger, request ModelStatusRequest) (*ModelStatus, error) {
	logger.Info("Getting model status", zap.String("model", request.ModelName), zap.String("backend", request.BackendType.String()))

	switch request.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return g.getTritonModelStatus(ctx, logger, request)
	// TODO: Implement other backend types: LLMD, Dynamo, TorchServe
	default:
		return nil, fmt.Errorf("unsupported backend type: %v", request.BackendType)
	}
}

// IsHealthy dispatches health checking based on backend type
func (g *gateway) IsHealthy(ctx context.Context, logger *zap.Logger, serverName string, backendType v2pb.BackendType) (bool, error) {
	logger.Info("Checking server health", zap.String("server", serverName), zap.String("backend", backendType.String()))

	switch backendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return g.isTritonHealthy(ctx, logger, serverName)
	// TODO: Implement other backend types: LLMD, Dynamo, TorchServe
	default:
		return false, fmt.Errorf("unsupported backend type: %v", backendType)
	}
}

// UpdateModelConfig updates model configuration for rolling out new models
func (g *gateway) UpdateModelConfig(ctx context.Context, logger *zap.Logger, request ModelConfigUpdateRequest) error {
	logger.Info("Updating model configuration", zap.String("server", request.InferenceServer), zap.String("backend", request.BackendType.String()), zap.Int("models", len(request.ModelConfigs)))

	switch request.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		// Use ConfigMap provider to update model configuration
		if g.configMapProvider == nil {
			return fmt.Errorf("ConfigMap provider not initialized")
		}
		configMapRequest := ConfigMapRequest{
			InferenceServer: request.InferenceServer,
			Namespace:       request.Namespace,
			BackendType:     request.BackendType,
			ModelConfigs:    request.ModelConfigs,
		}
		return g.configMapProvider.UpdateModelConfigMap(ctx, configMapRequest)
	// TODO: Implement other backend types: LLMD, Dynamo, TorchServe
	default:
		return fmt.Errorf("unsupported backend type: %v", request.BackendType)
	}
}

// ConfigMap Management implementations

// CreateModelConfigMap creates a ConfigMap for model configuration
func (g *gateway) CreateModelConfigMap(ctx context.Context, logger *zap.Logger, request ModelConfigMapRequest) error {
	if g.configMapProvider == nil {
		return fmt.Errorf("ConfigMap provider not initialized")
	}

	configMapRequest := ConfigMapRequest{
		InferenceServer: request.InferenceServer,
		Namespace:       request.Namespace,
		BackendType:     request.BackendType,
		ModelConfigs:    convertToConfigMapEntries(request.ModelConfigs),
		Labels:          request.Labels,
		Annotations:     request.Annotations,
	}

	return g.configMapProvider.CreateModelConfigMap(ctx, configMapRequest)
}

// UpdateModelConfigMap updates a ConfigMap for model configuration
func (g *gateway) UpdateModelConfigMap(ctx context.Context, _ *zap.Logger, request ModelConfigMapRequest) error {
	if g.configMapProvider == nil {
		return fmt.Errorf("ConfigMap provider not initialized")
	}

	configMapRequest := ConfigMapRequest{
		InferenceServer: request.InferenceServer,
		Namespace:       request.Namespace,
		BackendType:     request.BackendType,
		ModelConfigs:    convertToConfigMapEntries(request.ModelConfigs),
		Labels:          request.Labels,
		Annotations:     request.Annotations,
	}

	return g.configMapProvider.UpdateModelConfigMap(ctx, configMapRequest)
}

// DeleteModelConfigMap deletes a ConfigMap for model configuration
func (g *gateway) DeleteModelConfigMap(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace string) error {
	if g.configMapProvider == nil {
		return fmt.Errorf("ConfigMap provider not initialized")
	}

	return g.configMapProvider.DeleteModelConfigMap(ctx, inferenceServerName, namespace)
}

// Helper function to convert between types
func convertToConfigMapEntries(entries []ModelConfigEntry) []ModelConfigEntry {
	result := make([]ModelConfigEntry, len(entries))
	for i, entry := range entries {
		result[i] = ModelConfigEntry{
			Name:   entry.Name,
			S3Path: entry.S3Path,
		}
	}
	return result
}

// Infrastructure Management Methods

// CreateInfrastructure dispatches infrastructure creation based on backend type
func (g *gateway) CreateInfrastructure(ctx context.Context, logger *zap.Logger, request InfrastructureRequest) (*InfrastructureResponse, error) {
	logger.Info("Creating infrastructure", zap.String("server", request.InferenceServer.Name), zap.String("backend", request.BackendType.String()))

	switch request.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return g.createTritonInfrastructure(ctx, logger, request)
		// TODO: Implement other backend types: LLMD, Dynamo, TorchServ

	default:
		return nil, fmt.Errorf("unsupported backend type: %v", request.BackendType)
	}
}

// GetInfrastructureStatus dispatches infrastructure status checking based on backend type
func (g *gateway) GetInfrastructureStatus(ctx context.Context, logger *zap.Logger, request InfrastructureStatusRequest) (*InfrastructureStatus, error) {
	logger.Info("Getting infrastructure status", zap.String("server", request.InferenceServer), zap.String("backend", request.BackendType.String()))

	switch request.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return g.getTritonInfrastructureStatus(ctx, logger, request)
	// TODO: Implement other backend types: LLMD, Dynamo, TorchServe
	default:
		return nil, fmt.Errorf("unsupported backend type: %v", request.BackendType)
	}
}

// DeleteInfrastructure dispatches infrastructure deletion based on backend type
func (g *gateway) DeleteInfrastructure(ctx context.Context, logger *zap.Logger, request InfrastructureDeleteRequest) error {
	logger.Info("Deleting infrastructure", zap.String("server", request.InferenceServer), zap.String("backend", request.BackendType.String()))

	switch request.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return g.deleteTritonInfrastructure(ctx, logger, request)
	// TODO: Implement other backend types: LLMD, Dynamo, TorchServe
	default:
		return fmt.Errorf("unsupported backend type: %v", request.BackendType)
	}
}

// Proxy Management Methods

// ConfigureProxy sets up Istio VirtualService routing
func (g *gateway) ConfigureProxy(ctx context.Context, logger *zap.Logger, request ProxyConfigRequest) error {
	logger.Info("Configuring proxy", zap.String("server", request.InferenceServer), zap.String("model", request.ModelName))

	// Use the same Istio logic from the deployment provider
	return g.configureIstioProxy(ctx, logger, request)
}

// GetProxyStatus checks the status of Istio VirtualService configuration
func (g *gateway) GetProxyStatus(ctx context.Context, logger *zap.Logger, request ProxyStatusRequest) (*ProxyStatus, error) {
	logger.Info("Getting proxy status", zap.String("server", request.InferenceServer))

	return g.getIstioProxyStatus(ctx, logger, request)
}

// AddDeploymentSpecificRoute adds a deployment-specific route to the HTTPRoute
func (g *gateway) AddDeploymentSpecificRoute(ctx context.Context, logger *zap.Logger, request ProxyConfigRequest) error {
	logger.Info("Adding deployment-specific route", zap.String("server", request.InferenceServer), zap.String("deployment", request.DeploymentName), zap.String("model", request.ModelName))

	return g.addDeploymentSpecificRoute(ctx, logger, request)
}

// DeleteConfigMap deletes a ConfigMap by name and namespace
func (g *gateway) DeleteConfigMap(ctx context.Context, logger *zap.Logger, configMapName, namespace string) error {
	logger.Info("Deleting ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", namespace))

	if g.configMapProvider == nil {
		return fmt.Errorf("ConfigMapProvider not available")
	}

	// Use direct Kubernetes client to delete ConfigMap
	configMap := &corev1.ConfigMap{}
	configMap.Name = configMapName
	configMap.Namespace = namespace

	if err := g.kubeClient.Delete(ctx, configMap); err != nil {
		// Ignore not found errors as the ConfigMap may already be deleted
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to delete ConfigMap %s in namespace %s: %w", configMapName, namespace, err)
		}
		logger.Info("ConfigMap not found, already deleted", zap.String("configMap", configMapName))
	} else {
		logger.Info("ConfigMap deleted successfully", zap.String("configMap", configMapName), zap.String("namespace", namespace))
	}

	return nil
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
