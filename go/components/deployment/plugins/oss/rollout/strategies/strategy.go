package strategies

import (
	"context"
	"fmt"
	"math"
	"net/http"

	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"

	strategiesCommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/rollout/strategies/common"
)

// Params contains dependencies for strategy actor construction.
type Params struct {
	ClientFactory       clientfactory.ClientFactory
	RouteManager        routing.Manager
	BackendRegistry     *backends.Registry
	Logger              *zap.Logger
	BackendType         v2pb.BackendType

	// DynamicClient is the dynamic client for the control-plane cluster.
	DynamicClient dynamic.Interface

	// Client is the controller-runtime client for the control-plane cluster.
	Client client.Client

	// HTTPClient is the HTTP client for the control-plane cluster.
	HTTPClient *http.Client
}

// GetActorsForStrategy returns the ordered actor chain for the deployment's rollout strategy.
// Each cluster gets its own RollingRolloutActor; the model is exposed via a single
// DiscoveryRoutingActor that adds the deployment's rule to the InferenceServer's discovery
// route. Cleanup actors follow at the end so old models are removed only after every cluster
// has flipped to the new model.
func GetActorsForStrategy(ctx context.Context, params Params, deployment *v2pb.Deployment) ([]conditionInterfaces.ConditionActor[*v2pb.Deployment], error) {
	strategy := getDeploymentStrategy(deployment)
	params.Logger.Info("Selected rollout strategy", zap.String("strategy", strategy), zap.String("deployment", deployment.Name))

	switch strategy {
	case "zonal":
		return getZonalActors(params, deployment)
	// TODO(#623): Implement blast, shadow, and disaggregated strategies.
	case "rolling":
		fallthrough
	default:
		return getRollingActors(params, deployment)
	}
}

// getRollingActors builds the actor chain for the rolling strategy using incrementPercentage
// to control how many clusters advance per wave. Clusters within a wave load the model
// concurrently (BatchRolloutActor), then traffic is routed per-cluster sequentially before
// the next wave begins. Default incrementPercentage=100 advances all clusters at once.
//
// Example — 4 clusters, incrementPercentage=50:
//
//	Wave 0: [BatchRollout(c1,c2)] → [Route-c1] → [Route-c2]
//	Wave 1: [BatchRollout(c3,c4)] → [Route-c3] → [Route-c4]
//	        → [Discovery] → [Cleanup-c1..c4]
func getRollingActors(params Params, deployment *v2pb.Deployment) ([]conditionInterfaces.ConditionActor[*v2pb.Deployment], error) {
	targets, err := osscommon.ReadTargetClustersAnnotation(deployment)
	if err != nil {
		return nil, fmt.Errorf("read target clusters annotation: %w", err)
	}
	if len(targets) == 0 {
		return nil, nil
	}

	pct := int(deployment.Spec.GetStrategy().GetRolling().GetIncrementPercentage())
	if pct <= 0 || pct > 100 {
		pct = 100 // default: advance all clusters at once
	}
	batchSize := int(math.Ceil(float64(len(targets)) * float64(pct) / 100.0))
	if batchSize < 1 {
		batchSize = 1
	}

	params.Logger.Info("rolling strategy batch config",
		zap.Int("totalClusters", len(targets)),
		zap.Int("incrementPercentage", pct),
		zap.Int("batchSize", batchSize),
		zap.String("deployment", deployment.Name))

	actors := make([]conditionInterfaces.ConditionActor[*v2pb.Deployment], 0, 3*len(targets)+1)

	for batchIdx := 0; batchIdx < len(targets); batchIdx += batchSize {
		end := batchIdx + batchSize
		if end > len(targets) {
			end = len(targets)
		}
		batch := targets[batchIdx:end]
		waveName := batchIdx / batchSize

		actors = append(actors,
			strategiesCommon.NewBatchRolloutActor(params.ClientFactory, params.BackendRegistry, params.BackendType, params.Logger, batch, waveName),
		)
		for _, target := range batch {
			actors = append(actors, strategiesCommon.NewTrafficRoutingActor(params.ClientFactory, params.RouteManager, target))
		}
	}

	actors = append(actors, strategiesCommon.NewDiscoveryRoutingActor(params.DynamicClient, params.RouteManager))
	for _, target := range targets {
		actors = append(actors, strategiesCommon.NewModelCleanupActor(params.ClientFactory, params.BackendRegistry, params.BackendType, params.Logger, target))
	}

	return actors, nil
}

// getZonalActors builds the per-cluster actor chain for the zonal strategy.
// Clusters are processed sequentially: each cluster must load the model, route
// traffic, and complete its soak period before the next cluster begins.
// Old models are cleaned up only after every cluster has advanced.
func getZonalActors(params Params, deployment *v2pb.Deployment) ([]conditionInterfaces.ConditionActor[*v2pb.Deployment], error) {
	targets, err := osscommon.ReadTargetClustersAnnotation(deployment)
	if err != nil {
		return nil, fmt.Errorf("read target clusters annotation: %w", err)
	}
	if len(targets) == 0 {
		return nil, nil
	}

	soakSeconds := deployment.Spec.GetStrategy().GetZonal().GetRolloutPeriodInSeconds()

	// For each cluster (zone): [RollingRollout → TrafficRouting → ZonalSoak]
	// The condition engine stops at the first non-TRUE actor, so cluster N+1
	// cannot start until cluster N's soak actor returns TRUE.
	// DiscoveryRouting and ModelCleanup follow once all zones are done.
	actors := make([]conditionInterfaces.ConditionActor[*v2pb.Deployment], 0, 4*len(targets)+1)

	for _, target := range targets {
		actors = append(actors,
			strategiesCommon.NewRollingRolloutActor(params.ClientFactory, params.BackendRegistry, params.BackendType, params.Logger, target),
			strategiesCommon.NewTrafficRoutingActor(params.ClientFactory, params.RouteManager, target),
			strategiesCommon.NewZonalSoakActor(target, soakSeconds, params.Logger),
		)
	}
	actors = append(actors, strategiesCommon.NewDiscoveryRoutingActor(params.DynamicClient, params.RouteManager))
	for _, target := range targets {
		actors = append(actors, strategiesCommon.NewModelCleanupActor(params.ClientFactory, params.BackendRegistry, params.BackendType, params.Logger, target))
	}

	return actors, nil
}

// getDeploymentStrategy determines the rollout strategy from deployment configuration.
func getDeploymentStrategy(deployment *v2pb.Deployment) string {
	switch deployment.Spec.GetStrategy().GetRolloutStrategy().(type) {
	case *v2pb.DeploymentStrategy_Zonal:
		return "zonal"
	case *v2pb.DeploymentStrategy_Rolling:
		return "rolling"
	default:
		return "rolling"
	}
}
