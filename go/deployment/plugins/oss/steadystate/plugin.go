package steadystate

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

// Plugin handles steady state operations for OSS deployments
type Plugin struct {
	gateway inferenceserver.Gateway
	actors  []common.Actor
}


// NewPlugin creates a new steady state plugin
func NewPlugin(gateway inferenceserver.Gateway) *Plugin {
	plugin := &Plugin{
		gateway: gateway,
	}
	
	// Initialize actors
	plugin.actors = []common.Actor{
		&SteadyStateActor{gateway: gateway},
	}
	
	return plugin
}

// Execute runs the steady state process
func (p *Plugin) Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("Starting OSS steady state monitoring")
	
	for _, actor := range p.actors {
		logger.Info("Executing steady state actor", "type", actor.GetType())
		
		if err := actor.Execute(ctx, logger, deployment); err != nil {
			logger.Error(err, "Steady state actor execution failed", "type", actor.GetType())
			p.setCondition(deployment, actor.GetType(), apipb.CONDITION_STATUS_FALSE, err.Error())
			return fmt.Errorf("steady state monitoring failed at %s: %w", actor.GetType(), err)
		}
		
		p.setCondition(deployment, actor.GetType(), apipb.CONDITION_STATUS_TRUE, "Steady state monitoring completed successfully")
		logger.Info("Steady state actor completed successfully", "type", actor.GetType())
	}
	
	logger.Info("OSS steady state monitoring completed successfully")
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

// SteadyStateActor monitors deployment in steady state
type SteadyStateActor struct {
	gateway inferenceserver.Gateway
}

func (a *SteadyStateActor) GetType() string {
	return common.ActorTypeSteadyState
}

func (a *SteadyStateActor) Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("Monitoring deployment in steady state")
	
	// Check if there's a current revision to monitor
	if deployment.Status.CurrentRevision == nil {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_EMPTY
		deployment.Status.Message = "No current revision deployed"
		return nil
	}
	
	inferenceServerName := common.GetInferenceServerName(*deployment)
	if inferenceServerName == "" {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		deployment.Status.Message = "No inference server specified"
		return fmt.Errorf("no inference server specified")
	}
	
	// Check if inference server is healthy
	isHealthy, err := a.gateway.IsHealthy(ctx, logger, inferenceServerName, v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		logger.Error(err, "Failed to check inference server health")
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		deployment.Status.Message = fmt.Sprintf("Health check failed: %v", err)
		return err
	}
	
	// Update deployment state based on health
	if isHealthy {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		deployment.Status.Message = fmt.Sprintf("Deployment %s is healthy and serving", deployment.Status.CurrentRevision.Name)
	} else {
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_UNHEALTHY
		deployment.Status.Message = fmt.Sprintf("Deployment %s is unhealthy", deployment.Status.CurrentRevision.Name)
	}
	
	logger.Info("Steady state monitoring completed successfully", 
		"state", deployment.Status.State, 
		"healthy", isHealthy)
	return nil
}