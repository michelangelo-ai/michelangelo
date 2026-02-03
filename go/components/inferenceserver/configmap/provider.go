package configmap

// TODO(#621): ghosharitra: There's only one configmap per inference server and all deployments need to concurrently access these configmaps.
// Add appropriate locking mechanisms to ensure data consistency.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	modelListKey      = "model-list.json"
	modelConfigSuffix = "model-config"
)

var _ ModelConfigMapProvider = &defaultModelConfigMapProvider{} // ensure implementation satisfies interface

// defaultModelConfigMapProvider implements the ModelConfigMapProvider interface.
type defaultModelConfigMapProvider struct {
	kubeClient    client.Client
	clientFactory clientfactory.ClientFactory
	logger        *zap.Logger
}

// NewDefaultModelConfigMapProvider creates a new defaultModelConfigMapProvider instance
func NewDefaultModelConfigMapProvider(kubeClient client.Client, clientFactory clientfactory.ClientFactory, logger *zap.Logger) *defaultModelConfigMapProvider {
	return &defaultModelConfigMapProvider{
		kubeClient:    kubeClient,
		clientFactory: clientFactory,
		logger:        logger,
	}
}

// CreateModelConfigMap creates a ModelConfigMap for model configuration
func (p *defaultModelConfigMapProvider) CreateModelConfigMap(ctx context.Context, inferenceServer string, namespace string, modelConfigs []ModelConfigEntry, labels map[string]string, annotations map[string]string, targetCluster *v2pb.ClusterTarget) error {
	clusterClient, err := p.clientFactory.GetClient(ctx, targetCluster)
	if err != nil {
		return fmt.Errorf("failed to get cluster client: %w", err)
	}

	configMapName := generateConfigMapName(inferenceServer)

	// Check if ConfigMap already exists in the target cluster
	existing := &corev1.ConfigMap{}
	err = clusterClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, existing)
	if err == nil {
		p.logger.Info("ConfigMap already exists, skipping creation", zap.String("name", configMapName))
		return nil
	}

	// Build model list JSON
	modelListJSON, err := json.MarshalIndent(modelConfigs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal model configs for ConfigMap %s/%s: %w",
			namespace, configMapName, err)
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
		Data: map[string]string{
			modelListKey: string(modelListJSON),
		},
	}

	if err := clusterClient.Create(ctx, configMap); err != nil {
		return fmt.Errorf("failed to create ConfigMap %s/%s: %w",
			namespace, configMapName, err)
	}
	return nil
}

// GetModelsFromConfigMap retrieves a ConfigMap and parses its model configurations
func (p *defaultModelConfigMapProvider) GetModelsFromConfigMap(ctx context.Context, inferenceServer string, namespace string, targetCluster *v2pb.ClusterTarget) ([]ModelConfigEntry, error) {
	configMapName := generateConfigMapName(inferenceServer)
	clusterClient, err := p.clientFactory.GetClient(ctx, targetCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster client: %w", err)
	}

	configMap := &corev1.ConfigMap{}
	err = clusterClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap %s/%s: %w",
			namespace, configMapName, err)
	}

	modelConfigs, err := p.parseModelConfigsFromConfigMap(ctx, configMap)
	if err != nil {
		return nil, err
	}
	return modelConfigs, nil
}

// AddModelToConfigMap adds a model to a ModelConfigMap
func (p *defaultModelConfigMapProvider) AddModelToConfigMap(ctx context.Context, inferenceServer string, namespace string, modelConfig ModelConfigEntry, targetCluster *v2pb.ClusterTarget) error {
	configMapName := generateConfigMapName(inferenceServer)
	clusterClient, err := p.clientFactory.GetClient(ctx, targetCluster)
	if err != nil {
		return fmt.Errorf("failed to get cluster client: %w", err)
	}

	configMap := &corev1.ConfigMap{}
	err = clusterClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, configMap)
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap %s/%s: %w",
			namespace, configMapName, err)
	}

	currentConfigs, err := p.parseModelConfigsFromConfigMap(ctx, configMap)
	if err != nil {
		return err
	}

	// Add new model if not found
	found := false
	for i, config := range currentConfigs {
		if config.Name == modelConfig.Name {
			currentConfigs[i].StoragePath = modelConfig.StoragePath
			found = true
			break
		}
	}

	if !found {
		currentConfigs = append(currentConfigs, ModelConfigEntry{
			Name:        modelConfig.Name,
			StoragePath: modelConfig.StoragePath,
		})
	}

	// Update ConfigMap
	if err := p.updateConfigMapWithModels(ctx, clusterClient, configMap, currentConfigs); err != nil {
		return err
	}
	return nil
}

// RemoveModelFromConfigMap removes a model from a ModelConfigMap
func (p *defaultModelConfigMapProvider) RemoveModelFromConfigMap(ctx context.Context, inferenceServer string, namespace string, modelName string, targetCluster *v2pb.ClusterTarget) error {
	configMapName := generateConfigMapName(inferenceServer)
	clusterClient, err := p.clientFactory.GetClient(ctx, targetCluster)
	if err != nil {
		return fmt.Errorf("failed to get cluster client: %w", err)
	}

	configMap := &corev1.ConfigMap{}
	err = clusterClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, configMap)
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap %s/%s: %w",
			namespace, configMapName, err)
	}

	currentConfigs, err := p.parseModelConfigsFromConfigMap(ctx, configMap)
	if err != nil {
		return err
	}

	updatedConfigs := []ModelConfigEntry{}
	for _, config := range currentConfigs {
		if config.Name != modelName {
			updatedConfigs = append(updatedConfigs, config)
		}
	}

	// Update ConfigMap
	if err := p.updateConfigMapWithModels(ctx, clusterClient, configMap, updatedConfigs); err != nil {
		return err
	}
	return nil
}

// DeleteModelConfigMap deletes a ConfigMap for model configuration
func (p *defaultModelConfigMapProvider) DeleteModelConfigMap(ctx context.Context, inferenceServer string, namespace string, targetCluster *v2pb.ClusterTarget) error {
	configMapName := generateConfigMapName(inferenceServer)
	clusterClient, err := p.clientFactory.GetClient(ctx, targetCluster)
	if err != nil {
		return fmt.Errorf("failed to get cluster client: %w", err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
	}
	if err := clusterClient.Delete(ctx, configMap); err != nil {
		return fmt.Errorf("failed to delete ConfigMap %s/%s: %w",
			namespace, configMapName, err)
	}
	return nil
}

func (p *defaultModelConfigMapProvider) updateConfigMapWithModels(ctx context.Context, clusterClient client.Client, configMap *corev1.ConfigMap, modelConfigs []ModelConfigEntry) error {
	// Initialize data map if needed
	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	// Marshal the updated model list with proper formatting
	modelListJSON, err := json.MarshalIndent(modelConfigs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal model configs for ConfigMap %s/%s: %w",
			configMap.Namespace, configMap.Name, err)
	}

	// Update the ConfigMap data
	configMap.Data[modelListKey] = string(modelListJSON)

	// Apply the atomic update operation
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := clusterClient.Update(ctx, configMap); err != nil {
		return fmt.Errorf("failed to update ConfigMap %s/%s: %w",
			configMap.Namespace, configMap.Name, err)
	}
	return nil
}

func (p *defaultModelConfigMapProvider) parseModelConfigsFromConfigMap(ctx context.Context, configMap *corev1.ConfigMap) ([]ModelConfigEntry, error) {
	modelListJSON, exists := configMap.Data[modelListKey]
	if !exists {
		return []ModelConfigEntry{}, nil
	}

	var modelConfigs []ModelConfigEntry
	if err := json.Unmarshal([]byte(modelListJSON), &modelConfigs); err != nil {
		return nil, fmt.Errorf("failed to parse model-list.json from ConfigMap %s/%s: %w",
			configMap.Namespace, configMap.Name, err)
	}

	return modelConfigs, nil
}

func generateConfigMapName(inferenceServerName string) string {
	return fmt.Sprintf("%s-%s", inferenceServerName, modelConfigSuffix)
}
