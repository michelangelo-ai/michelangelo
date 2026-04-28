package strategies

import (
	"context"
	"net/http"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/route"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Params contains dependencies for strategy actors
type Params struct {
	Client              client.Client
	HTTPClient          *http.Client
	RouteProvider       route.RouteProvider
	Logger              *zap.Logger
	Registry            *backends.Registry
	ClientFactory       clientfactory.ClientFactory
	DefaultClient       client.Client
	ModelConfigProvider modelconfig.ModelConfigProvider
}

// GetActorsForStrategy returns actors for the appropriate strategy
func GetActorsForStrategy(ctx context.Context, params Params, deployment *v2pb.Deployment) ([]conditionInterfaces.ConditionActor[*v2pb.Deployment], error) {
	// Determine strategy from deployment spec or default to rolling
	strategy := getDeploymentStrategy(deployment)

	params.Logger.Info("Selected rollout strategy", zap.String("strategy", strategy), zap.String("deployment", deployment.Name))

	switch strategy {
	// TODO(#623): ghosharitra: Implement blast, zonal, shadow, and disaggregated strategies
	case "rolling":
		fallthrough
	default:
		return getRollingActors(params, deployment), nil
	}
}

// getDeploymentStrategy determines the rollout strategy from deployment configuration
func getDeploymentStrategy(deployment *v2pb.Deployment) string {
	switch deployment.Spec.GetStrategy().GetRolloutStrategy().(type) {
	case *v2pb.DeploymentStrategy_Rolling:
		return "rolling"
	default:
		return "rolling"
	}
}
