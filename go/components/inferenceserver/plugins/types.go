package plugins

import (
	"context"

	"go.uber.org/zap"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Plugin refers to a component that produces a list of actors, retrieves all current conditions for a given resource,
// and allows the placement of a condition within a resource. This matches Uber's proven Plugin interface.
type Plugin interface {
	// GetActors gets the list of ConditionActors for a particular plugin. The Engine will sequentially run through the
	// list of actors from this method.
	GetActors() []ConditionActor

	// GetConditions get the conditions for a particular Kubernetes custom resource.
	GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition

	// PutCondition puts a condition for a particular Kubernetes custom resource.
	PutCondition(resource *v2pb.InferenceServer, condition apipb.Condition)
}

// InferenceServerPlugin defines the interface for backend-specific plugins
// This provides higher-level plugin management for different lifecycle phases
type InferenceServerPlugin interface {
	// GetType returns the backend type this plugin handles
	GetType() v2pb.BackendType

	// GetCreationPlugin returns the plugin for infrastructure creation
	GetCreationPlugin() Plugin

	// GetDeletionPlugin returns the plugin for infrastructure cleanup
	GetDeletionPlugin(resource *v2pb.InferenceServer) Plugin
}

// ConditionActor refers to an implementation to collect and act upon a condition.
// This interface matches Uber's proven pattern exactly.
type ConditionActor interface {
	// Run runs the action that will attempt to move the condition status in the positive direction.
	// Run will be executed by the Engine for the first condition that is found to be negative.
	// Note that condition is a reference type, so it is modifiable from within the implementation. The engine will
	// ensure that only the ConditionActor that produces the condition can modify it.
	//
	// If there is a failure to perform any action, the plugin must set the appropriate properties in the provided
	// condition. Any errors that are returned are used only for logging purposes.
	Run(ctx context.Context, logger *zap.Logger, resource *v2pb.InferenceServer, condition *apipb.Condition) error

	// Retrieve retrieves a condition based on the previous apipb.Condition. The passed in apipb.Condition
	// can contain information saved from previous iterations of Retrieve and Run, which can be used to construct a
	// new condition. This should be implemented as a comparison with the expected state and the real world state.
	//
	// If there is a failure to retrieve the condition, the plugin must return a apipb.Condition with the
	// properties fulfilled. Any errors that are returned are used only for logging purposes.
	Retrieve(ctx context.Context, logger *zap.Logger, resource *v2pb.InferenceServer, condition apipb.Condition) (apipb.Condition, error)

	// GetType gets the type of the ConditionActor. This is used to determine apipb.Condition accountability.
	GetType() string
}

// PluginRegistry manages available plugins
type PluginRegistry interface {
	// RegisterPlugin registers a plugin for a specific backend type
	RegisterPlugin(backendType v2pb.BackendType, plugin InferenceServerPlugin)

	// GetPlugin returns the plugin for a given backend type
	GetPlugin(backendType v2pb.BackendType) (InferenceServerPlugin, error)
}

// Engine refers to the implementation that executes the conditional checks via the Retrieve method and runs the actions
// defined in the Run method of a ConditionActor. This matches Uber's proven Engine interface.
type Engine interface {
	// Run executes the plugin by running through the list of actors from the plugin and executing Retrieve and Run
	// for each actor. Only the first failing condition will have its Run method executed per engine execution.
	Run(ctx context.Context, logger *zap.Logger, plugin Plugin, resource *v2pb.InferenceServer) (*apipb.Condition, error)
}
