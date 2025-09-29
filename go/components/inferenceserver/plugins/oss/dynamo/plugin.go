package dynamo

import (
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// DynamoPlugin implements InferenceServerPlugin for Dynamo backend
type DynamoPlugin struct {
	gateway gateways.Gateway
}

// NewPlugin creates a new Dynamo plugin
func NewPlugin(gateway gateways.Gateway) plugins.InferenceServerPlugin {
	return &DynamoPlugin{
		gateway: gateway,
	}
}

// GetType returns the backend type this plugin handles
func (p *DynamoPlugin) GetType() v2pb.BackendType {
	return v2pb.BACKEND_TYPE_DYNAMO
}

// GetCreationPlugin returns the plugin for infrastructure creation
func (p *DynamoPlugin) GetCreationPlugin() plugins.Plugin {
	return &DynamoCreationPlugin{
		gateway: p.gateway,
	}
}

// GetDeletionPlugin returns the plugin for infrastructure cleanup
func (p *DynamoPlugin) GetDeletionPlugin(resource *v2pb.InferenceServer) plugins.Plugin {
	return &DynamoDeletionPlugin{
		gateway: p.gateway,
	}
}

// DynamoCreationPlugin implements the Plugin interface for creation lifecycle
type DynamoCreationPlugin struct {
	gateway gateways.Gateway
}

func (p *DynamoCreationPlugin) GetActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		NewValidationActor(p.gateway),
		NewPlatformDependenciesActor(p.gateway),
		NewResourceCreationActor(p.gateway),
		NewHealthCheckActor(p.gateway),
	}
}

func (p *DynamoCreationPlugin) GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition {
	return resource.Status.Conditions
}

func (p *DynamoCreationPlugin) PutCondition(resource *v2pb.InferenceServer, condition apipb.Condition) {
	if resource.Status.Conditions == nil {
		resource.Status.Conditions = []*apipb.Condition{}
	}

	// Find existing condition and update it
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = &condition
			return
		}
	}

	// Add new condition if not found
	resource.Status.Conditions = append(resource.Status.Conditions, &condition)
}

// DynamoDeletionPlugin implements the Plugin interface for deletion lifecycle
type DynamoDeletionPlugin struct {
	gateway gateways.Gateway
}

func (p *DynamoDeletionPlugin) GetActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		NewCleanupActor(p.gateway),
	}
}

func (p *DynamoDeletionPlugin) GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition {
	return resource.Status.Conditions
}

func (p *DynamoDeletionPlugin) PutCondition(resource *v2pb.InferenceServer, condition apipb.Condition) {
	if resource.Status.Conditions == nil {
		resource.Status.Conditions = []*apipb.Condition{}
	}

	// Find existing condition and update it
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = &condition
			return
		}
	}

	// Add new condition if not found
	resource.Status.Conditions = append(resource.Status.Conditions, &condition)
}
