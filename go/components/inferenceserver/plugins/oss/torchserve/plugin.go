package torchserve

import (
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// TorchServePlugin implements InferenceServerPlugin for TorchServe backend
type TorchServePlugin struct {
	gateway gateways.Gateway
}

// NewPlugin creates a new TorchServe plugin
func NewPlugin(gateway gateways.Gateway) plugins.InferenceServerPlugin {
	return &TorchServePlugin{
		gateway: gateway,
	}
}

// GetType returns the backend type this plugin handles
func (p *TorchServePlugin) GetType() v2pb.BackendType {
	return v2pb.BACKEND_TYPE_TORCHSERVE
}

// GetCreationPlugin returns the plugin for infrastructure creation
func (p *TorchServePlugin) GetCreationPlugin() plugins.Plugin {
	return &TorchServeCreationPlugin{
		gateway: p.gateway,
	}
}

// GetDeletionPlugin returns the plugin for infrastructure deletion
func (p *TorchServePlugin) GetDeletionPlugin(resource *v2pb.InferenceServer) plugins.Plugin {
	return &TorchServeDeletionPlugin{
		gateway: p.gateway,
	}
}

// TorchServeCreationPlugin handles TorchServe infrastructure creation
type TorchServeCreationPlugin struct {
	gateway gateways.Gateway
}

// GetActors returns the list of actors for TorchServe creation
func (p *TorchServeCreationPlugin) GetActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		&TorchServeValidationActor{
			gateway: p.gateway,
		},
		&TorchServeResourceCreationActor{
			gateway: p.gateway,
		},
		&TorchServeHealthCheckActor{
			gateway: p.gateway,
		},
	}
}

// TorchServeDeletionPlugin handles TorchServe infrastructure deletion
type TorchServeDeletionPlugin struct {
	gateway gateways.Gateway
}

// GetActors returns the list of actors for TorchServe deletion
func (p *TorchServeDeletionPlugin) GetActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		&TorchServeCleanupActor{
			gateway: p.gateway,
		},
	}
}

// GetConditions get the conditions for a particular Kubernetes custom resource.
func (p *TorchServeCreationPlugin) GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition {
	// Return conditions specific to TorchServe creation process
	// This would typically query the actual conditions from the resource status
	return []*apipb.Condition{}
}

// PutCondition puts a condition for a particular Kubernetes custom resource.
func (p *TorchServeCreationPlugin) PutCondition(resource *v2pb.InferenceServer, condition apipb.Condition) {
	// Update the resource with the given condition
	// This would typically update the resource status conditions
}

// GetConditions get the conditions for a particular Kubernetes custom resource.
func (p *TorchServeDeletionPlugin) GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition {
	// Return conditions specific to TorchServe deletion process
	return []*apipb.Condition{}
}

// PutCondition puts a condition for a particular Kubernetes custom resource.
func (p *TorchServeDeletionPlugin) PutCondition(resource *v2pb.InferenceServer, condition apipb.Condition) {
	// Update the resource with the given condition for deletion
}
