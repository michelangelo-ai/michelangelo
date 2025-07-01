package rollback

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Plugin handles rollback operations for OSS deployments
type Plugin struct {
	gateway inferenceserver.Gateway
	actors  []common.Actor
}


// NewPlugin creates a new rollback plugin
func NewPlugin(gateway inferenceserver.Gateway) *Plugin {
	plugin := &Plugin{
		gateway: gateway,
	}
	
	// Initialize actors
	plugin.actors = []common.Actor{
		&RollbackActor{gateway: gateway},
	}
	
	return plugin
}

// Execute runs the rollback process
func (p *Plugin) Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("Starting OSS rollback process")
	
	for _, actor := range p.actors {
		logger.Info("Executing rollback actor", "type", actor.GetType())
		
		if err := actor.Execute(ctx, logger, deployment); err != nil {
			logger.Error(err, "Rollback actor execution failed", "type", actor.GetType())
			p.setCondition(deployment, actor.GetType(), apipb.CONDITION_STATUS_FALSE, err.Error())
			return fmt.Errorf("rollback failed at %s: %w", actor.GetType(), err)
		}
		
		p.setCondition(deployment, actor.GetType(), apipb.CONDITION_STATUS_TRUE, "Rollback completed successfully")
		logger.Info("Rollback actor completed successfully", "type", actor.GetType())
	}
	
	logger.Info("OSS rollback process completed successfully")
	return nil
}

// GetActors returns the list of actors
func (p *Plugin) GetActors() []common.Actor {
	return p.actors
}

// setCondition sets a condition on the deployment
func (p *Plugin) setCondition(deployment *v2pb.Deployment, actorType string, status apipb.ConditionStatus, message string) {
	now := metav1.Now().Unix()
	
	// Find existing condition or create new one
	var condition *apipb.Condition
	for i, cond := range deployment.Status.Conditions {
		if cond.Type == actorType {
			condition = deployment.Status.Conditions[i]
			break
		}
	}
	
	if condition == nil {
		// Create new condition
		newCondition := &apipb.Condition{
			Type:   actorType,
			Status: status,
			LastUpdatedTimestamp: now,
			Message: message,
		}
		deployment.Status.Conditions = append(deployment.Status.Conditions, newCondition)
	} else {
		// Update existing condition
		condition.Status = status
		condition.LastUpdatedTimestamp = now
		condition.Message = message
	}
}

// RollbackActor handles deployment rollback
type RollbackActor struct {
	gateway inferenceserver.Gateway
}

func (a *RollbackActor) GetType() string {
	return common.ActorTypeRollback
}

func (a *RollbackActor) Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("Performing deployment rollback")
	
	// For OSS deployments, rollback means reverting to the previous revision
	if deployment.Status.CurrentRevision == nil {
		logger.Info("No current revision to rollback to")
		deployment.Status.Message = "No previous revision available for rollback"
		return nil
	}
	
	// Set desired revision back to current revision
	deployment.Spec.DesiredRevision = deployment.Status.CurrentRevision
	deployment.Status.CandidateRevision = nil
	deployment.Status.Message = fmt.Sprintf("Rolled back to revision %s", deployment.Status.CurrentRevision.Name)
	
	logger.Info("Deployment rollback completed successfully", 
		"revision", deployment.Status.CurrentRevision.Name)
	return nil
}