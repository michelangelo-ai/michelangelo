package creation

import (
	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpointregistry"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// TritonCreationPlugin orchestrates the condition actors for inference server creation.
type TritonCreationPlugin struct {
	backend               backends.Backend
	endpointRegistry      endpointregistry.EndpointRegistry
	controlPlaneClusterId string
	logger                *zap.Logger
}

// NewTritonCreationPlugin creates a plugin that manages validation, provisioning, health checks, and routing.
func NewTritonCreationPlugin(backend backends.Backend, endpointRegistry endpointregistry.EndpointRegistry, controlPlaneClusterId string, logger *zap.Logger) conditionInterfaces.Plugin[*v2pb.InferenceServer] {
	return &TritonCreationPlugin{
		backend:               backend,
		endpointRegistry:      endpointRegistry,
		controlPlaneClusterId: controlPlaneClusterId,
		logger:                logger,
	}
}

// GetActors returns the ordered list of condition actors for creation workflow.
func (p *TritonCreationPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return []conditionInterfaces.ConditionActor[*v2pb.InferenceServer]{
		NewValidationActor(p.backend, p.controlPlaneClusterId, p.logger),
		NewClusterWorkloadsActor(p.backend, p.logger),
		NewControlPlaneDiscoveryActor(p.endpointRegistry, p.logger),
		NewHealthCheckActor(p.backend, p.logger),
	}
}

// GetConditions retrieves the current conditions from the inference server status.
func (p *TritonCreationPlugin) GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition {
	return resource.Status.Conditions
}

// PutCondition updates or adds a condition to the inference server status.
func (p *TritonCreationPlugin) PutCondition(resource *v2pb.InferenceServer, condition *apipb.Condition) {
	if resource.Status.Conditions == nil {
		resource.Status.Conditions = []*apipb.Condition{}
	}

	// Find existing condition and update it
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}

	// Add new condition if not found
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}
