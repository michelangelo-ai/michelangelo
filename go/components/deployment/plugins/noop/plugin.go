package noop

import (
	"context"
	"time"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/base/pluginmanager"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	api "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// NoOpPlugin is a plugin that always succeeds and does nothing
type NoOpPlugin struct{}

// NewNoOpPlugin creates a new no-op plugin
func NewNoOpPlugin() plugins.Plugin {
	return &NoOpPlugin{}
}

// GetState always returns success state
func (p *NoOpPlugin) GetState(ctx context.Context, observability plugins.ObservabilityContext, modelDeployment *v2pb.Deployment) (*v2pb.DeploymentStatus, error) {
	// Return the current status unchanged
	return &modelDeployment.Status, nil
}

// HealthCheckGate always returns healthy
func (p *NoOpPlugin) HealthCheckGate(ctx context.Context, observability plugins.ObservabilityContext, modelDeployment *v2pb.Deployment) (bool, error) {
	return true, nil
}

// GetRolloutPlugin returns a completing conditions plugin
func (p *NoOpPlugin) GetRolloutPlugin(ctx context.Context, resource *v2pb.Deployment) (conditionInterfaces.Plugin[*v2pb.Deployment], error) {
	return &CompletingConditionsPlugin{}, nil
}

// GetRollbackPlugin returns a no-op conditions plugin
func (p *NoOpPlugin) GetRollbackPlugin() conditionInterfaces.Plugin[*v2pb.Deployment] {
	return &NoOpConditionsPlugin{}
}

// GetCleanupPlugin returns a no-op conditions plugin
func (p *NoOpPlugin) GetCleanupPlugin() conditionInterfaces.Plugin[*v2pb.Deployment] {
	return &NoOpConditionsPlugin{}
}

// GetSteadyStatePlugin returns a no-op conditions plugin
func (p *NoOpPlugin) GetSteadyStatePlugin() conditionInterfaces.Plugin[*v2pb.Deployment] {
	return &NoOpConditionsPlugin{}
}

// ParseStage returns the current stage
func (p *NoOpPlugin) ParseStage(resource *v2pb.Deployment) v2pb.DeploymentStage {
	return resource.Status.Stage
}

// PopulateDeploymentLogs does nothing in the no-op implementation
func (p *NoOpPlugin) PopulateDeploymentLogs(ctx context.Context, modelDeployment *v2pb.Deployment) {
	// No-op
}

// PopulateMessage does nothing in the no-op implementation
func (p *NoOpPlugin) PopulateMessage(ctx context.Context, modelDeployment *v2pb.Deployment) {
	// No-op
}

// CompletingConditionsPlugin is a conditions plugin that moves deployments to completion
type CompletingConditionsPlugin struct{}

// GetActors returns a single actor that completes deployments
func (p *CompletingConditionsPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.Deployment] {
	return []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
		&CompletingActor{},
	}
}

// GetConditions returns the conditions from the deployment status
func (p *CompletingConditionsPlugin) GetConditions(resource *v2pb.Deployment) []*api.Condition {
	return resource.Status.Conditions
}

// PutCondition sets a condition in the deployment status
func (p *CompletingConditionsPlugin) PutCondition(resource *v2pb.Deployment, condition *api.Condition) {
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
func (p *NoOpConditionsPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.Deployment] {
	return []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
		&NoOpActor{},
	}
}

// GetConditions returns the conditions from the deployment status
func (p *NoOpConditionsPlugin) GetConditions(resource *v2pb.Deployment) []*api.Condition {
	return resource.Status.Conditions
}

// PutCondition sets a condition in the deployment status
func (p *NoOpConditionsPlugin) PutCondition(resource *v2pb.Deployment, condition *api.Condition) {
	// No-op for the no-op plugin
}

// CompletingActor is an actor that moves deployments through stages to completion
type CompletingActor struct{}

// Run moves the deployment to the next stage or completion
func (a *CompletingActor) Run(ctx context.Context, resource *v2pb.Deployment, previousCondition *api.Condition) (*api.Condition, error) {
	now := time.Now().UnixMilli()

	// For no-op implementation, move through stages quickly and always return success
	switch resource.Status.Stage {
	case v2pb.DEPLOYMENT_STAGE_VALIDATION:
		// Move to next stage and return success
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_PLACEMENT
		resource.Status.Message = "Validation completed (no-op)"
		return &api.Condition{
			Type:                 "DeploymentProgressing",
			Status:               api.CONDITION_STATUS_TRUE,
			Reason:               "ValidationComplete",
			Message:              "Validation stage completed successfully",
			LastUpdatedTimestamp: now,
		}, nil

	case v2pb.DEPLOYMENT_STAGE_PLACEMENT:
		// Move to next stage and return success
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_RESOURCE_ACQUISITION
		resource.Status.Message = "Placement completed (no-op)"
		// Set candidate revision from desired revision
		resource.Status.CandidateRevision = resource.Spec.DesiredRevision
		return &api.Condition{
			Type:                 "DeploymentProgressing",
			Status:               api.CONDITION_STATUS_TRUE,
			Reason:               "PlacementComplete",
			Message:              "Placement stage completed successfully",
			LastUpdatedTimestamp: now,
		}, nil

	case v2pb.DEPLOYMENT_STAGE_RESOURCE_ACQUISITION:
		// Move to final stage and return success
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
		resource.Status.Message = "Deployment completed successfully (no-op)"
		resource.Status.CurrentRevision = resource.Status.CandidateRevision
		return &api.Condition{
			Type:                 "DeploymentProgressing",
			Status:               api.CONDITION_STATUS_TRUE,
			Reason:               "RolloutComplete",
			Message:              "Deployment completed successfully",
			LastUpdatedTimestamp: now,
		}, nil

	case v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE:
		// Already complete, just return success
		return &api.Condition{
			Type:                 "DeploymentProgressing",
			Status:               api.CONDITION_STATUS_TRUE,
			Reason:               "AlreadyComplete",
			Message:              "Deployment is already complete",
			LastUpdatedTimestamp: now,
		}, nil
	}

	// For any other stage, just return success to avoid retries
	return &api.Condition{
		Type:                 "DeploymentProgressing",
		Status:               api.CONDITION_STATUS_TRUE,
		Reason:               "NoOpComplete",
		Message:              "No-op processing completed for stage: " + resource.Status.Stage.String(),
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
func (a *NoOpActor) Run(ctx context.Context, resource *v2pb.Deployment, previousCondition *api.Condition) (*api.Condition, error) {
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

// RegisterNoOpPlugins registers the no-op plugin for all target types and common subtypes
func RegisterNoOpPlugins(registrar pluginmanager.Registrar[plugins.Plugin]) error {
	noOpPlugin := NewNoOpPlugin()

	// Register for inference server with empty subtype
	if err := registrar.RegisterPlugin(v2pb.TARGET_TYPE_INFERENCE_SERVER.String(), "", noOpPlugin); err != nil {
		return err
	}

	// Register for inference server with common subtypes
	if err := registrar.RegisterPlugin(v2pb.TARGET_TYPE_INFERENCE_SERVER.String(), "realtime-serving", noOpPlugin); err != nil {
		return err
	}

	if err := registrar.RegisterPlugin(v2pb.TARGET_TYPE_INFERENCE_SERVER.String(), "batch-serving", noOpPlugin); err != nil {
		return err
	}

	// Register for offline deployments
	if err := registrar.RegisterPlugin(v2pb.TARGET_TYPE_OFFLINE.String(), "", noOpPlugin); err != nil {
		return err
	}

	// Register for mobile deployments
	if err := registrar.RegisterPlugin(v2pb.TARGET_TYPE_MOBILE.String(), "", noOpPlugin); err != nil {
		return err
	}

	return nil
}
