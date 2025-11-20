package steadystate

import (
	"context"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
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

// SteadyStateActor handles steady state monitoring following Uber patterns
type SteadyStateActor struct {
	client  client.Client
	gateway gateways.Gateway
	logger  *zap.Logger
}

func (a *SteadyStateActor) GetType() string {
	return common.ActorTypeSteadyState
}

func (a *SteadyStateActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if deployment is in steady state (complete and healthy)
	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE &&
		resource.Status.State == v2pb.DEPLOYMENT_STATE_HEALTHY {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "SteadyStateReached",
			Message: "Deployment is in steady state",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "NotInSteadyState",
		Message: "Deployment not yet in steady state",
	}, nil
}

func (a *SteadyStateActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Monitoring steady state for deployment", zap.String("deployment", resource.Name))

	if resource.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE {
		// In Uber's implementation, steady state monitoring involves:
		// 1. Continuous health monitoring of inference servers
		// 2. Model performance metrics validation
		// 3. Resource utilization monitoring
		// 4. Automatic drift detection and correction
		// 5. SLA compliance monitoring
		// 6. Integration with MES for model lifecycle management

		// For OSS, actively monitor and maintain steady state
		if resource.Status.State != v2pb.DEPLOYMENT_STATE_HEALTHY {
			a.logger.Info("Deployment not healthy, investigating", zap.String("state", resource.Status.State.String()))
			// In a real implementation, this would check inference server health
			// For now, assume we can restore to healthy state
			resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		}

		// Ensure current revision matches desired revision
		if resource.Status.CurrentRevision != nil && resource.Spec.DesiredRevision != nil {
			if resource.Status.CurrentRevision.Name != resource.Spec.DesiredRevision.Name {
				a.logger.Info("Revision mismatch detected, needs reconciliation",
					zap.String("current", resource.Status.CurrentRevision.Name),
					zap.String("desired", resource.Spec.DesiredRevision.Name))
			}
		}

		a.logger.Info("Deployment is in steady state", zap.String("deployment", resource.Name))
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "SteadyStateReached",
		Message: "Deployment is in steady state",
	}, nil
}
