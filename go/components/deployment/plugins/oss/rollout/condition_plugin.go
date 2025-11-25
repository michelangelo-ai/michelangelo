package rollout

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/rollout/strategies"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/proxy"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.Plugin[*v2pb.Deployment] = &conditionPlugin{}

type conditionPlugin struct {
	actors []conditionInterfaces.ConditionActor[*v2pb.Deployment]
}

// Params contains dependencies for rollout plugin
type Params struct {
	Client                 client.Client
	ProxyProvider          proxy.ProxyProvider
	ModelConfigMapProvider configmap.ModelConfigMapProvider
	Gateway                gateways.Gateway
	Logger                 *zap.Logger
}

// NewRolloutPlugin creates a new rollout plugin following Uber patterns
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
			gateway: p.Gateway,
			logger:  logger,
		},
	}

	// Placement strategy actors (rolling strategy for OSS)
	placementActors, err := strategies.GetActorsForStrategy(ctx, strategies.Params{
		Client:                 p.Client,
		ProxyProvider:          p.ProxyProvider,
		ModelConfigMapProvider: p.ModelConfigMapProvider,
		Gateway:                p.Gateway,
		Logger:                 p.Logger,
	}, deployment)
	if err != nil {
		return nil, err
	}

	// Post-placement actors (completion and cleanup)
	postPlacementActors := []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
		&TrafficRoutingActor{
			ProxyProvider: p.ProxyProvider,
			Logger:        p.Logger,
		},
		&RolloutCompletionActor{
			gateway:                p.Gateway,
			modelConfigMapProvider: p.ModelConfigMapProvider,
			logger:                 p.Logger,
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
