package rollback

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &RollbackCompletionActor{}

// RollbackCompletionActor finishes a rollback. When no previous revision exists the
// deployment no longer serves anything, so the deployment's rule on the control-plane
// discovery route is removed. Its condition type is the terminal rollback marker the plugin's
// ParseStage maps to ROLLBACK_COMPLETE, so it must stay last in the actor chain.
type RollbackCompletionActor struct {
	dynamicClient dynamic.Interface
	routeManager  routing.Manager
	logger        *zap.Logger
}

// NewRollbackCompletionActor creates the terminal rollback actor.
func NewRollbackCompletionActor(p Params) *RollbackCompletionActor {
	logger := p.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RollbackCompletionActor{dynamicClient: p.DynamicClient, routeManager: p.RouteManager, logger: logger}
}

// GetType returns the terminal rollback condition type.
func (a *RollbackCompletionActor) GetType() string {
	return osscommon.ActorTypeRollback
}

// Retrieve reports TRUE when a previous revision keeps serving the deployment, or when the
// discovery rule is gone for a deployment with nothing left to serve.
func (a *RollbackCompletionActor) Retrieve(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if deployment.Status.GetCurrentRevision() != nil {
		return conditionsutil.GenerateTrueCondition(condition), nil
	}
	isName := deployment.Spec.GetInferenceServer().GetName()
	exists, err := a.routeManager.RuleExists(ctx, a.dynamicClient, routenames.DiscoveryRouteName(isName), deployment.Namespace,
		routing.Rule{MatchPath: routenames.DiscoveryMatchPath(isName, deployment.Name)})
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DiscoveryRouteStatusCheckFailed", err.Error()), nil
	}
	if exists {
		return conditionsutil.GenerateFalseCondition(condition, ReasonDiscoveryRouteStillPresent,
			fmt.Sprintf("discovery route for deployment %s still exists although there is no model to serve", deployment.Name)), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run removes the deployment's discovery rule.
func (a *RollbackCompletionActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if deployment.Status.GetCurrentRevision() != nil {
		return conditionsutil.GenerateTrueCondition(condition), nil
	}
	isName := deployment.Spec.GetInferenceServer().GetName()
	a.logger.Info("Removing discovery route; there is no previous model to serve", zap.String("deployment", deployment.Name))
	if err := a.routeManager.RemoveRules(ctx, a.dynamicClient, routenames.DiscoveryRouteName(isName), deployment.Namespace, routenames.DiscoveryMatchPath(isName, deployment.Name)); err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "DiscoveryRouteRemovalFailed", err.Error()), nil
	}
	return conditionsutil.GenerateTrueCondition(condition), nil
}
