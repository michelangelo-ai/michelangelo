package rollout

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

type configMapPreparationActor struct {
	client client.Client
	logger logr.Logger
}

var _ plugins.ConditionActor = &configMapPreparationActor{}

// GetType returns the actor type
func (a *configMapPreparationActor) GetType() string {
	return "ConfigMapReady"
}

// Run executes the ConfigMap preparation logic
func (a *configMapPreparationActor) Run(ctx context.Context, runtimeCtx plugins.RequestContext, deployment *v2pb.Deployment, condition *v2pb.Condition) error {
	runtimeCtx.Logger.Info("Preparing ConfigMap for deployment", "deployment", deployment.Name)
	
	// Get model information
	if deployment.Spec.DesiredRevision == nil {
		return fmt.Errorf("no desired revision specified")
	}
	
	var model v2pb.Model
	modelKey := client.ObjectKey{
		Namespace: deployment.Namespace,
		Name:      deployment.Spec.DesiredRevision.Name,
	}
	
	err := a.client.Get(ctx, modelKey, &model)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}
	
	// Create or update ConfigMap
	configMapName := fmt.Sprintf("%s-model-config", deployment.Spec.InferenceServer.Name)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: deployment.Namespace,
			Labels: map[string]string{
				"michelangelo.ai/deployment": deployment.Name,
				"michelangelo.ai/provider":   a.getProviderLabel(deployment),
			},
		},
		Data: map[string]string{
			"model-list.json": a.generateModelListJSON(&model),
			"model_name":      model.Name,
			"model_version":   "latest", // or extract from model spec
			"package_path":    a.getPackagePath(&model),
			"model_type":      a.getModelType(&model),
		},
	}
	
	// Try to create, if exists then update
	err = a.client.Create(ctx, configMap)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// Update existing ConfigMap
			existingConfigMap := &corev1.ConfigMap{}
			err = a.client.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: deployment.Namespace}, existingConfigMap)
			if err != nil {
				return fmt.Errorf("failed to get existing ConfigMap: %w", err)
			}
			
			// Update the data
			existingConfigMap.Data = configMap.Data
			err = a.client.Update(ctx, existingConfigMap)
			if err != nil {
				return fmt.Errorf("failed to update ConfigMap: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create ConfigMap: %w", err)
		}
	}
	
	runtimeCtx.Logger.Info("ConfigMap prepared successfully", "configMap", configMapName)
	return nil
}

// Retrieve checks the status of the ConfigMap preparation
func (a *configMapPreparationActor) Retrieve(ctx context.Context, runtimeCtx plugins.RequestContext, deployment *v2pb.Deployment, condition v2pb.Condition) (v2pb.Condition, error) {
	configMapName := fmt.Sprintf("%s-model-config", deployment.Spec.InferenceServer.Name)
	
	configMap := &corev1.ConfigMap{}
	err := a.client.Get(ctx, client.ObjectKey{
		Name:      configMapName,
		Namespace: deployment.Namespace,
	}, configMap)
	
	if err != nil {
		return v2pb.Condition{
			Type:    condition.Type,
			Status:  v2pb.CONDITION_STATUS_FALSE,
			Message: fmt.Sprintf("ConfigMap not found: %v", err),
			Reason:  "ConfigMapMissing",
		}, nil
	}
	
	// Validate required fields
	requiredFields := []string{"model-list.json", "model_name", "package_path"}
	for _, field := range requiredFields {
		if _, exists := configMap.Data[field]; !exists {
			return v2pb.Condition{
				Type:    condition.Type,
				Status:  v2pb.CONDITION_STATUS_FALSE,
				Message: fmt.Sprintf("ConfigMap missing required field: %s", field),
				Reason:  "ConfigMapIncomplete",
			}, nil
		}
	}
	
	return v2pb.Condition{
		Type:    condition.Type,
		Status:  v2pb.CONDITION_STATUS_TRUE,
		Message: "ConfigMap ready with all required fields",
		Reason:  "ConfigMapReady",
	}, nil
}

// Helper methods
func (a *configMapPreparationActor) getProviderLabel(deployment *v2pb.Deployment) string {
	if deployment.Spec.InferenceServer == nil {
		return "triton"
	}
	
	switch deployment.Spec.InferenceServer.BackendType {
	case v2pb.BACKEND_TYPE_TRITON:
		return "triton"
	case v2pb.BACKEND_TYPE_LLM_D:
		return "llm-d"
	case v2pb.BACKEND_TYPE_DYNAMO:
		return "dynamo"
	default:
		return "triton"
	}
}

func (a *configMapPreparationActor) generateModelListJSON(model *v2pb.Model) string {
	modelList := []map[string]string{
		{
			"name":    model.Name,
			"s3_path": a.getPackagePath(model),
		},
	}
	
	jsonBytes, err := json.MarshalIndent(modelList, "", "  ")
	if err != nil {
		a.logger.Error(err, "Failed to marshal model list")
		return "[]"
	}
	
	return string(jsonBytes)
}

func (a *configMapPreparationActor) getPackagePath(model *v2pb.Model) string {
	if len(model.Spec.DeployableArtifactUri) > 0 {
		return fmt.Sprintf("s3://deploy-models/%s", model.Spec.DeployableArtifactUri[0])
	}
	return ""
}

func (a *configMapPreparationActor) getModelType(model *v2pb.Model) string {
	// This could be extracted from model metadata or spec
	// For now, return a default or inferred type
	return "general"
}