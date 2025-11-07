package plugins

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Plugin is the interface that all Deployment plugins must implement. The deployment controller will
// use the appropriate plugin based on the string corresponding to the target name.
type Plugin interface {
	// GetState is unique to the other stages in that this will need to be run with every execution of the
	// reconcile loops. The others are dependent on the stage.
	GetState(ctx context.Context, observability ObservabilityContext, modelDeployment *v2pb.Deployment) (v2pb.DeploymentStatus, error)

	// HealthCheckGate is used to check if there are issues with the current model rollout. If the bool returned is false,
	// this indicates a problem with the rollout. Else, the rollout should proceed as usual.
	HealthCheckGate(ctx context.Context, observability ObservabilityContext, modelDeployment *v2pb.Deployment) (bool, error)

	GetRolloutPlugin(ctx context.Context, resource *v2pb.Deployment) (conditionInterfaces.Plugin[*v2pb.Deployment], error)
	GetRollbackPlugin() conditionInterfaces.Plugin[*v2pb.Deployment]
	GetCleanupPlugin() conditionInterfaces.Plugin[*v2pb.Deployment]
	GetSteadyStatePlugin() conditionInterfaces.Plugin[*v2pb.Deployment]
	ParseStage(resource *v2pb.Deployment) v2pb.DeploymentStage

	// PopulateDeploymentLogs is used to populate the deployment logs with the necessary error logs when
	// the deployment reaches a terminal state.
	PopulateDeploymentLogs(ctx context.Context, runtimeContext RequestContext, modelDeployment *v2pb.Deployment)

	// PopulateMessage is used to populate the deployment status message with the error message when the
	// deployment is rolled back or fails to roll out.
	PopulateMessage(ctx context.Context, runtimeContext RequestContext, modelDeployment *v2pb.Deployment)

	// // HandleCleanup handles cleanup when a deployment is being deleted, including ConfigMaps and other resources
	// HandleCleanup(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error
}

// ConditionActor refers to an implementation to collect and act upon a condition
// This must match conditionInterfaces.ConditionActor[*v2pb.Deployment]
type ConditionActor interface {
	// Run runs the action that will attempt to move the condition status in the positive direction
	Run(ctx context.Context, resource *v2pb.Deployment, previousCondition *apipb.Condition) (*apipb.Condition, error)

	// Retrieve retrieves the current state/status of the condition without performing any action.
	// This is used to check if the condition is satisfied before attempting to run the action.
	Retrieve(ctx context.Context, resource *v2pb.Deployment, previousCondition *apipb.Condition) (*apipb.Condition, error)

	// GetType gets the type of the ConditionActor
	GetType() string
}

// RequestContext contains the context for actor operations
type RequestContext struct {
	Deployment *v2pb.Deployment
	Logger     logr.Logger
}

// PluginResult is an alias to reconcile.Result
type PluginResult struct {
	NextStage v2pb.DeploymentStage
	Result    reconcile.Result
}

// ObservabilityContext is a wrapper for logging and metric collection
type ObservabilityContext struct {
	Logger logr.Logger
	Scope  interface{} // tally.Scope but avoiding import cycle
}

// Result is the struct that's returned from an engine run
type Result struct {
	reconcile.Result

	// AreSatisfied is true if all the conditions for a particular plugin execution are satisfied
	AreSatisfied bool

	// IsTerminal is returned if the maximum number of configured retries are exhausted
	IsTerminal bool
}
