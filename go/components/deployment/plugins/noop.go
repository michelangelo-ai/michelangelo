package plugins

import (
	"context"
	"time"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/types"
	api "github.com/michelangelo-ai/michelangelo/proto/api"
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
func (p *NoOpPlugin) GetRolloutPlugin(ctx context.Context, resource *types.Deployment) (conditionInterfaces.Plugin[*types.Deployment], error) {
	return &CompletingConditionsPlugin{}, nil
}

// GetRollbackPlugin returns a no-op conditions plugin
func (p *NoOpPlugin) GetRollbackPlugin() conditionInterfaces.Plugin[*types.Deployment] {
	return &NoOpConditionsPlugin{}
}

// GetCleanupPlugin returns a no-op conditions plugin
func (p *NoOpPlugin) GetCleanupPlugin() conditionInterfaces.Plugin[*types.Deployment] {
	return &NoOpConditionsPlugin{}
}

// GetSteadyStatePlugin returns a no-op conditions plugin
func (p *NoOpPlugin) GetSteadyStatePlugin() conditionInterfaces.Plugin[*types.Deployment] {
	return &NoOpConditionsPlugin{}
}

// ParseStage returns the current stage
func (p *NoOpPlugin) ParseStage(resource *types.Deployment) types.DeploymentStage {
	return resource.Status.Stage
}

// PopulateDeploymentLogs does nothing in the no-op implementation
func (p *NoOpPlugin) PopulateDeploymentLogs(ctx context.Context, modelDeployment *types.Deployment) {
	// No-op
}

// PopulateMessage does nothing in the no-op implementation
func (p *NoOpPlugin) PopulateMessage(ctx context.Context, modelDeployment *types.Deployment) {
	// No-op
}

// CompletingConditionsPlugin is a conditions plugin that moves deployments to completion
type CompletingConditionsPlugin struct{}

// GetActors returns a single actor that completes deployments
func (p *CompletingConditionsPlugin) GetActors() []conditionInterfaces.ConditionActor[*types.Deployment] {
	return []conditionInterfaces.ConditionActor[*types.Deployment]{
		&CompletingActor{},
	}
}

// GetConditions returns the conditions from the deployment status
func (p *CompletingConditionsPlugin) GetConditions(resource *types.Deployment) []*api.Condition {
	return resource.Status.Conditions
}

// PutCondition sets a condition in the deployment status
func (p *CompletingConditionsPlugin) PutCondition(resource *types.Deployment, condition *api.Condition) {
	// Update or add the condition
	for i, existing := range resource.Status.Conditions {
		if existing.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	// Add new condition if not found
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}

// NoOpConditionsPlugin is a conditions plugin that does nothing
type NoOpConditionsPlugin struct{}

// GetActors returns a single no-op actor
func (p *NoOpConditionsPlugin) GetActors() []conditionInterfaces.ConditionActor[*types.Deployment] {
	return []conditionInterfaces.ConditionActor[*types.Deployment]{
		&NoOpActor{},
	}
}

// GetConditions returns the conditions from the deployment status
func (p *NoOpConditionsPlugin) GetConditions(resource *types.Deployment) []*api.Condition {
	return resource.Status.Conditions
}

// PutCondition sets a condition in the deployment status
func (p *NoOpConditionsPlugin) PutCondition(resource *types.Deployment, condition *api.Condition) {
	// No-op for the no-op plugin
}

// CompletingActor is an actor that moves deployments through stages to completion
type CompletingActor struct{}

// Run moves the deployment to the next stage or completion
func (a *CompletingActor) Run(ctx context.Context, resource *types.Deployment, previousCondition *api.Condition) (*api.Condition, error) {
	now := time.Now().UnixMilli()

	// Move through the stages to completion
	switch resource.Status.Stage {
	case types.DEPLOYMENT_STAGE_VALIDATION:
		resource.Status.Stage = types.DEPLOYMENT_STAGE_PLACEMENT
		resource.Status.Message = "Validation completed"
		return &api.Condition{
			Type:                 "DeploymentProgressing",
			Status:               api.CONDITION_STATUS_UNKNOWN, // Continue processing
			Reason:               "ValidationComplete",
			Message:              "Validation stage completed successfully",
			LastUpdatedTimestamp: now,
		}, nil
	case types.DEPLOYMENT_STAGE_PLACEMENT:
		resource.Status.Stage = types.DEPLOYMENT_STAGE_RESOURCE_ACQUISITION
		resource.Status.Message = "Placement completed"
		return &api.Condition{
			Type:                 "DeploymentProgressing",
			Status:               api.CONDITION_STATUS_UNKNOWN, // Continue processing
			Reason:               "PlacementComplete",
			Message:              "Placement stage completed successfully",
			LastUpdatedTimestamp: now,
		}, nil
	case types.DEPLOYMENT_STAGE_RESOURCE_ACQUISITION:
		resource.Status.Stage = types.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		resource.Status.Message = "Deployment completed successfully (simplified version)"
		resource.Status.CurrentRevision = resource.Status.CandidateRevision
		return &api.Condition{
			Type:                 "DeploymentProgressing",
			Status:               api.CONDITION_STATUS_TRUE, // Terminal - success
			Reason:               "RolloutComplete",
			Message:              "Deployment completed successfully",
			LastUpdatedTimestamp: now,
		}, nil
	}

	// Default: mark as progressing but unknown
	return &api.Condition{
		Type:                 "DeploymentProgressing",
		Status:               api.CONDITION_STATUS_UNKNOWN,
		Reason:               "Processing",
		Message:              "Deployment is processing",
		LastUpdatedTimestamp: now,
	}, nil
}

// GetType returns the type of this actor
func (a *CompletingActor) GetType() string {
	return "DeploymentProgressing"
}

// NoOpActor is an actor that does nothing and marks as successful
type NoOpActor struct{}

// Run always returns a successful condition
func (a *NoOpActor) Run(ctx context.Context, resource *types.Deployment, previousCondition *api.Condition) (*api.Condition, error) {
	now := time.Now().UnixMilli()
	return &api.Condition{
		Type:                 "NoOp",
		Status:               api.CONDITION_STATUS_TRUE,
		Reason:               "NoOpComplete",
		Message:              "No-op operation completed successfully",
		LastUpdatedTimestamp: now,
	}, nil
}

// GetType returns the type of this actor
func (a *NoOpActor) GetType() string {
	return "NoOp"
}
