//go:generate mamockgen Gateway

package gateways

import (
	"context"

	"go.uber.org/zap"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// CreateInfrastructureRequest specifies the parameters for creating inference server infrastructure.
type CreateInfrastructureRequest struct {
	InferenceServer *v2pb.InferenceServer
	BackendType     v2pb.BackendType
	Namespace       string
	Resources       ResourceSpec
}

// CreateInfrastructureResponse provides the result of infrastructure creation operations.
type CreateInfrastructureResponse struct {
	State     v2pb.InferenceServerState
	Message   string
	Endpoints []string
	Details   map[string]interface{}
}

// ResourceSpec defines the resource allocation for inference server infrastructure.
type ResourceSpec struct {
	CPU         string
	Memory      string
	GPU         int32
	Replicas    int32
	ImageTag    string
	ModelConfig map[string]string
}

// DeleteInfrastructureRequest specifies the parameters for deleting inference server infrastructure.
type DeleteInfrastructureRequest struct {
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
}

// LoadModelRequest specifies the parameters for loading a model into an inference server.
type LoadModelRequest struct {
	ModelName       string
	ModelVersion    string
	PackagePath     string
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
	Config          map[string]string
}

// UnloadModelRequest specifies the parameters for unloading a model from an inference server.
type UnloadModelRequest struct {
	ModelName       string
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
}

// CheckModelStatusRequest specifies the parameters for checking model readiness and health.
type CheckModelStatusRequest struct {
	ModelName       string
	ModelVersion    string
	Namespace       string
	DeploymentName  string
	InferenceServer string
	BackendType     v2pb.BackendType
}

// CheckModelStatusResponse provides the model status check result.
type CheckModelStatusResponse struct {
	Status ModelStatus
}

// ModelStatus represents the current state and readiness of a loaded model.
type ModelStatus struct {
	State   v2pb.InferenceServerState
	Message string
	Ready   bool
}

// GetInfrastructureStatusRequest specifies the parameters for querying infrastructure status.
type GetInfrastructureStatusRequest struct {
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
}

// GetInfrastructureStatusResponse provides the infrastructure status query result.
type GetInfrastructureStatusResponse struct {
	Status InfrastructureStatus
}

// InfrastructureStatus represents the current state and health of inference server infrastructure.
type InfrastructureStatus struct {
	State     v2pb.InferenceServerState
	Message   string
	Ready     bool
	Endpoints []string
}

// HealthCheckRequest specifies the parameters for checking inference server health.
type HealthCheckRequest struct {
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
}

// Gateway provides a unified interface for managing inference servers across different backend types.
// It acts as a registry and dispatcher for backend-specific implementations.
type Gateway interface {
	// RegisterBackend associates a backend implementation with a specific backend type.
	RegisterBackend(backendType v2pb.BackendType, backend Backend)

	// CreateInfrastructure provisions the Kubernetes resources for an inference server.
	CreateInfrastructure(ctx context.Context, logger *zap.Logger, request CreateInfrastructureRequest) (*CreateInfrastructureResponse, error)

	// GetInfrastructureStatus queries the current state of inference server infrastructure.
	GetInfrastructureStatus(ctx context.Context, logger *zap.Logger, request GetInfrastructureStatusRequest) (*GetInfrastructureStatusResponse, error)

	// DeleteInfrastructure removes all Kubernetes resources for an inference server.
	DeleteInfrastructure(ctx context.Context, logger *zap.Logger, request DeleteInfrastructureRequest) error

	// LoadModel initiates loading a model into an inference server.
	LoadModel(ctx context.Context, logger *zap.Logger, request LoadModelRequest) error

	// UnloadModel removes a model from an inference server.
	UnloadModel(ctx context.Context, logger *zap.Logger, request UnloadModelRequest) error

	// CheckModelStatus verifies if a model is ready to serve requests.
	CheckModelStatus(ctx context.Context, logger *zap.Logger, request CheckModelStatusRequest) (bool, error)

	// IsHealthy checks if the inference server infrastructure is operational.
	IsHealthy(ctx context.Context, logger *zap.Logger, request HealthCheckRequest) (bool, error)
}

// Backend defines the interface for inference server backend implementations (Triton, vLLM, etc.).
// Each backend provides platform-specific logic for infrastructure and model management.
type Backend interface {
	// CreateInfrastructure provisions backend-specific Kubernetes resources.
	CreateInfrastructure(ctx context.Context, logger *zap.Logger, request CreateInfrastructureRequest) (*CreateInfrastructureResponse, error)

	// GetInfrastructureStatus queries the backend-specific infrastructure state.
	GetInfrastructureStatus(ctx context.Context, logger *zap.Logger, request GetInfrastructureStatusRequest) (*GetInfrastructureStatusResponse, error)

	// DeleteInfrastructure removes backend-specific Kubernetes resources.
	DeleteInfrastructure(ctx context.Context, logger *zap.Logger, request DeleteInfrastructureRequest) error

	// IsHealthy checks backend-specific health endpoints.
	IsHealthy(ctx context.Context, logger *zap.Logger, request HealthCheckRequest) (bool, error)

	// LoadModel loads a model using backend-specific APIs.
	LoadModel(ctx context.Context, logger *zap.Logger, request LoadModelRequest) error

	// UnloadModel unloads a model using backend-specific APIs.
	UnloadModel(ctx context.Context, logger *zap.Logger, request UnloadModelRequest) error

	// CheckModelStatus checks model readiness using backend-specific APIs.
	CheckModelStatus(ctx context.Context, logger *zap.Logger, request CheckModelStatusRequest) (bool, error)
}
