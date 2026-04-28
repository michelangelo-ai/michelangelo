package deletion

import (
	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// DeletionPlugin orchestrates the condition actors for inference server deletion.
type DeletionPlugin struct {
	client        client.Client
	clientFactory clientfactory.ClientFactory
	backend       backends.Backend
	logger        *zap.Logger
}

// NewDeletionPlugin creates a plugin that manages cleanup of all inference server resources.
func NewDeletionPlugin(client client.Client, clientFactory clientfactory.ClientFactory, registry *backends.Registry, logger *zap.Logger) conditionInterfaces.Plugin[*v2pb.InferenceServer] {
	_ = registry
	return &DeletionPlugin{
		client:        client,
		clientFactory: clientFactory,
		// todo: ghosharitra: interface broke, need to update
		backend: nil,
		logger:  logger,
	}
}

// GetActors returns the condition actors for deletion workflow.
func (p *DeletionPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return []conditionInterfaces.ConditionActor[*v2pb.InferenceServer]{
		NewCleanupActor(p.logger, p.client, p.clientFactory, p.backend),
	}
}

// GetConditions retrieves the current conditions from the inference server status.
func (p *DeletionPlugin) GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition {
	return resource.Status.Conditions
}

// PutCondition updates or adds a condition to the inference server status.
func (p *DeletionPlugin) PutCondition(resource *v2pb.InferenceServer, condition *apipb.Condition) {
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
