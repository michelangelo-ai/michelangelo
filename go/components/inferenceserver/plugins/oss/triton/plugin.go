package triton

import (
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// TritonPlugin implements InferenceServerPlugin for Triton backend
type TritonPlugin struct {
	gateway gateways.Gateway
}

// NewPlugin creates a new Triton plugin
func NewPlugin(gateway gateways.Gateway) plugins.InferenceServerPlugin {
	return &TritonPlugin{
		gateway: gateway,
	}
}

// GetType returns the backend type this plugin handles
func (p *TritonPlugin) GetType() v2pb.BackendType {
	return v2pb.BACKEND_TYPE_TRITON
}

// GetCreationPlugin returns the plugin for infrastructure creation
func (p *TritonPlugin) GetCreationPlugin() plugins.Plugin {
	return &TritonCreationPlugin{
		gateway: p.gateway,
	}
}

// GetDeletionPlugin returns the plugin for infrastructure cleanup
func (p *TritonPlugin) GetDeletionPlugin(resource *v2pb.InferenceServer) plugins.Plugin {
	return &TritonDeletionPlugin{
		gateway: p.gateway,
	}
}

// TritonCreationPlugin implements the Plugin interface for creation lifecycle
type TritonCreationPlugin struct {
	gateway gateways.Gateway
}

func (p *TritonCreationPlugin) GetActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		NewValidationActor(p.gateway),
		NewResourceCreationActor(p.gateway),
		NewHealthCheckActor(p.gateway),
		NewProxyConfigurationActor(p.gateway),
	}
}

func (p *TritonCreationPlugin) GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition {
	return resource.Status.Conditions
}

func (p *TritonCreationPlugin) PutCondition(resource *v2pb.InferenceServer, condition apipb.Condition) {
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

// TritonDeletionPlugin implements the Plugin interface for deletion lifecycle
type TritonDeletionPlugin struct {
	gateway gateways.Gateway
}

func (p *TritonDeletionPlugin) GetActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		NewCleanupActor(p.gateway),
	}
}

func (p *TritonDeletionPlugin) GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition {
	return resource.Status.Conditions
}

func (p *TritonDeletionPlugin) PutCondition(resource *v2pb.InferenceServer, condition apipb.Condition) {
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
