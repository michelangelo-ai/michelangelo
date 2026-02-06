package plugins

import (
	"context"

	"github.com/go-logr/logr"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
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

	// GetRolloutPlugin returns the condition plugin for progressive rollout operations.
	GetRolloutPlugin(ctx context.Context, resource *v2pb.Deployment) (conditionInterfaces.Plugin[*v2pb.Deployment], error)

	// GetRollbackPlugin returns the condition plugin for rollback operations.
	GetRollbackPlugin() conditionInterfaces.Plugin[*v2pb.Deployment]

	// GetCleanupPlugin returns the condition plugin for resource cleanup operations.
	GetCleanupPlugin() conditionInterfaces.Plugin[*v2pb.Deployment]

	// GetSteadyStatePlugin returns the condition plugin for steady state monitoring.
	GetSteadyStatePlugin() conditionInterfaces.Plugin[*v2pb.Deployment]

	// ParseStage determines the current deployment stage from the resource state.
	ParseStage(resource *v2pb.Deployment) v2pb.DeploymentStage

	// PopulateDeploymentLogs is used to populate the deployment logs with the necessary error logs when
	// the deployment reaches a terminal state.
	PopulateDeploymentLogs(ctx context.Context, runtimeContext RequestContext, modelDeployment *v2pb.Deployment)

	// PopulateMessage is used to populate the deployment status message with the error message when the
	// deployment is rolled back or fails to roll out.
	PopulateMessage(ctx context.Context, runtimeContext RequestContext, modelDeployment *v2pb.Deployment)
}

// RequestContext contains the context for actor operations.
type RequestContext struct {
	Deployment *v2pb.Deployment
	Logger     logr.Logger
}

// ObservabilityContext is a wrapper for logging and metric collection.
type ObservabilityContext struct {
	Logger logr.Logger
	Scope  interface{}
}
