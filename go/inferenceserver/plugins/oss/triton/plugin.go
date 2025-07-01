package triton

import (
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// TritonPlugin implements InferenceServerPlugin for Triton backend
type TritonPlugin struct {
	gateway inferenceserver.Gateway
}

// NewPlugin creates a new Triton plugin
func NewPlugin(gateway inferenceserver.Gateway) plugins.InferenceServerPlugin {
	return &TritonPlugin{
		gateway: gateway,
	}
}

// GetType returns the backend type this plugin handles
func (p *TritonPlugin) GetType() v2pb.BackendType {
	return v2pb.BACKEND_TYPE_TRITON
}

// GetCreationActors returns the actors needed for infrastructure creation
func (p *TritonPlugin) GetCreationActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		NewValidationActor(p.gateway),
		NewResourceCreationActor(p.gateway),
		NewHealthCheckActor(p.gateway),
		NewProxyConfigurationActor(p.gateway),
	}
}

// GetDeletionActors returns the actors needed for infrastructure cleanup
func (p *TritonPlugin) GetDeletionActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		NewCleanupActor(p.gateway),
	}
}

// GetStatusActors returns the actors for status checking
func (p *TritonPlugin) GetStatusActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		NewHealthCheckActor(p.gateway),
	}
}