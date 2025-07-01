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

// Execute performs the rollback operation
func (a *rollbackActor) Execute(ctx context.Context, requestCtx plugins.RequestContext, logger logr.Logger) (*apipb.Condition, error) {
	logger.Info("Executing rollback for deployment", "deployment", requestCtx.Deployment.Name)

	// Rollback logic:
	// 1. Revert to previous revision if available
	// 2. Update routing back to previous model
	// 3. Clean up failed candidate deployment

	if requestCtx.Deployment.Status.CurrentRevision != nil {
		logger.Info("Rolling back to previous revision", 
			"current", requestCtx.Deployment.Status.CurrentRevision.Name,
			"failed", requestCtx.Deployment.Spec.DesiredRevision.Name)
		
		// Set desired revision back to current (previous working) revision
		requestCtx.Deployment.Spec.DesiredRevision = requestCtx.Deployment.Status.CurrentRevision
		requestCtx.Deployment.Status.CandidateRevision = nil
	}

	requestCtx.Deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE
	requestCtx.Deployment.Status.Message = "Rollback completed successfully"

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_TRUE,
		Message: "Rollback executed successfully",
		Reason:  "RollbackExecuted",
	}, nil
}

// EvaluateCondition checks the status of the rollback
func (a *rollbackActor) EvaluateCondition(ctx context.Context, requestCtx plugins.RequestContext, logger logr.Logger) (*apipb.Condition, error) {
	// Check if rollback is complete
	if requestCtx.Deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE {
		return &apipb.Condition{
			Type:    a.GetType(),
			Status:  apipb.CONDITION_STATUS_TRUE,
			Message: "Rollback completed successfully",
			Reason:  "RollbackComplete",
		}, nil
	}

	return &apipb.Condition{
		Type:    a.GetType(),
		Status:  apipb.CONDITION_STATUS_FALSE,
		Message: "Rollback in progress",
		Reason:  "RollbackInProgress",
	}, nil
}