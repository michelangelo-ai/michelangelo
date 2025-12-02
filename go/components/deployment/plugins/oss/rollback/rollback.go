package rollback

import (
	"context"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// RollbackActor handles rollback operations following Uber patterns
type RollbackActor struct {
	logger *zap.Logger
}

func (a *RollbackActor) GetType() string {
	return common.ActorTypeRollback
}

func (a *RollbackActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	// Check if rollback is complete when we restore to the previous revision
	if resource.Status.CurrentRevision != nil {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Reason:  "RollbackCompleted",
			Message: "Rollback completed successfully",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "RollbackInProgress",
		Message: "Rollback in progress",
	}, nil
}

func (a *RollbackActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	a.logger.Info("Running rollback for deployment", zap.String("deployment", resource.Name))

	// Update deployment status to indicate rollback is in progress
	resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS
	resource.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY

	if resource.Status.CurrentRevision != nil {
		// Store the failed revision for reference
		failedRevision := resource.Spec.DesiredRevision

		// For OSS, rollback means restoring the previous revision
		resource.Spec.DesiredRevision = resource.Status.CurrentRevision

		// Update status to reflect rollback completion
		resource.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE
		resource.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY

		a.logger.Info("Rolled back to previous revision",
			zap.String("from", failedRevision.Name),
			zap.String("to", resource.Status.CurrentRevision.Name))
	} else {
		a.logger.Info("No previous revision available for rollback")
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "RollbackCompleted",
		Message: "Rollback completed successfully",
	}, nil
}
