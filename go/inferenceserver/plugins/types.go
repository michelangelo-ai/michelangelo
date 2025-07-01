package plugins

import (
	"context"

	"github.com/go-logr/logr"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// InferenceServerPlugin defines the interface for backend-specific plugins
type InferenceServerPlugin interface {
	// GetType returns the backend type this plugin handles
	GetType() v2pb.BackendType
	
	// GetCreationActors returns the actors needed for infrastructure creation
	GetCreationActors() []ConditionActor
	
	// GetDeletionActors returns the actors needed for infrastructure cleanup
	GetDeletionActors() []ConditionActor
	
	// GetStatusActors returns the actors for status checking
	GetStatusActors() []ConditionActor
}

// ConditionActor defines discrete operations that can set conditions
type ConditionActor interface {
	// GetType returns the condition type this actor manages
	GetType() string
	
	// Execute performs the actor's operation
	Execute(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error
	
	// EvaluateCondition checks the current state and returns appropriate condition
	EvaluateCondition(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) (*apipb.Condition, error)
}

// PluginRegistry manages available plugins
type PluginRegistry interface {
	// RegisterPlugin registers a plugin for a specific backend type
	RegisterPlugin(backendType v2pb.BackendType, plugin InferenceServerPlugin)
	
	// GetPlugin returns the plugin for a given backend type
	GetPlugin(backendType v2pb.BackendType) (InferenceServerPlugin, error)
}

// ActorEngine executes a series of condition actors
type ActorEngine interface {
	// ExecuteActors runs actors sequentially and updates conditions
	ExecuteActors(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer, actors []ConditionActor) error
}