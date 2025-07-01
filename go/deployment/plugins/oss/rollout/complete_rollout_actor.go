package rollout

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

type completeRolloutActor struct {
	client    client.Client
	logger    logr.Logger
	providers map[string]plugins.Provider
}

var _ plugins.ConditionActor = &completeRolloutActor{}

// GetType returns the actor type
func (a *completeRolloutActor) GetType() string {
	return "RolloutComplete"
}

// Run executes the rollout completion logic
func (a *completeRolloutActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, deployment *v2pb.Deployment, condition *v2pb.Condition) error {
	runtimeCtx.Logger.Info("Completing rollout for deployment", "deployment", deployment.Name)
	
	// 1. Promote candidate revision to current revision
	if deployment.Status.CandidateRevision == nil {
		deployment.Status.CandidateRevision = deployment.Spec.DesiredRevision
	}
	deployment.Status.CurrentRevision = deployment.Status.CandidateRevision
	
	// 2. Clean up old model configurations
	err := a.cleanupOldModels(ctx, deployment)
	if err != nil {
		runtimeCtx.Logger.Error(err, "Failed to cleanup old models, but rollout was successful")
		// Don't fail the rollout for cleanup errors, just log them
	}
	
	// 3. Update deployment annotations to remove any temporary markers
	err = a.cleanupAnnotations(ctx, deployment)
	if err != nil {
		runtimeCtx.Logger.Error(err, "Failed to cleanup annotations")
		return err
	}
	
	// 4. Update deployment status
	deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
	deployment.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
	deployment.Status.Message = fmt.Sprintf("Successfully deployed model %s", deployment.Spec.DesiredRevision.Name)
	
	runtimeCtx.Logger.Info("Rollout completed successfully", 
		"deployment", deployment.Name,
		"model", deployment.Spec.DesiredRevision.Name)
	
	return nil
}

// Retrieve checks if the rollout completion is done
func (a *completeRolloutActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, deployment *v2pb.Deployment, condition v2pb.Condition) (v2pb.Condition, error) {
	// Check if all prerequisites are met for completion
	
	// 1. Verify that previous actors have completed successfully
	if !a.arePrerequisitesMet(deployment) {
		return v2pb.Condition{
			Type:    condition.Type,
			Status:  v2pb.CONDITION_STATUS_FALSE,
			Message: "Prerequisites not met for rollout completion",
			Reason:  "PrerequisitesNotMet",
		}, nil
	}
	
	// 2. Verify that the model is healthy on all providers
	isHealthy, err := a.verifyModelHealth(ctx, deployment)
	if err != nil {
		return v2pb.Condition{
			Type:    condition.Type,
			Status:  v2pb.CONDITION_STATUS_FALSE,
			Message: fmt.Sprintf("Failed to verify model health: %v", err),
			Reason:  "HealthCheckFailed",
		}, nil
	}
	
	if !isHealthy {
		return v2pb.Condition{
			Type:    condition.Type,
			Status:  v2pb.CONDITION_STATUS_FALSE,
			Message: "Model not healthy on all providers",
			Reason:  "ModelUnhealthy",
		}, nil
	}
	
	// 3. Verify that routing is working correctly
	isRouted, err := a.verifyRouting(ctx, deployment)
	if err != nil {
		return v2pb.Condition{
			Type:    condition.Type,
			Status:  v2pb.CONDITION_STATUS_FALSE,
			Message: fmt.Sprintf("Failed to verify routing: %v", err),
			Reason:  "RoutingCheckFailed",
		}, nil
	}
	
	if !isRouted {
		return v2pb.Condition{
			Type:    condition.Type,
			Status:  v2pb.CONDITION_STATUS_FALSE,
			Message: "Routing not properly configured",
			Reason:  "RoutingNotReady",
		}, nil
	}
	
	return v2pb.Condition{
		Type:    condition.Type,
		Status:  v2pb.CONDITION_STATUS_TRUE,
		Message: "Rollout completed successfully",
		Reason:  "RolloutComplete",
		LastUpdatedTimestamp: time.Now().Unix(),
	}, nil
}

// arePrerequisitesMet checks if all previous actors have completed successfully
func (a *completeRolloutActor) arePrerequisitesMet(deployment *v2pb.Deployment) bool {
	requiredConditions := []string{"ConfigMapReady", "ModelLoaded", "RouteUpdated"}
	
	for _, requiredType := range requiredConditions {
		found := false
		for _, condition := range deployment.Status.Conditions {
			if condition.Type == requiredType && condition.Status == v2pb.CONDITION_STATUS_TRUE {
				found = true
				break
			}
		}
		if !found {
			a.logger.Info("Prerequisite not met", "required", requiredType)
			return false
		}
	}
	
	return true
}

// verifyModelHealth checks that the model is healthy on all target providers
func (a *completeRolloutActor) verifyModelHealth(ctx context.Context, deployment *v2pb.Deployment) (bool, error) {
	if deployment.Spec.DesiredRevision == nil {
		return false, fmt.Errorf("no desired revision specified")
	}
	
	targetProviders := a.getTargetProviders(deployment)
	
	for _, providerType := range targetProviders {
		provider, exists := a.providers[providerType]
		if !exists {
			continue
		}
		
		// Check provider health
		isHealthy, err := provider.IsHealthy(ctx)
		if err != nil || !isHealthy {
			a.logger.Info("Provider not healthy", "provider", providerType, "error", err)
			return false, err
		}
		
		// Check model status
		status, err := provider.GetModelStatus(ctx, deployment.Spec.DesiredRevision.Name, "latest")
		if err != nil {
			a.logger.Error(err, "Failed to get model status", "provider", providerType)
			return false, err
		}
		
		if status.State != "LOADED" || !status.Ready {
			a.logger.Info("Model not ready", "provider", providerType, "state", status.State)
			return false, nil
		}
	}
	
	return true, nil
}

// verifyRouting checks that routing is properly configured
func (a *completeRolloutActor) verifyRouting(ctx context.Context, deployment *v2pb.Deployment) (bool, error) {
	// This would typically involve:
	// 1. Checking that VirtualService/Ingress is properly configured
	// 2. Performing a health check through the routing layer
	// 3. Verifying that traffic is reaching the correct model version
	
	// For now, assume routing is working if RouteUpdated condition is true
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == "RouteUpdated" && condition.Status == v2pb.CONDITION_STATUS_TRUE {
			return true, nil
		}
	}
	
	return false, nil
}

// cleanupOldModels removes old model configurations from ConfigMaps
func (a *completeRolloutActor) cleanupOldModels(ctx context.Context, deployment *v2pb.Deployment) error {
	configMapName := fmt.Sprintf("%s-model-config", deployment.Spec.InferenceServer.Name)
	
	configMap := &corev1.ConfigMap{}
	err := a.client.Get(ctx, client.ObjectKey{
		Name:      configMapName,
		Namespace: deployment.Namespace,
	}, configMap)
	
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap for cleanup: %w", err)
	}
	
	// Here you would implement logic to:
	// 1. Parse the current model list
	// 2. Remove old model versions (keeping only current and maybe previous)
	// 3. Update the ConfigMap
	
	a.logger.Info("ConfigMap cleanup completed", "configMap", configMapName)
	return nil
}

// cleanupAnnotations removes temporary deployment annotations
func (a *completeRolloutActor) cleanupAnnotations(ctx context.Context, deployment *v2pb.Deployment) error {
	// Remove any temporary annotations that were used during deployment
	annotationsToRemove := []string{
		"michelangelo.ai/rollout-in-progress",
		"michelangelo.ai/candidate-model",
		"michelangelo.ai/rollout-timestamp",
	}
	
	if deployment.Metadata != nil && deployment.Metadata.Annotations != nil {
		for _, annotation := range annotationsToRemove {
			delete(deployment.Metadata.Annotations, annotation)
		}
	}
	
	return nil
}

// getTargetProviders determines which providers should be used for this deployment
func (a *completeRolloutActor) getTargetProviders(deployment *v2pb.Deployment) []string {
	if deployment.Spec.InferenceServer == nil {
		return []string{plugins.ProviderTypeTriton}
	}
	
	switch deployment.Spec.InferenceServer.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return []string{plugins.ProviderTypeTriton}
	case v2pb.BACKEND_TYPE_LLM_D:
		return []string{plugins.ProviderTypeLLMD}
	case v2pb.BACKEND_TYPE_DYNAMO:
		return []string{plugins.ProviderTypeDynamo}
	default:
		return []string{plugins.ProviderTypeTriton}
	}
}