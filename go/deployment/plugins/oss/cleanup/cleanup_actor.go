package cleanup

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

type cleanupActor struct {
	client    client.Client
	logger    logr.Logger
	providers map[string]plugins.Provider
}

var _ plugins.ConditionActor = &cleanupActor{}

// GetType returns the actor type
func (a *cleanupActor) GetType() string {
	return "CleanupComplete"
}

// Run executes the cleanup logic
func (a *cleanupActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, deployment *v2pb.Deployment, condition *v2pb.Condition) error {
	runtimeCtx.Logger.Info("Executing cleanup for deployment", "deployment", deployment.Name)

	// Cleanup tasks:
	// 1. Unload models from providers
	// 2. Remove ConfigMaps
	// 3. Clean up any temporary resources
	// 4. Clear deployment status

	// 1. Unload models from all providers
	err := a.unloadModelsFromProviders(ctx, deployment)
	if err != nil {
		runtimeCtx.Logger.Error(err, "Failed to unload models from providers")
		return err
	}

	// 2. Remove ConfigMap
	err = a.removeConfigMap(ctx, deployment)
	if err != nil {
		runtimeCtx.Logger.Error(err, "Failed to remove ConfigMap")
		return err
	}

	// 3. Clear deployment status
	deployment.Status.CurrentRevision = nil
	deployment.Status.CandidateRevision = nil
	deployment.Status.Conditions = nil
	deployment.Status.Stage = v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE
	deployment.Status.State = v2pb.DEPLOYMENT_STATE_EMPTY
	deployment.Status.Message = "Cleanup completed successfully"

	runtimeCtx.Logger.Info("Cleanup completed successfully", "deployment", deployment.Name)
	return nil
}

// Retrieve checks the status of the cleanup
func (a *cleanupActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, deployment *v2pb.Deployment, condition v2pb.Condition) (v2pb.Condition, error) {
	// Check if cleanup is complete
	if deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE {
		return v2pb.Condition{
			Type:    condition.Type,
			Status:  v2pb.CONDITION_STATUS_TRUE,
			Message: "Cleanup completed successfully",
			Reason:  "CleanupComplete",
		}, nil
	}

	// Check if ConfigMap still exists
	configMapName := fmt.Sprintf("%s-model-config", deployment.Spec.InferenceServer.Name)
	configMap := &corev1.ConfigMap{}
	err := a.client.Get(ctx, client.ObjectKey{
		Name:      configMapName,
		Namespace: deployment.Namespace,
	}, configMap)

	if err == nil {
		// ConfigMap still exists, cleanup not complete
		return v2pb.Condition{
			Type:    condition.Type,
			Status:  v2pb.CONDITION_STATUS_FALSE,
			Message: "ConfigMap still exists",
			Reason:  "CleanupInProgress",
		}, nil
	}

	return v2pb.Condition{
		Type:    condition.Type,
		Status:  v2pb.CONDITION_STATUS_FALSE,
		Message: "Cleanup in progress",
		Reason:  "CleanupInProgress",
	}, nil
}

// unloadModelsFromProviders removes models from all target providers
func (a *cleanupActor) unloadModelsFromProviders(ctx context.Context, deployment *v2pb.Deployment) error {
	if deployment.Status.CurrentRevision == nil {
		return nil // No model to unload
	}

	targetProviders := a.getTargetProviders(deployment)

	for _, providerType := range targetProviders {
		provider, exists := a.providers[providerType]
		if !exists {
			continue
		}

		err := provider.UnloadModel(ctx, deployment.Status.CurrentRevision.Name, "latest")
		if err != nil {
			a.logger.Error(err, "Failed to unload model from provider", "provider", providerType)
			return err
		}

		a.logger.Info("Model unloaded from provider", "provider", providerType, "model", deployment.Status.CurrentRevision.Name)
	}

	return nil
}

// removeConfigMap deletes the deployment's ConfigMap
func (a *cleanupActor) removeConfigMap(ctx context.Context, deployment *v2pb.Deployment) error {
	configMapName := fmt.Sprintf("%s-model-config", deployment.Spec.InferenceServer.Name)

	configMap := &corev1.ConfigMap{}
	err := a.client.Get(ctx, client.ObjectKey{
		Name:      configMapName,
		Namespace: deployment.Namespace,
	}, configMap)

	if err != nil {
		// ConfigMap already doesn't exist
		return nil
	}

	err = a.client.Delete(ctx, configMap)
	if err != nil {
		return fmt.Errorf("failed to delete ConfigMap: %w", err)
	}

	a.logger.Info("ConfigMap deleted successfully", "configMap", configMapName)
	return nil
}

// getTargetProviders determines which providers should be used for this deployment
func (a *cleanupActor) getTargetProviders(deployment *v2pb.Deployment) []string {
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