package llmd

import (
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// LLMDPlugin implements InferenceServerPlugin for LLMD backend
type LLMDPlugin struct {
	gateway inferenceserver.Gateway
}

// NewPlugin creates a new LLMD plugin
func NewPlugin(gateway inferenceserver.Gateway) plugins.InferenceServerPlugin {
	return &LLMDPlugin{
		gateway: gateway,
	}
}

// GetType returns the backend type this plugin handles
func (p *LLMDPlugin) GetType() v2pb.BackendType {
	return v2pb.BACKEND_TYPE_LLM_D
}

// GetCreationPlugin returns the plugin for infrastructure creation
func (p *LLMDPlugin) GetCreationPlugin() plugins.Plugin {
	return &LLMDCreationPlugin{
		gateway: p.gateway,
	}
}

// GetDeletionPlugin returns the plugin for infrastructure cleanup  
func (p *LLMDPlugin) GetDeletionPlugin(resource *v2pb.InferenceServer) plugins.Plugin {
	return &LLMDDeletionPlugin{
		gateway: p.gateway,
	}
}

// LLMDCreationPlugin implements the Plugin interface for creation lifecycle
type LLMDCreationPlugin struct {
	gateway inferenceserver.Gateway
}

func (p *LLMDCreationPlugin) GetActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		NewValidationActor(p.gateway),
		NewResourceCreationActor(p.gateway),
		NewHealthCheckActor(p.gateway),
	}
}

func (p *LLMDCreationPlugin) GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition {
	return resource.Status.Conditions
}

func (p *LLMDCreationPlugin) PutCondition(resource *v2pb.InferenceServer, condition apipb.Condition) {
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

// LLMDDeletionPlugin implements the Plugin interface for deletion lifecycle
type LLMDDeletionPlugin struct {
	gateway inferenceserver.Gateway
}

func (p *LLMDDeletionPlugin) GetActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		NewCleanupActor(p.gateway),
	}
}

func (p *LLMDDeletionPlugin) GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition {
	return resource.Status.Conditions
}

func (p *LLMDDeletionPlugin) PutCondition(resource *v2pb.InferenceServer, condition apipb.Condition) {
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