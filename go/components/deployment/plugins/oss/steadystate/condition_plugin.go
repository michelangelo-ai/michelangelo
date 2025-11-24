package steadystate

import (
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.Plugin[*v2pb.Deployment] = &conditionPlugin{}

type conditionPlugin struct {
	actors []conditionInterfaces.ConditionActor[*v2pb.Deployment]
}

// Params contains dependencies for steadystate plugin
type Params struct {
	Client  client.Client
	Gateway gateways.Gateway
	Logger  *zap.Logger
}

// NewSteadyStatePlugin creates a new steady state plugin following Uber patterns
func NewSteadyStatePlugin(p Params) conditionInterfaces.Plugin[*v2pb.Deployment] {
	return &conditionPlugin{actors: []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
		&SteadyStateActor{
			client:  p.Client,
			gateway: p.Gateway,
			logger:  p.Logger,
		},
	}}
}

// GetActors returns all actors for this plugin
func (p *conditionPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.Deployment] {
	return p.actors
}

// GetConditions gets the conditions for a deployment
func (p *conditionPlugin) GetConditions(resource *v2pb.Deployment) []*apipb.Condition {
	return resource.Status.Conditions
}

// PutCondition puts a condition for a deployment
func (p *conditionPlugin) PutCondition(resource *v2pb.Deployment, condition *apipb.Condition) {
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}
