package creation

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &TrafficRouteProvisionActor{}

// TrafficRouteProvisionActor reconciles the per-target-cluster HTTPRoute that
// forwards Triton-formatted requests to the local inference Service. One actor
// instance handles every cluster in the InferenceServer's spec; per-cluster
// failures are aggregated into a single condition.
type TrafficRouteProvisionActor struct {
	clientFactory clientfactory.ClientFactory
	routeManager  routing.Manager
	gatewayName   string
	logger        *zap.Logger
}

// NewTrafficRouteProvisionActor creates the condition actor that maintains
// the per-cluster traffic HTTPRoute for an InferenceServer.
func NewTrafficRouteProvisionActor(clientFactory clientfactory.ClientFactory, routeManager routing.Manager, gatewayName string, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &TrafficRouteProvisionActor{
		clientFactory: clientFactory,
		routeManager:  routeManager,
		gatewayName:   gatewayName,
		logger:        logger,
	}
}

// GetType returns the condition type identifier.
func (a *TrafficRouteProvisionActor) GetType() string {
	return common.TrafficRouteProvisionType
}

// Retrieve checks every target cluster has its traffic HTTPRoute provisioned.
// Reports the missing or unreachable clusters in the condition.
func (a *TrafficRouteProvisionActor) Retrieve(ctx context.Context, server *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	routeName := routenames.TrafficRouteName(server.Name)
	var missing []string
	for _, target := range server.Spec.ClusterTargets {
		clusterID := target.GetClusterId()
		dynamicClient, err := a.clientFactory.GetDynamicClient(ctx, target)
		if err != nil {
			missing = append(missing, fmt.Sprintf("%s: %v", clusterID, err))
			continue
		}
		provisioned, err := a.routeManager.Exists(ctx, dynamicClient, routeName, server.Namespace)
		if err != nil {
			missing = append(missing, fmt.Sprintf("%s: %v", clusterID, err))
			continue
		}
		if !provisioned {
			missing = append(missing, clusterID)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return conditionsutil.GenerateFalseCondition(condition, "TrafficRouteMissing", strings.Join(missing, ",")), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run ensures the traffic HTTPRoute exists in every target cluster. Per-cluster
// failures keep the condition FALSE without aborting other clusters.
func (a *TrafficRouteProvisionActor) Run(ctx context.Context, server *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	routeName := routenames.TrafficRouteName(server.Name)
	config := routing.RouteConfig{
		GatewayName:      a.gatewayName,
		GatewayNamespace: server.Namespace,
		Rules:            []routing.Rule{trafficDefaultRule(server.Name)},
	}
	defaultRule := trafficDefaultRule(server.Name)

	var failures []string
	for _, target := range server.Spec.ClusterTargets {
		clusterID := target.GetClusterId()
		dynamicClient, err := a.clientFactory.GetDynamicClient(ctx, target)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", clusterID, err))
			continue
		}
		if err := a.routeManager.Create(ctx, dynamicClient, routeName, server.Namespace, config); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", clusterID, err))
			continue
		}
		if err := a.routeManager.AddRules(ctx, dynamicClient, routeName, server.Namespace, defaultRule); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", clusterID, err))
		}
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		return conditionsutil.GenerateFalseCondition(condition, "EnsureFailed", strings.Join(failures, "; ")), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// trafficDefaultRule is the IS-level default rule on the per-cluster traffic HTTPRoute.
func trafficDefaultRule(isName string) routing.Rule {
	return routing.Rule{
		MatchPath:   "/cluster/" + isName,
		MatchType:   routing.PathMatchExact,
		RewritePath: "/v2",
		RewriteType: routing.RewriteFullPath,
		BackendName: isName + "-inference-service",
	}
}
