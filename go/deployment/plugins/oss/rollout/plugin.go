package rollout

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

// Plugin handles rollout operations for OSS deployments
type Plugin struct {
	gateway inferenceserver.Gateway
	actors  []common.Actor
}


// NewPlugin creates a new rollout plugin
func NewPlugin(gateway inferenceserver.Gateway) *Plugin {
	plugin := &Plugin{
		gateway: gateway,
	}
	
	// Initialize actors in execution order
	plugin.actors = []common.Actor{
		&ValidationActor{gateway: gateway},
		&ResourcePreparationActor{gateway: gateway},
		&ModelLoadActor{gateway: gateway},
		&HealthCheckActor{gateway: gateway},
	}
	
	return plugin
}

// Execute runs the rollout process
func (p *Plugin) Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("Starting OSS rollout process")
	
	for _, actor := range p.actors {
		logger.Info("Executing actor", "type", actor.GetType())
		
		if err := actor.Execute(ctx, logger, deployment); err != nil {
			logger.Error(err, "Actor execution failed", "type", actor.GetType())
			p.setCondition(deployment, actor.GetType(), apipb.CONDITION_STATUS_FALSE, err.Error())
			return fmt.Errorf("rollout failed at %s: %w", actor.GetType(), err)
		}
		
		p.setCondition(deployment, actor.GetType(), apipb.CONDITION_STATUS_TRUE, "Completed successfully")
		logger.Info("Actor completed successfully", "type", actor.GetType())
	}
	
	logger.Info("OSS rollout process completed successfully")
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
			Type:                 actorType,
			Status:               status,
			LastUpdatedTimestamp: now,
			Message:              message,
		}
		deployment.Status.Conditions = append(deployment.Status.Conditions, newCondition)
	} else {
		// Update existing condition
		condition.Status = status
		condition.LastUpdatedTimestamp = now
		condition.Message = message
	}
}

// ValidationActor validates the deployment configuration
type ValidationActor struct {
	gateway inferenceserver.Gateway
}

func (a *ValidationActor) GetType() string {
	return common.ActorTypeValidation
}

func (a *ValidationActor) Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("Validating deployment configuration")
	
	// Validate desired revision
	if deployment.Spec.DesiredRevision == nil {
		return fmt.Errorf("no desired revision specified")
	}
	
	// Validate inference server
	if deployment.Spec.GetInferenceServer() == nil {
		return fmt.Errorf("no inference server specified")
	}
	
	logger.Info("Deployment validation completed successfully")
	return nil
}

// ResourcePreparationActor prepares resources for deployment
type ResourcePreparationActor struct {
	gateway inferenceserver.Gateway
}

func (a *ResourcePreparationActor) GetType() string {
	return common.ActorTypeResourcePrep
}

func (a *ResourcePreparationActor) Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("Preparing resources for deployment")
	
	// Set candidate revision
	deployment.Status.CandidateRevision = deployment.Spec.DesiredRevision
	
	logger.Info("Resource preparation completed successfully")
	return nil
}

// ModelLoadActor loads the model onto the inference server
type ModelLoadActor struct {
	gateway inferenceserver.Gateway
}

func (a *ModelLoadActor) GetType() string {
	return common.ActorTypeModelLoad
}

func (a *ModelLoadActor) Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("Loading model onto inference server")
	
	inferenceServerName := common.GetInferenceServerName(*deployment)
	if inferenceServerName == "" {
		return fmt.Errorf("no inference server name specified")
	}
	
	modelConfig := common.BuildModelConfig(*deployment)
	
	// Create model load request
	loadRequest := inferenceserver.ModelLoadRequest{
		ModelName:       deployment.Spec.DesiredRevision.Name,
		ModelVersion:    "latest",
		PackagePath:     common.GetModelArtifactPath(*deployment),
		InferenceServer: inferenceServerName,
		BackendType:     v2pb.BACKEND_TYPE_TRITON, // OSS uses Triton backend
		Config:          modelConfig,
	}
	
	err := a.gateway.LoadModel(ctx, logger, loadRequest)
	if err != nil {
		return fmt.Errorf("failed to load model: %w", err)
	}
	
	logger.Info("Model loading completed successfully")
	return nil
}

// HealthCheckActor verifies the model is loaded and healthy
type HealthCheckActor struct {
	gateway inferenceserver.Gateway
}

func (a *HealthCheckActor) GetType() string {
	return common.ActorTypeHealthCheck
}

func (a *HealthCheckActor) Execute(ctx context.Context, logger logr.Logger, deployment *v2pb.Deployment) error {
	logger.Info("Performing health check on loaded model")
	
	inferenceServerName := common.GetInferenceServerName(*deployment)
	if inferenceServerName == "" {
		return fmt.Errorf("no inference server name specified")
	}
	
	// Check model status
	statusRequest := inferenceserver.ModelStatusRequest{
		ModelName:       deployment.Spec.DesiredRevision.Name,
		ModelVersion:    "latest",
		InferenceServer: inferenceServerName,
		BackendType:     v2pb.BACKEND_TYPE_TRITON,
	}
	
	isLoaded, err := a.gateway.CheckModelStatus(ctx, logger, statusRequest)
	if err != nil {
		return fmt.Errorf("failed to check model status: %w", err)
	}
	
	if !isLoaded {
		return fmt.Errorf("model is not loaded or ready")
	}
	
	// Check inference server health
	isHealthy, err := a.gateway.IsHealthy(ctx, logger, inferenceServerName, v2pb.BACKEND_TYPE_TRITON)
	if err != nil {
		return fmt.Errorf("failed to check inference server health: %w", err)
	}
	
	if !isHealthy {
		return fmt.Errorf("inference server is not healthy")
	}
	
	logger.Info("Health check completed successfully")
	return nil
}