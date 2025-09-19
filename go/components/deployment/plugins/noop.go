package plugins

import (
	"context"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/types"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/utils/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
)

// NoOpPlugin is a plugin that always succeeds and does nothing
type NoOpPlugin struct{}

// NewNoOpPlugin creates a new no-op plugin
func NewNoOpPlugin() Plugin {
	return &NoOpPlugin{}
}

// GetState always returns success state
func (p *NoOpPlugin) GetState(ctx context.Context, observability ObservabilityContext, modelDeployment *types.Deployment) (types.DeploymentStatus, error) {
	// Return the current status unchanged
	return modelDeployment.Status, nil
}

// HealthCheckGate always returns healthy
func (p *NoOpPlugin) HealthCheckGate(ctx context.Context, observability ObservabilityContext, modelDeployment *types.Deployment) (bool, error) {
	return true, nil
}

// GetRolloutPlugin returns a completing conditions plugin
func (p *NoOpPlugin) GetRolloutPlugin(ctx context.Context, resource *types.Deployment) (conditions.Plugin[*types.Deployment], error) {
	return &CompletingPlugin{}, nil
}

// GetRollbackPlugin returns a no-op conditions plugin
func (p *NoOpPlugin) GetRollbackPlugin() conditions.Plugin[*types.Deployment] {
	return conditions.NewNoOpPlugin[*types.Deployment]()
}

// GetCleanupPlugin returns a no-op conditions plugin
func (p *NoOpPlugin) GetCleanupPlugin() conditions.Plugin[*types.Deployment] {
	return conditions.NewNoOpPlugin[*types.Deployment]()
}

// GetSteadyStatePlugin returns a no-op conditions plugin
func (p *NoOpPlugin) GetSteadyStatePlugin() conditions.Plugin[*types.Deployment] {
	return conditions.NewNoOpPlugin[*types.Deployment]()
}

// ParseStage returns the current stage
func (p *NoOpPlugin) ParseStage(resource *types.Deployment) types.DeploymentStage {
	return resource.Status.Stage
}

// PopulateDeploymentLogs does nothing in the no-op implementation
func (p *NoOpPlugin) PopulateDeploymentLogs(ctx context.Context, runtimeContext conditions.RequestContext, modelDeployment *types.Deployment) {
	// No-op
}

// PopulateMessage does nothing in the no-op implementation
func (p *NoOpPlugin) PopulateMessage(ctx context.Context, runtimeContext conditions.RequestContext, modelDeployment *types.Deployment) {
	// No-op
}

// CompletingPlugin is a plugin that moves deployments to completion
type CompletingPlugin struct{}

// Execute moves the deployment to completion
func (p *CompletingPlugin) Execute(ctx context.Context, runtimeContext conditions.RequestContext, resource *types.Deployment) (conditions.Result, error) {
	// Move through the stages to completion
	switch resource.Status.Stage {
	case types.DEPLOYMENT_STAGE_VALIDATION:
		resource.Status.Stage = types.DEPLOYMENT_STAGE_PLACEMENT
		resource.Status.Message = "Validation completed"
	case types.DEPLOYMENT_STAGE_PLACEMENT:
		resource.Status.Stage = types.DEPLOYMENT_STAGE_RESOURCE_ACQUISITION
		resource.Status.Message = "Placement completed"
	case types.DEPLOYMENT_STAGE_RESOURCE_ACQUISITION:
		resource.Status.Stage = types.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		resource.Status.Message = "Deployment completed successfully (simplified version)"
		return conditions.Result{
			Result:     ctrl.Result{},
			IsTerminal: true,
		}, nil
	}

	// Continue processing
	return conditions.Result{
		Result:     ctrl.Result{Requeue: true},
		IsTerminal: false,
	}, nil
}