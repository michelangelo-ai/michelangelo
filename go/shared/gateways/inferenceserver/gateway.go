package inferenceserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Gateway provides a unified interface for inference server operations across different providers
type Gateway interface {
	// LoadModel triggers model loading on the target inference server
	LoadModel(ctx context.Context, logger logr.Logger, request ModelLoadRequest) error

	// CheckModelStatus checks if a model is loaded and ready on the inference server
	CheckModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (bool, error)

	// GetModelStatus gets detailed status information about a model
	GetModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (*ModelStatus, error)

	// IsHealthy checks if the inference server is healthy
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

// gateway implements the Gateway interface
type gateway struct {
	httpClient *http.Client
}

// NewGateway creates a new inference server gateway
func NewGateway() Gateway {
	return &gateway{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
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
	default:
		return false, fmt.Errorf("unsupported backend type: %v", backendType)
	}
}