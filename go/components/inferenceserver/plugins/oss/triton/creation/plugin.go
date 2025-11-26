package creation

import (
	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// TritonCreationPlugin implements the Plugin interface for creation lifecycle
type TritonCreationPlugin struct {
	gateway       gateways.Gateway
	proxyProvider proxy.ProxyProvider
	logger        *zap.Logger
}

func NewTritonCreationPlugin(gateway gateways.Gateway, proxyProvider proxy.ProxyProvider, logger *zap.Logger) conditionInterfaces.Plugin[*v2pb.InferenceServer] {
	return &TritonCreationPlugin{
		gateway:       gateway,
		proxyProvider: proxyProvider,
		logger:        logger,
	}
}

func (p *TritonCreationPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return []conditionInterfaces.ConditionActor[*v2pb.InferenceServer]{
		NewValidationActor(p.gateway, p.logger, p.proxyProvider),
		NewResourceCreationActor(p.gateway, p.logger),
		NewHealthCheckActor(p.gateway, p.logger),
		NewProxyConfigurationActor(p.gateway, p.proxyProvider, p.logger),
	}
}

func (p *TritonCreationPlugin) GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition {
	return resource.Status.Conditions
}

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
