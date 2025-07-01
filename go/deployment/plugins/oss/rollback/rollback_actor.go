package rollback

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

type rollbackActor struct {
	client client.Client
	logger logr.Logger
}

var _ plugins.ConditionActor = &rollbackActor{}

// GetType returns the actor type
func (a *rollbackActor) GetType() string {
	return "RollbackComplete"
}

// Run executes the rollback logic
func (a *rollbackActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, deployment *v2pb.Deployment, condition *v2pb.Condition) error {
	runtimeCtx.Logger.Info("Executing rollback for deployment", "deployment", deployment.Name)

	// Rollback logic:
	// 1. Revert to previous revision if available
	// 2. Update routing back to previous model
	// 3. Clean up failed candidate deployment

	if deployment.Status.CurrentRevision != nil {
		runtimeCtx.Logger.Info("Rolling back to previous revision", 
			"current", deployment.Status.CurrentRevision.Name,
			"failed", deployment.Spec.DesiredRevision.Name)
		
		// Set desired revision back to current (previous working) revision
		deployment.Spec.DesiredRevision = deployment.Status.CurrentRevision
		deployment.Status.CandidateRevision = nil
	}

	deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE
	deployment.Status.Message = "Rollback completed successfully"

	return nil
}

// Retrieve checks the status of the rollback
func (a *rollbackActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, deployment *v2pb.Deployment, condition apipb.Condition) (apipb.Condition, error) {
	// Check if rollback is complete
	if deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE {
		return apipb.Condition{
			Type:    condition.Type,
			Status:  apipb.CONDITION_STATUS_TRUE,
			Message: "Rollback completed successfully",
			Reason:  "RollbackComplete",
		}, nil
	}

	return apipb.Condition{
		Type:    condition.Type,
		Status:  apipb.CONDITION_STATUS_FALSE,
		Message: "Rollback in progress",
		Reason:  "RollbackInProgress",
	}, nil
}