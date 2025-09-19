package plugins

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/types"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/utils/conditions"
)

// Plugin is the interface that all Deployment plugins must implement. The deployment controller will
// use the appropriate plugin based on the string corresponding to the target name.
type Plugin interface {
	// GetState is unique to the other stages in that this will need to be run with every execution of the
	// reconcile loops. The others are dependent on the stage.
	GetState(ctx context.Context, observability ObservabilityContext, modelDeployment *types.Deployment) (types.DeploymentStatus, error)

	// HealthCheckGate is used to check if there are issues with the current model rollout. If the bool returned is false,
	// this indicates a problem with the rollout. Else, the rollout should proceed as usual.
	HealthCheckGate(ctx context.Context, observability ObservabilityContext, modelDeployment *types.Deployment) (bool, error)

	GetRolloutPlugin(ctx context.Context, resource *types.Deployment) (conditions.Plugin[*types.Deployment], error)
	GetRollbackPlugin() conditions.Plugin[*types.Deployment]
	GetCleanupPlugin() conditions.Plugin[*types.Deployment]
	GetSteadyStatePlugin() conditions.Plugin[*types.Deployment]
	ParseStage(resource *types.Deployment) types.DeploymentStage

	// PopulateDeploymentLogs is used to populate the deployment logs with the necessary error logs when
	// the deployment reaches a terminal state.
	PopulateDeploymentLogs(ctx context.Context, runtimeContext conditions.RequestContext, modelDeployment *types.Deployment)

	// PopulateMessage is used to populate the deployment status message with the error message when the
	// deployment is rolled back or fails to roll out.
	PopulateMessage(ctx context.Context, runtimeContext conditions.RequestContext, modelDeployment *types.Deployment)
}

// ObservabilityContext is a wrapper for logging and metric collection containing the
// tags injected from upstream components.
type ObservabilityContext struct {
	Logger logr.Logger
	Scope  interface{} // Simplified scope
}