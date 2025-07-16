package gateways

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigMapProvider manages ConfigMaps for inference servers
type ConfigMapProvider struct {
	kubeClient client.Client
	logger     logr.Logger
}

// ModelConfigEntry represents a model configuration entry
type ModelConfigEntry struct {
	Name   string `json:"name"`
	S3Path string `json:"s3_path"`
}

// ConfigMapRequest contains information needed to create/update a ConfigMap
type ConfigMapRequest struct {
	InferenceServer string
	Namespace       string
	BackendType     v2pb.BackendType
	ModelConfigs    []ModelConfigEntry
	Labels          map[string]string
	Annotations     map[string]string
}

// NewConfigMapProvider creates a new ConfigMap provider
func NewConfigMapProvider(kubeClient client.Client, logger logr.Logger) *ConfigMapProvider {
	return &ConfigMapProvider{
		kubeClient: kubeClient,
		logger:     logger,
	}
}

// CreateModelConfigMap creates a ConfigMap for model configuration
func (p *ConfigMapProvider) CreateModelConfigMap(ctx context.Context, request ConfigMapRequest) error {
	configMapName := fmt.Sprintf("%s-model-config", request.InferenceServer)
	
	p.logger.Info("Creating model ConfigMap", "configMap", configMapName, "namespace", request.Namespace, "backend", request.BackendType)

	// Check if ConfigMap already exists
	existing := &corev1.ConfigMap{}
	err := p.kubeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, existing)
	if err == nil {
		p.logger.Info("ConfigMap already exists, skipping creation", "name", configMapName)
		return nil
	}

	// Build model list JSON
	modelListJSON, err := json.MarshalIndent(request.ModelConfigs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal model configs: %w", err)
	}

	// Prepare labels
	labels := map[string]string{
		"app.kubernetes.io/component":  "model-config",
		"app.kubernetes.io/part-of":    "michelangelo",
		"michelangelo.ai/backend-type": request.BackendType.String(),
		"michelangelo.ai/inference-server": request.InferenceServer,
	}
	
	// Add custom labels
	for k, v := range request.Labels {
		labels[k] = v
	}

	// Create ConfigMap
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        configMapName,
			Namespace:   request.Namespace,
			Labels:      labels,
			Annotations: request.Annotations,
		},
		Data: map[string]string{
			"model-list.json": string(modelListJSON),
		},
	}

	if err := p.kubeClient.Create(ctx, configMap); err != nil {
		return fmt.Errorf("failed to create ConfigMap: %w", err)
	}

	p.logger.Info("Model ConfigMap created successfully", "configMap", configMapName, "modelCount", len(request.ModelConfigs))
	return nil
}

// UpdateModelConfigMap updates an existing ConfigMap with new model configurations
func (p *ConfigMapProvider) UpdateModelConfigMap(ctx context.Context, request ConfigMapRequest) error {
	configMapName := fmt.Sprintf("%s-model-config", request.InferenceServer)
	
	p.logger.Info("Updating model ConfigMap", "configMap", configMapName, "namespace", request.Namespace)

	// Get existing ConfigMap
	configMap := &corev1.ConfigMap{}
	err := p.kubeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, configMap)
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap %s: %w", configMapName, err)
	}

	// Build updated model list JSON
	modelListJSON, err := json.MarshalIndent(request.ModelConfigs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal model configs: %w", err)
	}

	// Update the ConfigMap data
	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}
	configMap.Data["model-list.json"] = string(modelListJSON)

	// Update labels if provided
	if len(request.Labels) > 0 {
		if configMap.Labels == nil {
			configMap.Labels = make(map[string]string)
		}
		for k, v := range request.Labels {
			configMap.Labels[k] = v
		}
	}

	// Update annotations if provided
	if len(request.Annotations) > 0 {
		if configMap.Annotations == nil {
			configMap.Annotations = make(map[string]string)
		}
		for k, v := range request.Annotations {
			configMap.Annotations[k] = v
		}
	}

	// Apply the update
	if err := p.kubeClient.Update(ctx, configMap); err != nil {
		return fmt.Errorf("failed to update ConfigMap %s: %w", configMapName, err)
	}

	p.logger.Info("Model ConfigMap updated successfully", "configMap", configMapName, "modelCount", len(request.ModelConfigs))
	return nil
}

// DeleteModelConfigMap deletes a ConfigMap for model configuration
func (p *ConfigMapProvider) DeleteModelConfigMap(ctx context.Context, inferenceServerName, namespace string) error {
	configMapName := fmt.Sprintf("%s-model-config", inferenceServerName)
	
	p.logger.Info("Deleting model ConfigMap", "configMap", configMapName, "namespace", namespace)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
	}

	if err := p.kubeClient.Delete(ctx, configMap); err != nil {
		return fmt.Errorf("failed to delete ConfigMap %s: %w", configMapName, err)
	}

	p.logger.Info("Model ConfigMap deleted successfully", "configMap", configMapName)
	return nil
}

// GetModelConfigMap retrieves a ConfigMap and parses its model configurations
func (p *ConfigMapProvider) GetModelConfigMap(ctx context.Context, inferenceServerName, namespace string) ([]ModelConfigEntry, error) {
	configMapName := fmt.Sprintf("%s-model-config", inferenceServerName)
	
	p.logger.Info("Getting model ConfigMap", "configMap", configMapName, "namespace", namespace)

	configMap := &corev1.ConfigMap{}
	err := p.kubeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap %s: %w", configMapName, err)
	}

	modelListJSON, exists := configMap.Data["model-list.json"]
	if !exists {
		p.logger.Info("No model-list.json found in ConfigMap", "configMap", configMapName)
		return []ModelConfigEntry{}, nil
	}

	var modelConfigs []ModelConfigEntry
	if err := json.Unmarshal([]byte(modelListJSON), &modelConfigs); err != nil {
		return nil, fmt.Errorf("failed to parse model-list.json from ConfigMap %s: %w", configMapName, err)
	}

	p.logger.Info("Model ConfigMap retrieved successfully", "configMap", configMapName, "modelCount", len(modelConfigs))
	return modelConfigs, nil
}

// CreateDefaultModelConfig creates a default model configuration for an inference server
func (p *ConfigMapProvider) CreateDefaultModelConfig(inferenceServerName, defaultModelName, defaultModelPath string) []ModelConfigEntry {
	return []ModelConfigEntry{
		{
			Name:   defaultModelName,
			S3Path: defaultModelPath,
		},
	}
}

// AddModelToConfig adds a new model to existing configuration
func (p *ConfigMapProvider) AddModelToConfig(ctx context.Context, inferenceServerName, namespace, modelName, modelPath string) error {
	// Get current config
	currentConfigs, err := p.GetModelConfigMap(ctx, inferenceServerName, namespace)
	if err != nil {
		return fmt.Errorf("failed to get current model config: %w", err)
	}

	// Check if model already exists
	for _, config := range currentConfigs {
		if config.Name == modelName {
			// Update existing model path
			config.S3Path = modelPath
			break
		}
	}

	// Add new model if not found
	found := false
	for i, config := range currentConfigs {
		if config.Name == modelName {
			currentConfigs[i].S3Path = modelPath
			found = true
			break
		}
	}
	
	if !found {
		currentConfigs = append(currentConfigs, ModelConfigEntry{
			Name:   modelName,
			S3Path: modelPath,
		})
	}

	// Update ConfigMap
	request := ConfigMapRequest{
		InferenceServer: inferenceServerName,
		Namespace:       namespace,
		ModelConfigs:    currentConfigs,
	}

	return p.UpdateModelConfigMap(ctx, request)
}

// RemoveModelFromConfig removes a model from existing configuration
func (p *ConfigMapProvider) RemoveModelFromConfig(ctx context.Context, inferenceServerName, namespace, modelName string) error {
	// Get current config
	currentConfigs, err := p.GetModelConfigMap(ctx, inferenceServerName, namespace)
	if err != nil {
		return fmt.Errorf("failed to get current model config: %w", err)
	}

	// Remove model from config
	updatedConfigs := make([]ModelConfigEntry, 0, len(currentConfigs))
	for _, config := range currentConfigs {
		if config.Name != modelName {
			updatedConfigs = append(updatedConfigs, config)
		}
	}

	// Update ConfigMap
	request := ConfigMapRequest{
		InferenceServer: inferenceServerName,
		Namespace:       namespace,
		ModelConfigs:    updatedConfigs,
	}

	return p.UpdateModelConfigMap(ctx, request)
}