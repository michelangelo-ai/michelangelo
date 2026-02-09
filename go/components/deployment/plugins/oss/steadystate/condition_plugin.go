package steadystate

import (
	"net/http"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.Plugin[*v2pb.Deployment] = &conditionPlugin{}

// conditionPlugin orchestrates steady state actors for continuous health monitoring.
type conditionPlugin struct {
	actors []conditionInterfaces.ConditionActor[*v2pb.Deployment]
}

// Params contains dependencies injected for steady state plugin initialization.
type Params struct {
	Gateway    gateways.Gateway
	Client     client.Client
	HTTPClient *http.Client
	Logger     *zap.Logger
}

// NewSteadyStatePlugin creates a steady state monitoring workflow plugin.
func NewSteadyStatePlugin(p Params) conditionInterfaces.Plugin[*v2pb.Deployment] {
	return &conditionPlugin{actors: []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
		&SteadyStateActor{
			gateway:    p.Gateway,
			httpClient: p.HTTPClient,
			client:     p.Client,
			logger:     p.Logger,
		},
	}}
}

// GetActors returns the steady state monitoring actors.
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
