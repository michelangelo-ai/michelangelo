package rollout

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

type modelLoadingActor struct {
	client    client.Client
	logger    logr.Logger
	providers map[string]plugins.Provider
}

var _ plugins.ConditionActor = &modelLoadingActor{}

// GetType returns the actor type
func (a *modelLoadingActor) GetType() string {
	return "ModelLoaded"
}

// Run executes the model loading logic
func (a *modelLoadingActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, deployment *v2pb.Deployment, condition *v2pb.Condition) error {
	runtimeCtx.Logger.Info("Loading model on inference providers", "deployment", deployment.Name)
	
	// Get model configuration from ConfigMap
	modelConfig, err := a.getModelConfig(ctx, deployment)
	if err != nil {
		return fmt.Errorf("failed to get model config: %w", err)
	}
	
	// Determine target providers based on backend type
	targetProviders := a.getTargetProviders(deployment)
	
	// Load model on each target provider
	for _, providerType := range targetProviders {
		provider, exists := a.providers[providerType]
		if !exists {
			runtimeCtx.Logger.Info("Provider not available, skipping", "provider", providerType)
			continue
		}
		
		loadRequest := plugins.ModelLoadRequest{
			ModelName:     modelConfig.ModelName,
			ModelVersion:  modelConfig.ModelVersion,
			PackagePath:   modelConfig.PackagePath,
			Config:        modelConfig.Config,
			InferenceSpec: deployment.Spec.InferenceServer,
		}
		
		err = provider.LoadModel(ctx, loadRequest)
		if err != nil {
			runtimeCtx.Logger.Error(err, "Failed to load model on provider", "provider", providerType)
			return fmt.Errorf("failed to load model on %s: %w", providerType, err)
		}
		
		runtimeCtx.Logger.Info("Model loading initiated", "provider", providerType, "model", modelConfig.ModelName)
	}
	
	return nil
}

// Retrieve checks the status of model loading
func (a *modelLoadingActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, deployment *v2pb.Deployment, condition v2pb.Condition) (v2pb.Condition, error) {
	modelConfig, err := a.getModelConfig(ctx, deployment)
	if err != nil {
		return v2pb.Condition{
			Type:    condition.Type,
			Status:  v2pb.CONDITION_STATUS_FALSE,
			Message: fmt.Sprintf("Failed to get model config: %v", err),
			Reason:  "ConfigError",
		}, nil
	}
	
	targetProviders := a.getTargetProviders(deployment)
	var notReadyProviders []string
	var messages []string
	
	// Check model status on all target providers
	for _, providerType := range targetProviders {
		provider, exists := a.providers[providerType]
		if !exists {
			continue
		}
		
		status, err := provider.GetModelStatus(ctx, modelConfig.ModelName, modelConfig.ModelVersion)
		if err != nil {
			messages = append(messages, fmt.Sprintf("%s: error checking status - %v", providerType, err))
			notReadyProviders = append(notReadyProviders, providerType)
			continue
		}
		
		if status.State != "LOADED" || !status.Ready {
			messages = append(messages, fmt.Sprintf("%s: %s", providerType, status.State))
			notReadyProviders = append(notReadyProviders, providerType)
		} else {
			messages = append(messages, fmt.Sprintf("%s: ready", providerType))
		}
	}
	
	if len(notReadyProviders) > 0 {
		return v2pb.Condition{
			Type:    condition.Type,
			Status:  v2pb.CONDITION_STATUS_FALSE,
			Message: fmt.Sprintf("Models not ready on providers: %v", messages),
			Reason:  "ModelsLoading",
		}, nil
	}
	
	return v2pb.Condition{
		Type:    condition.Type,
		Status:  v2pb.CONDITION_STATUS_TRUE,
		Message: fmt.Sprintf("Models loaded on all providers: %v", messages),
		Reason:  "ModelsReady",
	}, nil
}

// ModelConfig represents the configuration needed for model loading
type ModelConfig struct {
	ModelName    string
	ModelVersion string
	PackagePath  string
	Config       string
	ModelType    string
}

// getModelConfig retrieves model configuration from ConfigMap
func (a *modelLoadingActor) getModelConfig(ctx context.Context, deployment *v2pb.Deployment) (*ModelConfig, error) {
	configMapName := fmt.Sprintf("%s-model-config", deployment.Spec.InferenceServer.Name)
	
	configMap := &corev1.ConfigMap{}
	err := a.client.Get(ctx, client.ObjectKey{
		Name:      configMapName,
		Namespace: deployment.Namespace,
	}, configMap)
	
	if err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap: %w", err)
	}
	
	return &ModelConfig{
		ModelName:    configMap.Data["model_name"],
		ModelVersion: configMap.Data["model_version"],
		PackagePath:  configMap.Data["package_path"],
		Config:       configMap.Data["model-list.json"],
		ModelType:    configMap.Data["model_type"],
	}, nil
}

// getTargetProviders determines which providers should be used for this deployment
func (a *modelLoadingActor) getTargetProviders(deployment *v2pb.Deployment) []string {
	if deployment.Spec.InferenceServer == nil {
		return []string{plugins.ProviderTypeTriton} // Default to Triton
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