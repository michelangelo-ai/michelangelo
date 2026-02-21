package creation

import (
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpointregistry"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// CreationPlugin orchestrates the condition actors for inference server creation.
type CreationPlugin struct {
	client           client.Client
	clientFactory    clientfactory.ClientFactory
	endpointRegistry endpointregistry.EndpointRegistry
	registry         *backends.Registry
	logger           *zap.Logger
}

// NewCreationPlugin creates a plugin that manages validation, provisioning, health checks, and routing.
func NewCreationPlugin(client client.Client, clientFactory clientfactory.ClientFactory, registry *backends.Registry, endpointRegistry endpointregistry.EndpointRegistry, logger *zap.Logger) conditionInterfaces.Plugin[*v2pb.InferenceServer] {
	return &CreationPlugin{
		client:           client,
		clientFactory:    clientFactory,
		registry:         registry,
		endpointRegistry: endpointRegistry,
		logger:           logger,
	}
}

// GetActors returns the ordered list of condition actors for creation workflow.
func (p *CreationPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return []conditionInterfaces.ConditionActor[*v2pb.InferenceServer]{
		NewValidationActor(p.logger),
		NewBackendProvisionActor(p.logger, p.client, p.clientFactory, p.registry),
		NewControlPlaneDiscoveryActor(p.endpointRegistry, p.logger),
		NewHealthCheckActor(p.logger, p.client, p.clientFactory, p.registry),
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
