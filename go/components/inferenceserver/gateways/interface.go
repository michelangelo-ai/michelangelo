//go:generate mamockgen Gateway

package gateways

import (
	"context"

	"go.uber.org/zap"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

type DeploymentTargetInfo struct {
	BackendType    v2pb.BackendType
	ClusterTargets []*v2pb.ClusterTarget
}

// Gateway provides a unified interface for interacting with inference servers across different backend types.
type Gateway interface {
	// GetDeploymentTargetInfo returns the backend type and cluster targets for an inference server.
	// Use this to retrieve the ClusterTarget configs needed to call other Gateway methods (e.g., LoadModel, CheckModelStatus).
	GetDeploymentTargetInfo(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) (*DeploymentTargetInfo, error)
	// LoadModel initiates loading a model into an inference server.
	// This is an asynchronous operation and returns immediately. The caller is responsible for ensuring whether the model is loaded successfully.
	LoadModel(ctx context.Context, logger *zap.Logger, modelName string, storagePath string, inferenceServerName string, namespace string, targetCluster *v2pb.ClusterTarget) error
	// UnloadModel removes a model from an inference server.
	// This is an asynchronous operation and returns immediately. The caller is responsible for ensuring whether the model is unloaded successfully.
	UnloadModel(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, targetCluster *v2pb.ClusterTarget) error
	// CheckModelStatus verifies if a model is ready to serve requests.
	CheckModelStatus(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, targetCluster *v2pb.ClusterTarget, backendType v2pb.BackendType) (bool, error)
	// CheckModelExists checks if a model exists in an inference server.
	CheckModelExists(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, targetCluster *v2pb.ClusterTarget, backendType v2pb.BackendType) (bool, error)
	// InferenceServerIsHealthy checks if the inference server is healthy.
	InferenceServerIsHealthy(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string, targetCluster *v2pb.ClusterTarget, backendType v2pb.BackendType) (bool, error)
	// GetControlPlaneServiceName returns the name of the control plane service for an inference server.
	GetControlPlaneServiceName(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) (string, error)
}
