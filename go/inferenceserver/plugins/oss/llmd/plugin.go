package llmd

import (
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
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

// GetCreationActors returns the actors needed for infrastructure creation
func (p *LLMDPlugin) GetCreationActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		NewValidationActor(p.gateway),
		NewResourceCreationActor(p.gateway),
		NewHealthCheckActor(p.gateway),
	}
}

// GetDeletionActors returns the actors needed for infrastructure cleanup
func (p *LLMDPlugin) GetDeletionActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		NewCleanupActor(p.gateway),
	}
}

// GetStatusActors returns the actors for status checking
func (p *LLMDPlugin) GetStatusActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		NewHealthCheckActor(p.gateway),
	}
}