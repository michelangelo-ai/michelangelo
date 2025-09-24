package conditionInterfaces

import (
	"context"

	api "github.com/michelangelo-ai/michelangelo/proto/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Engine refers to the implementation that executes the conditional checks via the Retrieve method and runs the actions
// defined in the Run method of a ConditionActor.
type Engine[T client.Object] interface {
	// Run runs a plugin against a particular resource. The type of the resource is guaranteed by any implementation
	// to match the return type.
	Run(ctx context.Context, plugin Plugin[T], resource T) (Result, error)
}

type ConditionActor[T client.Object] interface {
	// Run runs the action that will attempt to move the condition status in the positive direction.
	// If there is a failure to perform any action, the plugin must set the appropriate properties in the returned
	// condition. Any errors that are returned are used only for logging purposes.
	Run(ctx context.Context, resource T, previousCondition *api.Condition) (*api.Condition, error)

	// GetType gets the type of the ConditionActor. This is used to determine api.Condition accountability.
	GetType() string
}

// Plugin refers to a component that produces a list of actors, retrieves all current conditions for a given resource,
// and allows the placement of a condition within a resource.
type Plugin[T client.Object] interface {
	// GetActors gets the list of ConditionActors for a particular plugin. The Engine will sequentially run through the
	// list of actors from this method.
	GetActors() []ConditionActor[T]

	// GetConditions get the conditions for a particular Kubernetes custom resource.
	GetConditions(resource T) []*api.Condition

	// PutCondition puts a condition for a particular Kubernetes custom resource.
	PutCondition(resource T, condition *api.Condition)
}

// Result is the struct that's returned from an engine run.
type Result struct {
	ctrl.Result

	// AreSatisfied is true if all the conditions for a particular plugin execution are satisfied.
	AreSatisfied bool

	// IsTerminal is returned if the maximum number of configured retries are exhausted.
	IsTerminal bool

	// IsKilled is returned if execute workflow process has been killed
	IsKilled bool
}
