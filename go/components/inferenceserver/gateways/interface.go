//go:generate mamockgen Gateway

package gateways

import (
	"context"

	"go.uber.org/zap"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// TargetClusterConnection contains the connection details for a Kubernetes cluster
// where an inference server is deployed. It is used by Gateway methods to connect
// to the target cluster and perform model operations (load, unload, health check).
type TargetClusterConnection struct {
	// ClusterId is the unique identifier for the target cluster.
	ClusterId string `json:"cluster_id"`
	// Host is the API server hostname or IP address of the target cluster.
	Host string `json:"host"`
	// Port is the API server port of the target cluster.
	Port string `json:"port"`
	// TokenTag is the key used to retrieve the authentication token from a secret.
	TokenTag string `json:"token_tag"`
	// CaDataTag is the key used to retrieve the CA certificate data from a secret.
	CaDataTag string `json:"ca_data_tag"`
	// IsControlPlaneCluster indicates whether this cluster is the control plane cluster.
	// The control plane cluster hosts the central routing and service discovery components.
	IsControlPlaneCluster bool `json:"is_control_plane_cluster"`
}

// DeploymentTargetInfo contains the target cluster information for a deployment.
// It is returned by GetDeploymentTargetInfo and used to determine where models
// should be loaded and how to connect to those clusters.
type DeploymentTargetInfo struct {
	// BackendType specifies the inference server backend (e.g., Triton, vLLM).
	BackendType v2pb.BackendType
	// ClusterTargets is the list of clusters where the inference server is deployed.
	ClusterTargets []*TargetClusterConnection
}

// Gateway provides a unified interface for interacting with inference servers across different backend types.
type Gateway interface {
	// GetDeploymentTargetInfo returns the backend type and cluster targets for an inference server.
	// Use this to retrieve the ClusterTarget configs needed to call other Gateway methods (e.g., LoadModel, CheckModelStatus).
	GetDeploymentTargetInfo(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) (*DeploymentTargetInfo, error)
	// LoadModel initiates loading a model into an inference server.
	// This is an asynchronous operation and returns immediately. The caller is responsible for ensuring whether the model is loaded successfully.
	LoadModel(ctx context.Context, logger *zap.Logger, modelName string, storagePath string, inferenceServerName string, namespace string, targetCluster *TargetClusterConnection) error
	// UnloadModel removes a model from an inference server.
	// This is an asynchronous operation and returns immediately. The caller is responsible for ensuring whether the model is unloaded successfully.
	UnloadModel(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, targetCluster *TargetClusterConnection) error
	// CheckModelStatus verifies if a model is ready to serve requests.
	CheckModelStatus(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, targetCluster *TargetClusterConnection, backendType v2pb.BackendType) (bool, error)
	// CheckModelExists checks if a model exists in an inference server.
	CheckModelExists(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, targetCluster *TargetClusterConnection, backendType v2pb.BackendType) (bool, error)
	// InferenceServerIsHealthy checks if the inference server is healthy.
	InferenceServerIsHealthy(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string, targetCluster *TargetClusterConnection, backendType v2pb.BackendType) (bool, error)
	// GetControlPlaneServiceName returns the name of the control plane service for an inference server.
	GetControlPlaneServiceName(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) (string, error)
}
