package deletion

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &RouteCleanupActor{}

// RouteCleanupActor removes the InferenceServer's HTTPRoutes during deletion:
// the discovery route in the control plane and the traffic route in every
// target cluster.
type RouteCleanupActor struct {
	dynamicClient dynamic.Interface
	clientFactory clientfactory.ClientFactory
	routeManager  routing.Manager
	logger        *zap.Logger
}

// NewRouteCleanupActor creates the condition actor that tears down both
// HTTPRoutes when an InferenceServer is being deleted.
func NewRouteCleanupActor(dynamicClient dynamic.Interface, clientFactory clientfactory.ClientFactory, routeManager routing.Manager, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &RouteCleanupActor{
		dynamicClient: dynamicClient,
		clientFactory: clientFactory,
		routeManager:  routeManager,
		logger:        logger,
	}
}

// GetType returns the condition type identifier.
func (a *RouteCleanupActor) GetType() string {
	return common.RouteCleanupConditionType
}

// Retrieve reports TRUE only when the discovery and traffic HTTPRoutes are
// gone in every relevant cluster, so the engine fires Run while any of them
// still exist.
func (a *RouteCleanupActor) Retrieve(ctx context.Context, server *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	exists, err := a.routeManager.Exists(ctx, a.dynamicClient, routenames.DiscoveryRouteName(server.Name), server.Namespace)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "GetFailed", fmt.Sprintf("discovery: %v", err)), nil
	}
	if exists {
		return conditionsutil.GenerateFalseCondition(condition, "DiscoveryRouteStillExists", "discovery HTTPRoute is still present"), nil
	}
	var remaining []string
	for _, target := range server.Spec.ClusterTargets {
		clusterID := target.GetClusterId()
		dynamicClient, err := a.clientFactory.GetDynamicClient(ctx, target)
		if err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "GetFailed", fmt.Sprintf("%s: %v", clusterID, err)), nil
		}
		exists, err := a.routeManager.Exists(ctx, dynamicClient, routenames.TrafficRouteName(server.Name), server.Namespace)
		if err != nil {
			return conditionsutil.GenerateFalseCondition(condition, "GetFailed", fmt.Sprintf("%s: %v", clusterID, err)), nil
		}
		if exists {
			remaining = append(remaining, clusterID)
		}
	}
	if len(remaining) > 0 {
		sort.Strings(remaining)
		return conditionsutil.GenerateFalseCondition(condition, "TrafficRouteStillExists", strings.Join(remaining, ",")), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run deletes the discovery route in the control plane and the traffic route
// in every target cluster.
func (a *RouteCleanupActor) Run(ctx context.Context, server *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	var failures []string
	if err := a.routeManager.Delete(ctx, a.dynamicClient, routenames.DiscoveryRouteName(server.Name), server.Namespace); err != nil {
		failures = append(failures, fmt.Sprintf("discovery: %v", err))
	}
	for _, target := range server.Spec.ClusterTargets {
		clusterID := target.GetClusterId()
		dynamicClient, err := a.clientFactory.GetDynamicClient(ctx, target)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", clusterID, err))
			continue
		}
		if err := a.routeManager.Delete(ctx, dynamicClient, routenames.TrafficRouteName(server.Name), server.Namespace); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", clusterID, err))
		}
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		return conditionsutil.GenerateFalseCondition(condition, "DeleteFailed", strings.Join(failures, "; ")), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}
