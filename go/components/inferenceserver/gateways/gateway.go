package gateways

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// gateway implements the Gateway interface
type gateway struct {
	httpClient *http.Client
	kubeClient client.Client

	registry *registry

	modelConfigMapProvider configmap.ModelConfigMapProvider
}

type Params struct {
	Logger                 *zap.Logger
	KubeClient             client.Client
	ModelConfigMapProvider configmap.ModelConfigMapProvider
}

// NewGatewayWithClients creates a new inference server gateway with Kubernetes clients
func NewGatewayWithClients(p Params) Gateway {
	gateway := &gateway{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		kubeClient: p.KubeClient,
		registry:   newRegistry(),

		modelConfigMapProvider: p.ModelConfigMapProvider,
	}

	// Register Triton backend with its endpoint configuration
	gateway.registry.registerBackend(v2pb.BACKEND_TYPE_TRITON, backends.NewTritonBackend(p.KubeClient, p.ModelConfigMapProvider, p.Logger))
	return gateway
}

// LoadModel initiates loading a model into an inference server
func (g *gateway) LoadModel(ctx context.Context, logger *zap.Logger, modelName string, storagePath string, inferenceServerName string, namespace string, backendType v2pb.BackendType) error {
	logger.Info("Loading model", zap.String("model", modelName), zap.String("storagePath", storagePath), zap.String("inferenceServerName", inferenceServerName), zap.String("namespace", namespace), zap.String("backendType", backendType.String()))
	// Currrently, the only way to load a model is to append to an inference server's configmap
	if err := g.modelConfigMapProvider.AddModelToConfigMap(ctx, inferenceServerName, namespace, configmap.ModelConfigEntry{
		Name:        modelName,
		StoragePath: storagePath,
	}); err != nil {
		logger.Error("failed to initiate model loading", zap.Error(err), zap.String("operation", "load_model"), zap.String("model", modelName), zap.String("inferenceServerName", inferenceServerName), zap.String("namespace", namespace), zap.String("backendType", backendType.String()))
		return fmt.Errorf("failed to initiate model loading: %w", err)
	}
	logger.Info("successfully initiated model loading", zap.String("model", modelName), zap.String("inferenceServerName", inferenceServerName), zap.String("namespace", namespace), zap.String("backendType", backendType.String()))
	return nil
}

// UnloadModel initiates unloading a model from an inference server
func (g *gateway) UnloadModel(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, backendType v2pb.BackendType) error {
	logger.Info("Unloading model", zap.String("model", modelName), zap.String("inferenceServerName", inferenceServerName), zap.String("namespace", namespace), zap.String("backendType", backendType.String()))
	// Currrently, the only way to unload a model is to remove it from an inference server's configmap
	if err := g.modelConfigMapProvider.RemoveModelFromConfigMap(ctx, inferenceServerName, namespace, modelName); err != nil {
		logger.Error("failed to initiate model unloading", zap.Error(err), zap.String("operation", "unload_model"), zap.String("model", modelName), zap.String("inferenceServerName", inferenceServerName), zap.String("namespace", namespace), zap.String("backendType", backendType.String()))
		return fmt.Errorf("failed to initiate model unloading: %w", err)
	}
	logger.Info("successfully initiated model unloading", zap.String("model", modelName), zap.String("inferenceServerName", inferenceServerName), zap.String("namespace", namespace), zap.String("backendType", backendType.String()))
	return nil
}

// CheckModelStatus dispatches model status checking based on backend type
func (g *gateway) CheckModelStatus(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, backendType v2pb.BackendType) (bool, error) {
	logger.Info("Checking model status", zap.String("model", modelName), zap.String("inferenceServerName", inferenceServerName), zap.String("namespace", namespace), zap.String("backendType", backendType.String()))
	if backendType == v2pb.BACKEND_TYPE_INVALID {
		return false, fmt.Errorf("invalid backend type: %v", backendType)
	}
	backend, err := g.registry.getBackend(backendType)
	if err != nil {
		logger.Error("failed to get backend", zap.Error(err), zap.String("operation", "check_model_status"), zap.String("model", modelName), zap.String("inferenceServerName", inferenceServerName), zap.String("namespace", namespace), zap.String("backendType", backendType.String()))
		return false, fmt.Errorf("failed to get backend for model %s on %s/%s: %w", modelName, namespace, inferenceServerName, err)
	}
	return backend.CheckModelStatus(ctx, logger, modelName, inferenceServerName, namespace)
}

// CheckModelExists checks if a model exists in an inference server.
func (g *gateway) CheckModelExists(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, backendType v2pb.BackendType) (bool, error) {
	logger.Info("Checking model exists", zap.String("model", modelName), zap.String("inferenceServerName", inferenceServerName), zap.String("namespace", namespace), zap.String("backendType", backendType.String()))
	currentConfigs, err := g.modelConfigMapProvider.GetModelsFromConfigMap(ctx, inferenceServerName, namespace)
	if err != nil {
		logger.Error("failed to check if model exists in inference server", zap.Error(err), zap.String("operation", "check_model_exists"), zap.String("model", modelName), zap.String("inferenceServerName", inferenceServerName), zap.String("namespace", namespace), zap.String("backendType", backendType.String()))
		return false, fmt.Errorf("failed to check existance of model %s in inference server %s in namespace %s: %w", modelName, inferenceServerName, namespace, err)
	}

	for _, config := range currentConfigs {
		if config.Name == modelName {
			return true, nil
		}
	}
	return false, nil
}

// IsHealthy dispatches health checking based on backend type
func (g *gateway) InferenceServerIsHealthy(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string, backendType v2pb.BackendType) (bool, error) {
	logger.Info("Checking server health", zap.String("server", inferenceServerName), zap.String("namespace", namespace), zap.String("backendType", backendType.String()))
	if backendType == v2pb.BACKEND_TYPE_INVALID {
		return false, fmt.Errorf("invalid backend type: %v", backendType)
	}
	backend, err := g.registry.getBackend(backendType)
	if err != nil {
		return false, fmt.Errorf("unable to get backend for inference server %s in namespace %s: %w", inferenceServerName, namespace, err)
	}
	return backend.IsHealthy(ctx, logger, inferenceServerName, namespace)
}
