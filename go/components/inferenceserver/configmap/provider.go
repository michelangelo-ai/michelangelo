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

	p.logger.Info("Creating model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", request.Namespace))

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

// GetModelsFromConfigMap retrieves a ConfigMap and parses its model configurations
func (p *defaultModelConfigMapProvider) GetModelsFromConfigMap(ctx context.Context, request GetModelConfigMapRequest) ([]ModelConfigEntry, error) {
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

	modelConfigs, err := p.parseModelConfigsFromConfigMap(ctx, configMap)
	if err != nil {
		return nil, err
	}
	p.logger.Info("Model ConfigMap retrieved successfully", zap.String("configMap", configMapName), zap.Int("modelCount", len(modelConfigs)))
	return modelConfigs, nil
}

// AddModelToConfigMap adds a model to a ModelConfigMap
func (p *defaultModelConfigMapProvider) AddModelToConfigMap(ctx context.Context, request AddModelToConfigMapRequest) error {
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
		return fmt.Errorf("failed to get ConfigMap %s/%s: %w",
			request.Namespace, configMapName, err)
	}

	currentConfigs, err := p.parseModelConfigsFromConfigMap(ctx, configMap)
	if err != nil {
		return err
	}

	// Add new model if not found
	found := false
	for i, config := range currentConfigs {
		if config.Name == request.ModelConfig.Name {
			currentConfigs[i].S3Path = request.ModelConfig.S3Path
			found = true
			break
		}
	}

	if !found {
		currentConfigs = append(currentConfigs, ModelConfigEntry{
			Name:   request.ModelConfig.Name,
			S3Path: request.ModelConfig.S3Path,
		})
	}

	// Update ConfigMap
	if err := p.updateConfigMapWithModels(ctx, configMap, currentConfigs); err != nil {
		return err
	}
	p.logger.Info("Model successfully added to ConfigMap", zap.String("configMap", configMapName), zap.Int("modelCount", len(currentConfigs)))
	return nil
}

// RemoveModelFromConfigMap removes a model from a ModelConfigMap
func (p *defaultModelConfigMapProvider) RemoveModelFromConfigMap(ctx context.Context, request RemoveModelFromConfigMapRequest) error {
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
		return fmt.Errorf("failed to get ConfigMap %s/%s: %w",
			request.Namespace, configMapName, err)
	}

	currentConfigs, err := p.parseModelConfigsFromConfigMap(ctx, configMap)
	if err != nil {
		return err
	}

	updatedConfigs := []ModelConfigEntry{}
	fmt.Printf("DEBUG: RemoveModelFromConfig: Current configs: %v\n", currentConfigs)
	for _, config := range currentConfigs {
		if config.Name != request.ModelName {
			updatedConfigs = append(updatedConfigs, config)
		}
	}
	fmt.Printf("DEBUG: RemoveModelFromConfig: Updated configs: %v\n", updatedConfigs)
	// Update ConfigMap
	if err := p.updateConfigMapWithModels(ctx, configMap, updatedConfigs); err != nil {
		return err
	}
	p.logger.Info("Model successfully removed from ConfigMap", zap.String("configMap", configMapName), zap.Int("modelCount", len(updatedConfigs)))
	return nil
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

func (p *defaultModelConfigMapProvider) updateConfigMapWithModels(ctx context.Context, configMap *corev1.ConfigMap, modelConfigs []ModelConfigEntry) error {
	// Initialize data map if needed
	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	// Marshal the updated model list with proper formatting
	modelListJSON, err := json.MarshalIndent(modelConfigs, "", "  ")
	if err != nil {
		p.logger.Error("failed to marshal model configs",
			zap.Error(err),
			zap.String("operation", "update_configmap"),
			zap.String("namespace", configMap.Namespace),
			zap.String("configMap", configMap.Name))
		return fmt.Errorf("failed to marshal model configs for ConfigMap %s/%s: %w",
			configMap.Namespace, configMap.Name, err)
	}

	// Update the ConfigMap data
	configMap.Data[modelListKey] = string(modelListJSON)

	// Apply the atomic update operation
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := p.kubeClient.Update(ctx, configMap); err != nil {
		p.logger.Error("failed to update ConfigMap",
			zap.Error(err),
			zap.String("operation", "update_configmap"),
			zap.String("namespace", configMap.Namespace),
			zap.String("configMap", configMap.Name))
		return fmt.Errorf("failed to update ConfigMap %s/%s: %w",
			configMap.Namespace, configMap.Name, err)
	}

	p.logger.Info("Model ConfigMap updated successfully", zap.String("configMap", configMap.Name), zap.Int("modelCount", len(modelConfigs)))
	return nil
}

func (p *defaultModelConfigMapProvider) parseModelConfigsFromConfigMap(ctx context.Context, configMap *corev1.ConfigMap) ([]ModelConfigEntry, error) {
	modelListJSON, exists := configMap.Data[modelListKey]
	if !exists {
		p.logger.Info("No model-list.json found in ConfigMap", zap.String("configMap", configMap.Name))
		return []ModelConfigEntry{}, nil
	}

	var modelConfigs []ModelConfigEntry
	if err := json.Unmarshal([]byte(modelListJSON), &modelConfigs); err != nil {
		p.logger.Error("failed to parse model-list.json from ConfigMap",
			zap.Error(err),
			zap.String("operation", "get_configmap"),
			zap.String("namespace", configMap.Namespace),
			zap.String("configMap", configMap.Name))
		return nil, fmt.Errorf("failed to parse model-list.json from ConfigMap %s/%s: %w",
			configMap.Namespace, configMap.Name, err)
	}

	return modelConfigs, nil
}
