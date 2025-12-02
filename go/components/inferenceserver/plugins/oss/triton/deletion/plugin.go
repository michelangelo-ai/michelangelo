package deletion

import (
	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// TritonDeletionPlugin implements the Plugin interface for deletion lifecycle
type TritonDeletionPlugin struct {
	gateway                gateways.Gateway
	proxyProvider          proxy.ProxyProvider
	modelConfigMapProvider configmap.ModelConfigMapProvider
	logger                 *zap.Logger
}

func NewTritonDeletionPlugin(gateway gateways.Gateway, proxyProvider proxy.ProxyProvider, modelConfigMapProvider configmap.ModelConfigMapProvider, logger *zap.Logger) conditionInterfaces.Plugin[*v2pb.InferenceServer] {
	return &TritonDeletionPlugin{
		gateway:                gateway,
		proxyProvider:          proxyProvider,
		modelConfigMapProvider: modelConfigMapProvider,
		logger:                 logger,
	}
}

func (p *TritonDeletionPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return []conditionInterfaces.ConditionActor[*v2pb.InferenceServer]{
		NewCleanupActor(p.gateway, p.modelConfigMapProvider, p.proxyProvider, p.logger),
	}
}

func (p *TritonDeletionPlugin) GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition {
	return resource.Status.Conditions
}

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
