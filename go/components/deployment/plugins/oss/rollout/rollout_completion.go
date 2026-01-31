package rollout

import (
	"context"

	"go.uber.org/zap"

	"github.com/gogo/protobuf/types"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &RolloutCompletionActor{}

// RolloutCompletionActor finalizes deployment by updating CurrentRevision and cleaning up rollout metadata.
type RolloutCompletionActor struct {
	gateway gateways.Gateway
	logger  *zap.Logger
}

// GetType returns the condition type identifier for rollout completion.
func (a *RolloutCompletionActor) GetType() string {
	return common.ActorTypeRolloutComplete
}

// Retrieve checks if the deployment has reached rollout complete stage with healthy state.
func (a *RolloutCompletionActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	cleanupComplete := &types.BoolValue{}
	_ = types.UnmarshalAny(condition.Metadata, cleanupComplete)
	if cleanupComplete.Value {
		return conditionsutil.GenerateTrueCondition(condition), nil
	}
	return conditionsutil.GenerateFalseCondition(condition, "CompletionTasksPending", "Rollout completion tasks are pending"), nil
}

// Run updates CurrentRevision to DesiredRevision and removes temporary rollout annotations.
func (a *RolloutCompletionActor) Run(ctx context.Context, deployment *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running rollout completion tasks for deployment", zap.String("deployment", deployment.Name))

	// Clean up any temporary annotations or metadata
	if deployment.Annotations != nil {
		// Remove rollout-specific annotations
		delete(deployment.Annotations, "rollout.michelangelo.ai/in-progress")
		delete(deployment.Annotations, "rollout.michelangelo.ai/start-time")
	}

	var err error
	cleanupComplete := &types.BoolValue{Value: true}
	condition.Metadata, err = types.MarshalAny(cleanupComplete)
	if err != nil {
		a.logger.Error("marshal failed", zap.Error(err))
		return condition, nil
	}

	return conditionsutil.GenerateTrueCondition(condition), nil
}
