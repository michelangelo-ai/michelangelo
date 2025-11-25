package gateways

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// gateway implements the Gateway interface
type gateway struct {
	httpClient    *http.Client
	kubeClient    client.Client
	dynamicClient dynamic.Interface

	registry *registry

	modelConfigMapProvider configmap.ModelConfigMapProvider
}

type Params struct {
	KubeClient             client.Client
	DynamicClient          dynamic.Interface
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
	backendType, err := g.ensureInferenceServerBackendType(ctx, request.BackendType, request.InferenceServer.Name, request.InferenceServer.Namespace)
	if err != nil {
		return nil, err
	}
	backend, err := g.registry.getBackend(backendType)
	if err != nil {
		return nil, err
	}
	return backend.CreateInfrastructure(ctx, logger, request)
}

// GetInfrastructureStatus dispatches infrastructure status checking based on backend type
func (g *gateway) GetInfrastructureStatus(ctx context.Context, logger *zap.Logger, request GetInfrastructureStatusRequest) (*GetInfrastructureStatusResponse, error) {
	logger.Info("Getting infrastructure status", zap.String("server", request.InferenceServer), zap.String("backend", request.BackendType.String()))
	backendType, err := g.ensureInferenceServerBackendType(ctx, request.BackendType, request.InferenceServer, request.Namespace)
	if err != nil {
		return nil, err
	}
	backend, err := g.registry.getBackend(backendType)
	if err != nil {
		return nil, err
	}
	return backend.GetInfrastructureStatus(ctx, logger, request)
}

// DeleteInfrastructure dispatches infrastructure deletion based on backend type
func (g *gateway) DeleteInfrastructure(ctx context.Context, logger *zap.Logger, request DeleteInfrastructureRequest) error {
	logger.Info("Deleting infrastructure", zap.String("server", request.InferenceServer), zap.String("backend", request.BackendType.String()))
	backendType, err := g.ensureInferenceServerBackendType(ctx, request.BackendType, request.InferenceServer, request.Namespace)
	if err != nil {
		return err
	}
	backend, err := g.registry.getBackend(backendType)
	if err != nil {
		return err
	}
	return backend.DeleteInfrastructure(ctx, logger, request)
}

// IsHealthy dispatches health checking based on backend type
func (g *gateway) IsHealthy(ctx context.Context, logger *zap.Logger, request HealthCheckRequest) (bool, error) {
	logger.Info("Checking server health", zap.String("server", request.InferenceServer), zap.String("backend", request.BackendType.String()))
	backendType, err := g.ensureInferenceServerBackendType(ctx, request.BackendType, request.InferenceServer, request.Namespace)
	if err != nil {
		return false, err
	}
	backend, err := g.registry.getBackend(backendType)
	if err != nil {
		return false, fmt.Errorf("unable to get backend for inference server %s in namespace %s: %w", request.InferenceServer, request.Namespace, err)
	}
	return backend.IsHealthy(ctx, logger, request)
}

// Model Management Methods

// LoadModel dispatches model loading based on backend type
func (g *gateway) LoadModel(ctx context.Context, logger *zap.Logger, request LoadModelRequest) error {
	logger.Info("Loading model", zap.String("model", request.ModelName), zap.String("backend", request.BackendType.String()))
	backendType, err := g.ensureInferenceServerBackendType(ctx, request.BackendType, request.InferenceServer, request.Namespace)
	if err != nil {
		return err
	}
	backend, err := g.registry.getBackend(backendType)
	if err != nil {
		return err
	}
	return backend.LoadModel(ctx, logger, request)
}

// UnloadModel dispatches model unloading based on backend type
func (g *gateway) UnloadModel(ctx context.Context, logger *zap.Logger, request UnloadModelRequest) error {
	logger.Info("Unloading model", zap.String("model", request.ModelName), zap.String("backend", request.BackendType.String()))
	backendType, err := g.ensureInferenceServerBackendType(ctx, request.BackendType, request.InferenceServer, request.Namespace)
	if err != nil {
		return err
	}
	backend, err := g.registry.getBackend(backendType)
	if err != nil {
		return err
	}
	return backend.UnloadModel(ctx, logger, request)
}

// CheckModelStatus dispatches model status checking based on backend type
func (g *gateway) CheckModelStatus(ctx context.Context, logger *zap.Logger, request CheckModelStatusRequest) (bool, error) {
	logger.Info("Checking model status", zap.String("model", request.ModelName), zap.String("backend", request.BackendType.String()))
	backendType, err := g.ensureInferenceServerBackendType(ctx, request.BackendType, request.InferenceServer, request.Namespace)
	if err != nil {
		return false, err
	}
	backend, err := g.registry.getBackend(backendType)
	if err != nil {
		return false, err
	}
	return backend.CheckModelStatus(ctx, logger, request)
}

func (g *gateway) ensureInferenceServerBackendType(ctx context.Context, backendType v2pb.BackendType, inferenceServerName, namespace string) (v2pb.BackendType, error) {
	if backendType != v2pb.BACKEND_TYPE_INVALID {
		return backendType, nil
	}
	inferredBackendType := g.getInferenceServerBackendType(ctx, inferenceServerName, namespace)
	if inferredBackendType == v2pb.BACKEND_TYPE_INVALID {
		return v2pb.BACKEND_TYPE_INVALID, fmt.Errorf("unable to get backend type for inference server %s in namespace %s", inferenceServerName, namespace)
	}
	return inferredBackendType, nil
}

func (g *gateway) getInferenceServerBackendType(ctx context.Context, inferenceServerName, namespace string) v2pb.BackendType {
	inferenceServer := &v2pb.InferenceServer{}
	err := g.kubeClient.Get(ctx, client.ObjectKey{
		Name:      inferenceServerName,
		Namespace: namespace,
	}, inferenceServer)
	if err != nil {
		return v2pb.BACKEND_TYPE_INVALID
	}
	return inferenceServer.Spec.GetBackendType()
}
