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

// TritonDeletionPlugin orchestrates the condition actors for inference server deletion.
type TritonDeletionPlugin struct {
	backend       backends.Backend
	client        client.Client
	clientFactory clientfactory.ClientFactory
	logger        *zap.Logger
}

// NewTritonDeletionPlugin creates a plugin that manages cleanup of all inference server resources.
func NewTritonDeletionPlugin(backend backends.Backend, client client.Client, clientFactory clientfactory.ClientFactory, logger *zap.Logger) conditionInterfaces.Plugin[*v2pb.InferenceServer] {
	return &TritonDeletionPlugin{
		backend:       backend,
		client:        client,
		clientFactory: clientFactory,
		logger:        logger,
	}
}

// GetActors returns the condition actors for deletion workflow.
func (p *TritonDeletionPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return []conditionInterfaces.ConditionActor[*v2pb.InferenceServer]{
		NewCleanupActor(p.logger, p.client, p.clientFactory, p.backend),
	}
}

// GetConditions retrieves the current conditions from the inference server status.
func (p *TritonDeletionPlugin) GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition {
	return resource.Status.Conditions
}

// PutCondition updates or adds a condition to the inference server status.
func (p *TritonDeletionPlugin) PutCondition(resource *v2pb.InferenceServer, condition *apipb.Condition) {
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
