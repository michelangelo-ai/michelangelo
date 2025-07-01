package plugins

import (
	"context"

	"github.com/go-logr/logr"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Provider defines the interface for inference providers
type Provider interface {
	// LoadModel triggers model loading on the target inference server
	LoadModel(ctx context.Context, request ModelLoadRequest) error

	// GetModelStatus checks the status of a model on the inference server
	GetModelStatus(ctx context.Context, modelName, modelVersion string) (*ModelStatus, error)

	// UnloadModel removes a model from the inference server
	UnloadModel(ctx context.Context, modelName, modelVersion string) error

	// IsHealthy checks if the inference server is healthy
	IsHealthy(ctx context.Context) (bool, error)

	// GetType returns the provider type
	GetType() string
}

// ConditionActor defines an actor that can evaluate deployment conditions
type ConditionActor interface {
	// GetType returns the actor type
	GetType() string

	// EvaluateCondition evaluates whether the actor's condition is met
	EvaluateCondition(ctx context.Context, requestCtx RequestContext, logger logr.Logger) (*apipb.Condition, error)

	// Execute performs the actor's main operation
	Execute(ctx context.Context, requestCtx RequestContext, logger logr.Logger) (*apipb.Condition, error)
}

// RequestContext contains the context for actor operations
type RequestContext struct {
	Deployment *v2pb.Deployment
	Model      *v2pb.Model
}

// ModelLoadRequest contains information needed to load a model
type ModelLoadRequest struct {
	ModelName       string
	ModelVersion    string
	PackagePath     string
	InferenceServer string
	BackendType     v2pb.BackendType
	Config          map[string]string
}

// ModelStatus represents the status of a model
type ModelStatus struct {
	State   string // LOADING, LOADED, FAILED, NOT_FOUND
	Message string
	Ready   bool
}

// Provider types
const (
	ProviderTypeTriton = "triton"
	ProviderTypeLLMD   = "llmd"
	ProviderTypeDynamo = "dynamo"
)