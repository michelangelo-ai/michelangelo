package dynamo

import (
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// DynamoPlugin implements InferenceServerPlugin for Dynamo backend
type DynamoPlugin struct {
	gateway inferenceserver.Gateway
}

// NewPlugin creates a new Dynamo plugin
func NewPlugin(gateway inferenceserver.Gateway) plugins.InferenceServerPlugin {
	return &DynamoPlugin{
		gateway: gateway,
	}
}

// GetType returns the backend type this plugin handles
func (p *DynamoPlugin) GetType() v2pb.BackendType {
	return v2pb.BACKEND_TYPE_DYNAMO
}

// GetCreationActors returns the actors needed for infrastructure creation
func (p *DynamoPlugin) GetCreationActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		NewValidationActor(p.gateway),
		NewPlatformDependenciesActor(p.gateway),
		NewResourceCreationActor(p.gateway),
		NewHealthCheckActor(p.gateway),
	}
}

// GetDeletionActors returns the actors needed for infrastructure cleanup
func (p *DynamoPlugin) GetDeletionActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		NewCleanupActor(p.gateway),
	}
}

// GetStatusActors returns the actors for status checking
func (p *DynamoPlugin) GetStatusActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		NewHealthCheckActor(p.gateway),
	}
}