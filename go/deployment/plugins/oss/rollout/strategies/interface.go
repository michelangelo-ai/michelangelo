package strategies

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Params contains dependencies for strategy actors
type Params struct {
	Client  client.Client
	Gateway inferenceserver.Gateway
	Logger  logr.Logger
}

// GetActorsForStrategy returns actors for the appropriate strategy
func GetActorsForStrategy(ctx context.Context, params Params, deployment *v2pb.Deployment) ([]plugins.ConditionActor, error) {
	// For OSS, we default to rolling strategy
	// In Uber's implementation, this would check deployment.Spec.Strategy to determine
	// which strategy to use (rolling, zonal, blast, shadow, etc.)
	
	return GetRollingActors(params, deployment), nil
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