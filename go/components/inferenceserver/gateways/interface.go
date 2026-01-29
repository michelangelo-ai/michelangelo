//go:generate mamockgen Gateway

package gateways

import (
	"context"

	"go.uber.org/zap"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Gateway provides a unified interface for inteacting with inference servers across different backend types.
type Gateway interface {
	// LoadModel initiates loading a model into an inference server.
	// This is an asynchronous operation and returns immediately. The caller is responsible for ensuring whether the model is loaded successfully.
	LoadModel(ctx context.Context, logger *zap.Logger, modelName string, storagePath string, inferenceServerName string, namespace string, backendType v2pb.BackendType) error
	// UnloadModel removes a model from an inference server.
	// This is an asynchronous operation and returns immediately. The caller is responsible for ensuring whether the model is unloaded successfully.
	UnloadModel(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, backendType v2pb.BackendType) error
	// CheckModelStatus verifies if a model is ready to serve requests.
	CheckModelStatus(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, backendType v2pb.BackendType) (bool, error)
	// CheckModelExists checks if a model exists in an inference server.
	CheckModelExists(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, backendType v2pb.BackendType) (bool, error)
	// InferenceServerIsHealthy checks if the inference server is healthy.
	InferenceServerIsHealthy(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string, backendType v2pb.BackendType) (bool, error)
}
