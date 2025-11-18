package configmap

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	modelListKey      = "model-list.json"
	modelConfigSuffix = "model-config"
)

var _ ModelConfigMapProvider = &defaultModelConfigMapProvider{} // ensure implementation satisfies interface

var addSuffixToString = func(str, suffix string) string {
	return fmt.Sprintf("%s-%s", str, suffix)
}

// defaultModelConfigMapProvider implements the ModelConfigMapProvider interface.
type defaultModelConfigMapProvider struct {
	kubeClient client.Client
	logger     *zap.Logger
}

// NewDefaultModelConfigMapProvider creates a new defaultModelConfigMapProvider instance
func NewDefaultModelConfigMapProvider(client client.Client, logger *zap.Logger) *defaultModelConfigMapProvider {
	return &defaultModelConfigMapProvider{
		kubeClient: client,
		logger:     logger,
	}
}

// CreateModelConfigMap creates a ModelConfigMap for model configuration
func (p *defaultModelConfigMapProvider) CreateModelConfigMap(ctx context.Context, request CreateModelConfigMapRequest) error {
	configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)

	p.logger.Info("Creating model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", request.Namespace), zap.String("backend", request.BackendType.String()))

	// Check if ConfigMap already exists
	existing := &corev1.ConfigMap{}
	err := p.kubeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, existing)
	if err == nil {
		p.logger.Info("ConfigMap already exists, skipping creation", zap.String("name", configMapName))
		return nil
	}

	// Build model list JSON
	modelListJSON, err := json.MarshalIndent(request.ModelConfigs, "", "  ")
	if err != nil {
		p.logger.Error("failed to marshal model configs",
			zap.Error(err),
			zap.String("operation", "create_configmap"),
			zap.String("namespace", request.Namespace),
			zap.String("configMap", configMapName))
		return fmt.Errorf("failed to marshal model configs for ConfigMap %s/%s: %w",
			request.Namespace, configMapName, err)
	}

	// Prepare labels
	labels := map[string]string{
		"app.kubernetes.io/component":      "model-config",
		"app.kubernetes.io/part-of":        "michelangelo",
		"michelangelo.ai/backend-type":     request.BackendType.String(),
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
			modelListKey: string(modelListJSON),
		},
	}

	if err := p.kubeClient.Create(ctx, configMap); err != nil {
		p.logger.Error("failed to create ConfigMap",
			zap.Error(err),
			zap.String("operation", "create_configmap"),
			zap.String("namespace", request.Namespace),
			zap.String("configMap", configMapName))
		return fmt.Errorf("failed to create ConfigMap %s/%s: %w",
			request.Namespace, configMapName, err)
	}

	p.logger.Info("Model ConfigMap created successfully", zap.String("configMap", configMapName), zap.Int("modelCount", len(request.ModelConfigs)))
	return nil
}

// UpdateModelConfigMap updates an existing ConfigMap with new model configurations
func (p *defaultModelConfigMapProvider) UpdateModelConfigMap(ctx context.Context, request UpdateModelConfigMapRequest) error {
	configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)

	p.logger.Info("Updating model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", request.Namespace))

	// Get existing ConfigMap
	configMap := &corev1.ConfigMap{}
	err := p.kubeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, configMap)
	if err != nil {
		p.logger.Error("failed to get ConfigMap",
			zap.Error(err),
			zap.String("operation", "update_configmap"),
			zap.String("namespace", request.Namespace),
			zap.String("configMap", configMapName))
		return fmt.Errorf("failed to get ConfigMap %s/%s: %w",
			request.Namespace, configMapName, err)
	}

	// Initialize data map if needed
	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	// Parse existing model list if it exists
	var existingModelList []ModelConfigEntry
	if data, exists := configMap.Data[modelListKey]; exists && data != "" {
		if parseErr := json.Unmarshal([]byte(data), &existingModelList); parseErr != nil {
			p.logger.Error("failed to parse existing model list, starting fresh",
				zap.Error(parseErr),
				zap.String("operation", "update_configmap"),
				zap.String("namespace", request.Namespace),
				zap.String("configMap", configMapName))
			existingModelList = []ModelConfigEntry{}
		}
	}

	// Build updated model list based on request - this allows for atomic replacement or append operations
	var updatedModelList []ModelConfigEntry
	if len(request.ModelConfigs) > 0 {
		// Use the provided model configs (could be replacement or append)
		updatedModelList = request.ModelConfigs
	} else {
		// Keep existing models if no new configs provided
		updatedModelList = existingModelList
	}

	// Marshal the updated model list with proper formatting
	modelListJSON, err := json.MarshalIndent(updatedModelList, "", "  ")
	if err != nil {
		p.logger.Error("failed to marshal model configs",
			zap.Error(err),
			zap.String("operation", "update_configmap"),
			zap.String("namespace", request.Namespace),
			zap.String("configMap", configMapName))
		return fmt.Errorf("failed to marshal model configs for ConfigMap %s/%s: %w",
			request.Namespace, configMapName, err)
	}

	// Update the ConfigMap data
	configMap.Data[modelListKey] = string(modelListJSON)

	// Update labels if provided
	if len(request.Labels) > 0 {
		if configMap.Labels == nil {
			configMap.Labels = make(map[string]string)
		}
		for k, v := range request.Labels {
			configMap.Labels[k] = v
		}
	}

	// Apply the atomic update operation
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := p.kubeClient.Update(ctx, configMap); err != nil {
		p.logger.Error("failed to update ConfigMap",
			zap.Error(err),
			zap.String("operation", "update_configmap"),
			zap.String("namespace", request.Namespace),
			zap.String("configMap", configMapName))
		return fmt.Errorf("failed to update ConfigMap %s/%s: %w",
			request.Namespace, configMapName, err)
	}
	p.logger.Info("Model ConfigMap updated successfully", zap.String("configMap", configMapName), zap.Int("modelCount", len(updatedModelList)))
	return nil
}

// GetModelConfigMap retrieves a ConfigMap and parses its model configurations
func (p *defaultModelConfigMapProvider) GetModelConfigMap(ctx context.Context, request GetModelConfigMapRequest) ([]ModelConfigEntry, error) {
	configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)

	p.logger.Info("Getting model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", request.Namespace))

	configMap := &corev1.ConfigMap{}
	err := p.kubeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, configMap)
	if err != nil {
		p.logger.Error("failed to get ConfigMap",
			zap.Error(err),
			zap.String("operation", "get_configmap"),
			zap.String("namespace", request.Namespace),
			zap.String("configMap", configMapName))
		return nil, fmt.Errorf("failed to get ConfigMap %s/%s: %w",
			request.Namespace, configMapName, err)
	}

	modelListJSON, exists := configMap.Data[modelListKey]
	if !exists {
		p.logger.Info("No model-list.json found in ConfigMap", zap.String("configMap", configMapName))
		return []ModelConfigEntry{}, nil
	}

	var modelConfigs []ModelConfigEntry
	if err := json.Unmarshal([]byte(modelListJSON), &modelConfigs); err != nil {
		p.logger.Error("failed to parse model-list.json from ConfigMap",
			zap.Error(err),
			zap.String("operation", "get_configmap"),
			zap.String("namespace", request.Namespace),
			zap.String("configMap", configMapName))
		return nil, fmt.Errorf("failed to parse model-list.json from ConfigMap %s/%s: %w",
			request.Namespace, configMapName, err)
	}

	p.logger.Info("Model ConfigMap retrieved successfully", zap.String("configMap", configMapName), zap.Int("modelCount", len(modelConfigs)))
	return modelConfigs, nil
}

// DeleteModelConfigMap deletes a ConfigMap for model configuration
func (p *defaultModelConfigMapProvider) DeleteModelConfigMap(ctx context.Context, request DeleteModelConfigMapRequest) error {
	configMapName := addSuffixToString(request.InferenceServer, modelConfigSuffix)

	p.logger.Info("Deleting model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", request.Namespace))

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: request.Namespace,
		},
	}

	if err := p.kubeClient.Delete(ctx, configMap); err != nil {
		p.logger.Error("failed to delete ConfigMap",
			zap.Error(err),
			zap.String("operation", "delete_configmap"),
			zap.String("namespace", request.Namespace),
			zap.String("configMap", configMapName))
		return fmt.Errorf("failed to delete ConfigMap %s/%s: %w",
			request.Namespace, configMapName, err)
	}

	p.logger.Info("Model ConfigMap deleted successfully", zap.String("configMap", configMapName))
	return nil
}
