package cleanup

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

// Plugin handles cleanup operations for OSS deployments
type Plugin struct {
	gateway inferenceserver.Gateway
	actors  []common.Actor
}


// NewPlugin creates a new cleanup plugin
func NewPlugin(gateway inferenceserver.Gateway) *Plugin {
	plugin := &Plugin{
		gateway: gateway,
	}
	
	// Initialize actors
	plugin.actors = []common.Actor{
		&CleanupActor{gateway: gateway},
	}
	
	return plugin
}

// Execute runs the cleanup process
func (p *Plugin) Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("Starting OSS cleanup process")
	
	for _, actor := range p.actors {
		logger.Info("Executing cleanup actor", "type", actor.GetType())
		
		if err := actor.Execute(ctx, logger, deployment); err != nil {
			logger.Error(err, "Cleanup actor execution failed", "type", actor.GetType())
			p.setCondition(deployment, actor.GetType(), apipb.CONDITION_STATUS_FALSE, err.Error())
			return fmt.Errorf("cleanup failed at %s: %w", actor.GetType(), err)
		}
		
		p.setCondition(deployment, actor.GetType(), apipb.CONDITION_STATUS_TRUE, "Cleanup completed successfully")
		logger.Info("Cleanup actor completed successfully", "type", actor.GetType())
	}
	
	logger.Info("OSS cleanup process completed successfully")
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

// CleanupActor handles deployment cleanup
type CleanupActor struct {
	gateway inferenceserver.Gateway
}

func (a *CleanupActor) GetType() string {
	return common.ActorTypeCleanup
}

func (a *CleanupActor) Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("Performing deployment cleanup")
	
	// For OSS deployments, cleanup is handled by the inference server controller
	// when the deployment is deleted. We just need to clear the deployment status.
	
	// Clear current and candidate revisions
	deployment.Status.CurrentRevision = nil
	deployment.Status.CandidateRevision = nil
	deployment.Status.State = v2pb.DEPLOYMENT_STATE_EMPTY
	deployment.Status.Message = "Deployment cleanup completed"
	
	logger.Info("Deployment cleanup completed successfully")
	return nil
}