package strategies

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Params contains dependencies for strategy actors
type Params struct {
	Client  client.Client
	Gateway gateways.Gateway
	Logger  logr.Logger
}

// GetActorsForStrategy returns actors for the appropriate strategy
func GetActorsForStrategy(ctx context.Context, params Params, deployment *v2pb.Deployment) ([]plugins.ConditionActor, error) {
	// Determine strategy from deployment spec or default to rolling
	strategy := getDeploymentStrategy(deployment)
	
	params.Logger.Info("Selected rollout strategy", "strategy", strategy, "deployment", deployment.Name)
	
	switch strategy {
	case "blast":
		return GetBlastActors(params, deployment), nil
	case "zonal":
		return GetZonalActors(params, deployment), nil
	case "shadow":
		return GetShadowActors(params, deployment), nil
	case "disaggregated":
		return GetDisaggregatedActors(params, deployment), nil
	case "rolling":
		fallthrough
	default:
		return GetRollingActors(params, deployment), nil
	}
}

// getDeploymentStrategy determines the rollout strategy from deployment configuration
func getDeploymentStrategy(deployment *v2pb.Deployment) string {
	// Check annotations for strategy override
	if deployment.Annotations != nil {
		if strategy, ok := deployment.Annotations["rollout.strategy"]; ok {
			return strategy
		}
	}
	
	// Check labels for strategy
	if deployment.Labels != nil {
		if strategy, ok := deployment.Labels["rollout.strategy"]; ok {
			return strategy
		}
	}
	
	// Default to rolling strategy
	return "rolling"
}

// GetRollingActors returns actors for rolling rollout strategy
func GetRollingActors(params Params, deployment *v2pb.Deployment) []plugins.ConditionActor {
	return []plugins.ConditionActor{
		&ModelSyncActor{
			client:  params.Client,
			gateway: params.Gateway,
			logger:  params.Logger,
		},
		&RollingRolloutActor{
			client:  params.Client,
			gateway: params.Gateway,
			logger:  params.Logger,
		},
	}
}