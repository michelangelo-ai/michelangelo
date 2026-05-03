package cleanup

import (
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/discovery"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/route"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.Plugin[*v2pb.Deployment] = &conditionPlugin{}

// conditionPlugin orchestrates cleanup actors to remove deployment resources.
type conditionPlugin struct {
	actors []conditionInterfaces.ConditionActor[*v2pb.Deployment]
}

// Params contains dependencies injected for cleanup plugin initialization.
type Params struct {
	Client                 client.Client
	ClientFactory          clientfactory.ClientFactory
	RouteProvider          route.RouteProvider
	ModelDiscoveryProvider discovery.ModelDiscoveryProvider
	ModelConfigProvider    modelconfig.ModelConfigProvider
	Logger                 *zap.Logger
}

// NewCleanupPlugin creates a cleanup workflow plugin.
func NewCleanupPlugin(p Params) conditionInterfaces.Plugin[*v2pb.Deployment] {
	return &conditionPlugin{actors: []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
		&CleanupActor{
			Client:                 p.Client,
			ClientFactory:          p.ClientFactory,
			RouteProvider:          p.RouteProvider,
			ModelDiscoveryProvider: p.ModelDiscoveryProvider,
			ModelConfigProvider:    p.ModelConfigProvider,
			Logger:                 p.Logger,
		},
	}}
}

// GetActors returns the cleanup actors.
func (p *conditionPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.Deployment] {
	return p.actors
}

// GetConditions retrieves the current conditions from the deployment status.
func (p *conditionPlugin) GetConditions(resource *v2pb.Deployment) []*apipb.Condition {
	return resource.Status.Conditions
}

// PutCondition updates or adds a condition to the deployment status.
func (p *conditionPlugin) PutCondition(resource *v2pb.Deployment, condition *apipb.Condition) {
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}
