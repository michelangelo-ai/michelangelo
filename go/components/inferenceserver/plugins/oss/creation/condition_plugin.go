package creation

import (
	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpoints"
	modelconfig "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// CreationPlugin orchestrates the condition actors for inference server creation.
type CreationPlugin struct {
	clientFactory       clientfactory.ClientFactory
	registry            *backends.Registry
	modelConfigProvider modelconfig.ModelConfigProvider
	endpointPublisher   endpoints.EndpointPublisher
	endpointSource      endpoints.EndpointSource
	logger              *zap.Logger
}

// NewCreationPlugin creates a plugin that manages validation, provisioning, health checks, and routing.
func NewCreationPlugin(clientFactory clientfactory.ClientFactory, registry *backends.Registry, modelConfigProvider modelconfig.ModelConfigProvider, endpointPublisher endpoints.EndpointPublisher, endpointSource endpoints.EndpointSource, logger *zap.Logger) conditionInterfaces.Plugin[*v2pb.InferenceServer] {
	return &CreationPlugin{
		clientFactory:       clientFactory,
		registry:            registry,
		modelConfigProvider: modelConfigProvider,
		endpointPublisher:   endpointPublisher,
		endpointSource:      endpointSource,
		logger:              logger,
	}
}

// GetActors returns the ordered list of condition actors for creation workflow.
func (p *CreationPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return []conditionInterfaces.ConditionActor[*v2pb.InferenceServer]{
		NewValidationActor(p.registry, p.logger),
		NewBackendProvisionActor(p.clientFactory, p.registry, p.logger),
		NewEndpointPublishActor(p.endpointPublisher, p.endpointSource, p.logger),
		NewModelConfigProvisionActor(p.clientFactory, p.modelConfigProvider, p.logger),
		NewHealthCheckActor(p.clientFactory, p.registry, p.logger),
	}
}

// GetConditions retrieves the current conditions from the inference server status.
func (p *CreationPlugin) GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition {
	return resource.Status.Conditions
}

// PutCondition updates or adds a condition to the inference server status.
func (p *CreationPlugin) PutCondition(resource *v2pb.InferenceServer, condition *apipb.Condition) {
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
