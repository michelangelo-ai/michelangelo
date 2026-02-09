package rollout

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/rollout/strategies"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/route"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	modelconfig "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.Plugin[*v2pb.Deployment] = &conditionPlugin{}

// conditionPlugin orchestrates rollout actors in sequence: validation, preparation, placement, routing, completion.
type conditionPlugin struct {
	actors []conditionInterfaces.ConditionActor[*v2pb.Deployment]
}

// Params contains dependencies injected for rollout plugin initialization.
type Params struct {
	Client              client.Client
	DynamicClient       dynamic.Interface
	RouteProvider       route.RouteProvider
	Gateway             gateways.Gateway
	ModelConfigProvider modelconfig.ModelConfigProvider
	Logger              *zap.Logger
}

// NewRolloutPlugin creates a rollout workflow plugin with deployment-specific strategy actors.
func NewRolloutPlugin(ctx context.Context, p Params, deployment *v2pb.Deployment) (conditionInterfaces.Plugin[*v2pb.Deployment], error) {
	logger := p.Logger.With(zap.String("deployment", fmt.Sprintf("%s/%s", deployment.GetNamespace(), deployment.GetName())))

	// Pre-placement actors (preparation and validation)
	prePlacementActors := []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
		&ValidationActor{
			logger: logger,
		},
		&AssetPreparationActor{
			gateway: p.Gateway,
			logger:  logger,
		},
		&ResourceAcquisitionActor{
			client:  p.Client,
			gateway: p.Gateway,
			logger:  logger,
		},
	}

	// Placement strategy actors (rolling strategy for OSS)
	placementActors, err := strategies.GetActorsForStrategy(ctx, strategies.Params{
		Client:              p.Client,
		DynamicClient:       p.DynamicClient,
		RouteProvider:       p.RouteProvider,
		Gateway:             p.Gateway,
		ModelConfigProvider: p.ModelConfigProvider,
		Logger:              p.Logger,
	}, deployment)
	if err != nil {
		return nil, err
	}

	// Post-placement actors
	postPlacementActors := []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
		&RolloutCompletionActor{
			gateway: p.Gateway,
			logger:  p.Logger,
		},
	}

	// Combine all actors in sequence
	actors := make([]conditionInterfaces.ConditionActor[*v2pb.Deployment], 0,
		len(prePlacementActors)+len(placementActors)+len(postPlacementActors))
	actors = append(actors, prePlacementActors...)
	actors = append(actors, placementActors...)
	actors = append(actors, postPlacementActors...)

	return &conditionPlugin{
		actors: actors,
	}, nil
}

// GetActors returns the ordered sequence of rollout actors.
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
