package creation

import (
	"context"

	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.InferenceServer] = &DiscoveryRouteProvisionActor{}

// DiscoveryRouteProvisionActor reconciles the control-plane HTTPRoute that
// exposes the InferenceServer at /{inferenceServerName} and forwards inbound
// traffic to the discovery Service that fans out across hosting clusters.
type DiscoveryRouteProvisionActor struct {
	dynamicClient dynamic.Interface
	routeManager  routing.Manager
	gatewayName   string
	logger        *zap.Logger
}

// NewDiscoveryRouteProvisionActor creates the condition actor that maintains
// the control-plane discovery HTTPRoute for an InferenceServer.
func NewDiscoveryRouteProvisionActor(dynamicClient dynamic.Interface, routeManager routing.Manager, gatewayName string, logger *zap.Logger) conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	return &DiscoveryRouteProvisionActor{
		dynamicClient: dynamicClient,
		routeManager:  routeManager,
		gatewayName:   gatewayName,
		logger:        logger,
	}
}

// GetType returns the condition type identifier.
func (a *DiscoveryRouteProvisionActor) GetType() string {
	return common.DiscoveryRouteProvisionType
}

// Retrieve checks whether the discovery HTTPRoute has been provisioned.
func (a *DiscoveryRouteProvisionActor) Retrieve(ctx context.Context, server *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	provisioned, err := a.routeManager.Exists(ctx, a.dynamicClient, routenames.DiscoveryRouteName(server.Name), server.Namespace)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "GetFailed", err.Error()), nil
	}
	if !provisioned {
		return conditionsutil.GenerateFalseCondition(condition, "DiscoveryRouteMissing", "discovery HTTPRoute is not provisioned"), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run ensures the discovery HTTPRoute exists with its default rule.
func (a *DiscoveryRouteProvisionActor) Run(ctx context.Context, server *v2pb.InferenceServer, condition *apipb.Condition) (*apipb.Condition, error) {
	routeName := routenames.DiscoveryRouteName(server.Name)
	config := routing.RouteConfig{
		GatewayName:      a.gatewayName,
		GatewayNamespace: server.Namespace,
		OwnerRef: &routing.OwnerRef{
			APIVersion: v2pb.GroupVersion.String(),
			Kind:       "InferenceServer",
			Name:       server.Name,
			UID:        server.UID,
		},
		Rules: []routing.Rule{discoveryDefaultRule(server.Name)},
	}
	if err := a.routeManager.Create(ctx, a.dynamicClient, routeName, server.Namespace, config); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "EnsureFailed", err.Error()), nil
	}
	if err := a.routeManager.AddRules(ctx, a.dynamicClient, routeName, server.Namespace, discoveryDefaultRule(server.Name)); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "EnsureFailed", err.Error()), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// discoveryDefaultRule is the IS-level catch-all rule on the discovery HTTPRoute.
func discoveryDefaultRule(isName string) routing.Rule {
	return routing.Rule{
		MatchPath:   "/" + isName,
		MatchType:   routing.PathMatchExact,
		RewritePath: "/cluster/" + isName,
		RewriteType: routing.RewriteFullPath,
		BackendName: isName + "-endpoints",
	}
}
