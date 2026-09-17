package modelconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/common/keyedmutex"
)

const (
	modelListKey      = "model-list.json"
	modelConfigSuffix = "model-config"
)

var _ ModelConfigProvider = &defaultModelConfigProvider{} // ensure implementation satisfies interface

// defaultModelConfigProvider implements the ModelConfigProvider interface through a backing store of ConfigMaps.
type defaultModelConfigProvider struct {
	// mu serializes read-modify-write cycles per ConfigMap. See mutateModels.
	mu *keyedmutex.Map
}

// NewDefaultModelConfigProvider creates a new defaultModelConfigProvider instance
func NewDefaultModelConfigProvider() *defaultModelConfigProvider {
	return &defaultModelConfigProvider{mu: keyedmutex.New()}
}

// CreateModelConfigMap creates a ModelConfigMap for model configuration
func (p *defaultModelConfigProvider) CreateModelConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServer string, namespace string, labels map[string]string, annotations map[string]string) error {
	configMapName := generateConfigMapName(inferenceServer)

	logger.Info("Creating model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", namespace))

	// Check if ConfigMap already exists
	existing := &corev1.ConfigMap{}
	err := kubeclient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, existing)
	if err == nil {
		logger.Info("ConfigMap already exists, skipping creation", zap.String("name", configMapName))
		return nil
	}

	// Prepare labels
	newLabels := map[string]string{
		"app.kubernetes.io/component":      "model-config",
		"app.kubernetes.io/part-of":        "michelangelo",
		"michelangelo.ai/inference-server": inferenceServer,
	}

	// Add custom labels
	for k, v := range labels {
		newLabels[k] = v
	}

	// Create ConfigMap
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        configMapName,
			Namespace:   namespace,
			Labels:      newLabels,
			Annotations: annotations,
		},
		Data: make(map[string]string),
	}

	if err := kubeclient.Create(ctx, configMap); err != nil {
		logger.Error("failed to create ConfigMap",
			zap.Error(err),
			zap.String("operation", "create_modelconfig"),
			zap.String("namespace", namespace),
			zap.String("configMap", configMapName))
		return fmt.Errorf("failed to create ConfigMap %s/%s: %w",
			namespace, configMapName, err)
	}

	logger.Info("Model ConfigMap created successfully", zap.String("configMap", configMapName))
	return nil
}

// CheckModelConfigExists checks if a model config exists for an inference server.
func (p *defaultModelConfigProvider) CheckModelConfigExists(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServer string, namespace string) (bool, error) {
	configMapName := generateConfigMapName(inferenceServer)

	configMap := &corev1.ConfigMap{}
	if err := kubeclient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, configMap); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		logger.Error("failed to check model config existence",
			zap.Error(err),
			zap.String("operation", "model_config_exists"),
			zap.String("namespace", namespace),
			zap.String("configMap", configMapName))
		return false, fmt.Errorf("failed to check model config existence %s/%s: %w", namespace, configMapName, err)
	}

	return true, nil
}

// GetModelsFromConfig retrieves all models from a configmap.
func (p *defaultModelConfigProvider) GetModelsFromConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServer string, namespace string) ([]ModelConfigEntry, error) {
	configMapName := generateConfigMapName(inferenceServer)

	logger.Info("Getting model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", namespace))

	configMap := &corev1.ConfigMap{}
	err := kubeclient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, configMap)
	if err != nil {
		logger.Error("failed to get ConfigMap",
			zap.Error(err),
			zap.String("operation", "get_modelconfig"),
			zap.String("namespace", namespace),
			zap.String("configMap", configMapName))
		return nil, fmt.Errorf("failed to get ConfigMap %s/%s: %w",
			namespace, configMapName, err)
	}

	modelConfigs, err := p.parseModelConfigsFromConfigMap(ctx, logger, kubeclient, configMap)
	if err != nil {
		return nil, err
	}
	logger.Info("Model ConfigMap retrieved successfully", zap.String("configMap", configMapName), zap.Int("modelCount", len(modelConfigs)))
	return modelConfigs, nil
}

// AddModelToConfig adds a model to a ConfigMap
func (p *defaultModelConfigProvider) AddModelToConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServer string, namespace string, entry ModelConfigEntry) error {
	return p.mutateModels(ctx, logger, kubeclient, inferenceServer, namespace, func(currentConfigs []ModelConfigEntry) []ModelConfigEntry {
		// Entries are keyed by (deployment, model): refresh this deployment's entry only.
		for i, config := range currentConfigs {
			if config.Name == entry.Name && config.DeploymentName == entry.DeploymentName {
				currentConfigs[i].StoragePath = entry.StoragePath
				return currentConfigs
			}
		}
		return append(currentConfigs, entry)
	})
}

// RemoveModelFromConfig removes the named deployment's entry for a model from a configmap.
func (p *defaultModelConfigProvider) RemoveModelFromConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServer string, namespace string, deploymentName string, modelName string) error {
	return p.mutateModels(ctx, logger, kubeclient, inferenceServer, namespace, func(currentConfigs []ModelConfigEntry) []ModelConfigEntry {
		updatedConfigs := []ModelConfigEntry{}
		for _, config := range currentConfigs {
			if !ownsEntry(config, deploymentName, modelName) {
				updatedConfigs = append(updatedConfigs, config)
			}
		}
		logger.Info("Removing model from ConfigMap", zap.String("deployment", deploymentName), zap.String("model", modelName), zap.Int("modelCount", len(updatedConfigs)))
		return updatedConfigs
	})
}

// DeleteModelConfig deletes a configmap for model configuration.
func (p *defaultModelConfigProvider) DeleteModelConfig(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServer string, namespace string) error {
	configMapName := generateConfigMapName(inferenceServer)

	logger.Info("Deleting model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", namespace))

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
	}

	if err := kubeclient.Delete(ctx, configMap); err != nil {
		logger.Error("failed to delete ConfigMap",
			zap.Error(err),
			zap.String("operation", "delete_modelconfig"),
			zap.String("namespace", namespace),
			zap.String("configMap", configMapName))
		return fmt.Errorf("failed to delete ConfigMap %s/%s: %w",
			namespace, configMapName, err)
	}

	logger.Info("Model ConfigMap deleted successfully", zap.String("configMap", configMapName))
	return nil
}

// mutateModels applies mutate to an inference server's model list and writes the result back.
//
// A single ConfigMap holds the entries for every deployment on an inference server. The lock
// serializes writers within this process; the retry re-reads and re-applies when an update is
// rejected for a stale resourceVersion.
func (p *defaultModelConfigProvider) mutateModels(ctx context.Context, logger *zap.Logger, kubeclient client.Client, inferenceServer string, namespace string, mutate func([]ModelConfigEntry) []ModelConfigEntry) error {
	configMapName := generateConfigMapName(inferenceServer)

	unlock := p.mu.Lock(namespace + "/" + configMapName)
	defer unlock()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		logger.Info("Getting model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", namespace))
		configMap := &corev1.ConfigMap{}
		if err := kubeclient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, configMap); err != nil {
			logger.Error("failed to get ConfigMap",
				zap.Error(err),
				zap.String("operation", "get_modelconfig"),
				zap.String("namespace", namespace),
				zap.String("configMap", configMapName))
			return fmt.Errorf("failed to get ConfigMap %s/%s: %w",
				namespace, configMapName, err)
		}

		currentConfigs, err := p.parseModelConfigsFromConfigMap(ctx, logger, kubeclient, configMap)
		if err != nil {
			return err
		}

		return p.updateConfigMapWithModels(ctx, logger, kubeclient, configMap, mutate(currentConfigs))
	})
}

func (p *defaultModelConfigProvider) updateConfigMapWithModels(ctx context.Context, logger *zap.Logger, kubeclient client.Client, configMap *corev1.ConfigMap, modelConfigs []ModelConfigEntry) error {
	// Initialize data map if needed
	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	// Marshal the updated model list with proper formatting
	modelListJSON, err := json.MarshalIndent(modelConfigs, "", "  ")
	if err != nil {
		logger.Error("failed to marshal model configs",
			zap.Error(err),
			zap.String("operation", "update_modelconfig"),
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

	if err := kubeclient.Update(ctx, configMap); err != nil {
		logger.Error("failed to update ConfigMap",
			zap.Error(err),
			zap.String("operation", "update_modelconfig"),
			zap.String("namespace", configMap.Namespace),
			zap.String("configMap", configMap.Name))
		return fmt.Errorf("failed to update ConfigMap %s/%s: %w",
			configMap.Namespace, configMap.Name, err)
	}

	logger.Info("Model ConfigMap updated successfully", zap.String("configMap", configMap.Name), zap.Int("modelCount", len(modelConfigs)))
	return nil
}

func (p *defaultModelConfigProvider) parseModelConfigsFromConfigMap(ctx context.Context, logger *zap.Logger, kubeclient client.Client, configMap *corev1.ConfigMap) ([]ModelConfigEntry, error) {
	modelListJSON, exists := configMap.Data[modelListKey]
	if !exists {
		logger.Info("No model-list.json found in ConfigMap", zap.String("configMap", configMap.Name))
		return []ModelConfigEntry{}, nil
	}

	var modelConfigs []ModelConfigEntry
	if err := json.Unmarshal([]byte(modelListJSON), &modelConfigs); err != nil {
		logger.Error("failed to parse model-list.json from ConfigMap",
			zap.Error(err),
			zap.String("operation", "get_modelconfig"),
			zap.String("namespace", configMap.Namespace),
			zap.String("configMap", configMap.Name))
		return nil, fmt.Errorf("failed to parse model-list.json from ConfigMap %s/%s: %w",
			configMap.Namespace, configMap.Name, err)
	}

	return modelConfigs, nil
}

func generateConfigMapName(inferenceServer string) string {
	return fmt.Sprintf("%s-%s", inferenceServer, modelConfigSuffix)
}

// ownsEntry reports whether an entry belongs to the named deployment's use of a model.
func ownsEntry(entry ModelConfigEntry, deploymentName string, modelName string) bool {
	return entry.Name == modelName && entry.DeploymentName == deploymentName
}
