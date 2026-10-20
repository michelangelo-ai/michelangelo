package strategies

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	goapi "github.com/michelangelo-ai/michelangelo/go/api"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	modelconfig "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"

	strategiesCommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/rollout/strategies/common"
)

// Params contains dependencies for strategy actor construction.
type Params struct {
	ClientFactory       clientfactory.ClientFactory
	RouteManager        routing.Manager
	BackendRegistry     *backends.Registry
	ModelConfigProvider modelconfig.ModelConfigProvider
	Logger              *zap.Logger

	// Settings are the resolved rollout knobs: canary, load budget and soak.
	Settings osscommon.RolloutSettings

	// DynamicClient is the dynamic client for the control-plane cluster. Retained so that
	// actors operating on control-plane-only resources can access it directly.
	DynamicClient dynamic.Interface

	// Client is the controller-runtime client for the control-plane cluster.
	Client client.Client

	// APIHandler reads control-plane API objects that may live in metadata storage rather
	// than etcd, such as the immutable Model kind.
	APIHandler goapi.Handler

	// HTTPClient is the HTTP client for the control-plane cluster.
	HTTPClient *http.Client
}

// GetActorsForStrategy returns the ordered actor chain for the deployment's rollout strategy.
// Every strategy rolls the model out one cluster at a time; the strategy only decides the
// per-cluster safety steps (canary, soak) through the resolved Settings. Inside a cluster the
// order is canary → load on every replica → route traffic → soak, and the model is exposed via
// a single DiscoveryRoutingActor once every cluster serves it. Cleanup actors follow at the end
// so old models are removed only after every cluster has flipped to the new model.
func GetActorsForStrategy(ctx context.Context, params Params, deployment *v2pb.Deployment) ([]conditionInterfaces.ConditionActor[*v2pb.Deployment], error) {
	strategy := getDeploymentStrategy(deployment)
	params.Logger.Info("Selected rollout strategy",
		zap.String("strategy", strategy),
		zap.String("deployment", deployment.Name),
		zap.Bool("canary", params.Settings.Canary),
		zap.Duration("soakPeriod", params.Settings.SoakPeriod),
		zap.Duration("modelLoadTimeout", params.Settings.ModelLoadTimeout))

	return getClusterSequenceActors(params, deployment)
}

// getClusterSequenceActors builds the per-cluster actor chain. The actor list is constructed
// from the cluster snapshot annotation written by PlacementPrepActor.
func getClusterSequenceActors(params Params, deployment *v2pb.Deployment) ([]conditionInterfaces.ConditionActor[*v2pb.Deployment], error) {
	targets, err := osscommon.ReadTargetClustersAnnotation(deployment)
	if err != nil {
		return nil, fmt.Errorf("read target clusters annotation: %w", err)
	}
	if len(targets) == 0 {
		// Annotation absent or no healthy clusters yet. Return an empty list so
		// per-cluster actors are omitted this reconcile; they are added once the
		// annotation is written and a healthy cluster is available.
		return nil, nil
	}

	deps := strategiesCommon.ClusterActorDeps{
		ClientFactory:       params.ClientFactory,
		APIHandler:          params.APIHandler,
		BackendRegistry:     params.BackendRegistry,
		ModelConfigProvider: params.ModelConfigProvider,
		RouteManager:        params.RouteManager,
		Logger:              params.Logger,
		Settings:            params.Settings,
	}

	// Per-cluster [Canary, RollingRollout, TrafficRouting, Soak] runs come first, so cluster
	// N only starts once cluster N-1 serves the new model and has soaked on it. A single
	// DiscoveryRoutingActor then exposes the deployment via the control-plane discovery
	// route. Per-cluster ModelCleanup actors run at the end so old models are removed only
	// after every cluster has flipped.
	actors := make([]conditionInterfaces.ConditionActor[*v2pb.Deployment], 0, 5*len(targets)+1)

	for _, target := range targets {
		if params.Settings.Canary {
			actors = append(actors, strategiesCommon.NewCanaryRolloutActor(deps, target))
		}
		actors = append(actors,
			strategiesCommon.NewRollingRolloutActor(deps, target),
			strategiesCommon.NewTrafficRoutingActor(deps, target),
		)
		if params.Settings.SoakPeriod > 0 {
			actors = append(actors, strategiesCommon.NewSoakActor(deps, target))
		}
	}
	actors = append(actors, strategiesCommon.NewDiscoveryRoutingActor(params.DynamicClient, params.RouteManager))
	for _, target := range targets {
		actors = append(actors, strategiesCommon.NewModelCleanupActor(params.ClientFactory, params.ModelConfigProvider, params.Logger, target))
	}

	return actors, nil
}

// getDeploymentStrategy names the rollout strategy from deployment configuration.
func getDeploymentStrategy(deployment *v2pb.Deployment) string {
	switch deployment.Spec.GetStrategy().GetRolloutStrategy().(type) {
	case *v2pb.DeploymentStrategy_Zonal:
		return "zonal"
	case *v2pb.DeploymentStrategy_Blast:
		return "blast"
	case *v2pb.DeploymentStrategy_Rolling:
		return "rolling"
	default:
		return "rolling"
	}
}
