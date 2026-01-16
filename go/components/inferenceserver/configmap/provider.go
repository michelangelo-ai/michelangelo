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
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/secrets"
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
func NewDefaultModelConfigMapProvider(kubeClient client.Client, logger *zap.Logger) *defaultModelConfigMapProvider {
	sp := secrets.NewProvider(kubeClient)
	cf := clientfactory.NewClientFactory(kubeClient, sp, kubeClient.Scheme(), logger)
	return &defaultModelConfigMapProvider{
		kubeClient:    kubeClient,
		clientFactory: cf,
		logger:        logger,
	}
}

// CreateModelConfigMap creates a ModelConfigMap for model configuration
func (p *defaultModelConfigMapProvider) CreateModelConfigMap(ctx context.Context, inferenceServer string, namespace string, modelConfigs []ModelConfigEntry, labels map[string]string, annotations map[string]string, targetCluster *v2pb.ClusterTarget) error {
	clusterClient, err := getClusterClientFromTargetCluster(ctx, targetCluster, p.clientFactory)
	if err != nil {
		return fmt.Errorf("failed to get cluster client: %w", err)
	}

	configMapName := generateConfigMapName(inferenceServer)

	p.logger.Info("Creating model ConfigMap",
		zap.String("configMap", configMapName),
		zap.String("namespace", namespace),
		zap.String("cluster", targetCluster.ClusterId))

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
		p.logger.Error("failed to marshal model configs",
			zap.Error(err),
			zap.String("operation", "create_configmap"),
			zap.String("namespace", namespace),
			zap.String("configMap", configMapName))
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
		p.logger.Error("failed to create ConfigMap",
			zap.Error(err),
			zap.String("operation", "create_configmap"),
			zap.String("namespace", namespace),
			zap.String("configMap", configMapName))
		return fmt.Errorf("failed to create ConfigMap %s/%s: %w",
			namespace, configMapName, err)
	}

	p.logger.Info("Model ConfigMap created successfully", zap.String("configMap", configMapName), zap.Int("modelCount", len(modelConfigs)))
	return nil
}

// GetModelsFromConfigMap retrieves a ConfigMap and parses its model configurations
func (p *defaultModelConfigMapProvider) GetModelsFromConfigMap(ctx context.Context, inferenceServer string, namespace string, targetCluster *v2pb.ClusterTarget) ([]ModelConfigEntry, error) {
	configMapName := generateConfigMapName(inferenceServer)
	p.logger.Info("Getting model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", namespace))

	clusterClient, err := getClusterClientFromTargetCluster(ctx, targetCluster, p.clientFactory)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster client: %w", err)
	}

	configMap := &corev1.ConfigMap{}
	err = clusterClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, configMap)
	if err != nil {
		p.logger.Error("failed to get ConfigMap",
			zap.Error(err),
			zap.String("operation", "get_configmap"),
			zap.String("namespace", namespace),
			zap.String("configMap", configMapName))
		return nil, fmt.Errorf("failed to get ConfigMap %s/%s: %w",
			namespace, configMapName, err)
	}

	modelConfigs, err := p.parseModelConfigsFromConfigMap(ctx, configMap)
	if err != nil {
		return nil, err
	}
	p.logger.Info("Model ConfigMap retrieved successfully", zap.String("configMap", configMapName), zap.Int("modelCount", len(modelConfigs)))
	return modelConfigs, nil
}

// AddModelToConfigMap adds a model to a ModelConfigMap
func (p *defaultModelConfigMapProvider) AddModelToConfigMap(ctx context.Context, inferenceServer string, namespace string, modelConfig ModelConfigEntry, targetCluster *v2pb.ClusterTarget) error {
	configMapName := generateConfigMapName(inferenceServer)
	p.logger.Info("Getting model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", namespace))

	clusterClient, err := getClusterClientFromTargetCluster(ctx, targetCluster, p.clientFactory)
	if err != nil {
		return fmt.Errorf("failed to get cluster client: %w", err)
	}

	configMap := &corev1.ConfigMap{}
	err = clusterClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, configMap)
	if err != nil {
		p.logger.Error("failed to get ConfigMap",
			zap.Error(err),
			zap.String("operation", "get_configmap"),
			zap.String("namespace", namespace),
			zap.String("configMap", configMapName))
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
	p.logger.Info("Model successfully added to ConfigMap", zap.String("configMap", configMapName), zap.Int("modelCount", len(currentConfigs)))
	return nil
}

// RemoveModelFromConfigMap removes a model from a ModelConfigMap
func (p *defaultModelConfigMapProvider) RemoveModelFromConfigMap(ctx context.Context, inferenceServer string, namespace string, modelName string, targetCluster *v2pb.ClusterTarget) error {
	configMapName := generateConfigMapName(inferenceServer)
	p.logger.Info("Getting model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", namespace))

	clusterClient, err := getClusterClientFromTargetCluster(ctx, targetCluster, p.clientFactory)
	if err != nil {
		return fmt.Errorf("failed to get cluster client: %w", err)
	}

	configMap := &corev1.ConfigMap{}
	err = clusterClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, configMap)
	if err != nil {
		p.logger.Error("failed to get ConfigMap",
			zap.Error(err),
			zap.String("operation", "get_configmap"),
			zap.String("namespace", namespace),
			zap.String("configMap", configMapName))
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
	p.logger.Info("Model successfully removed from ConfigMap", zap.String("configMap", configMapName), zap.Int("modelCount", len(updatedConfigs)))
	return nil
}

// DeleteModelConfigMap deletes a ConfigMap for model configuration
func (p *defaultModelConfigMapProvider) DeleteModelConfigMap(ctx context.Context, inferenceServer string, namespace string, targetCluster *v2pb.ClusterTarget) error {
	configMapName := generateConfigMapName(inferenceServer)
	p.logger.Info("Deleting model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", namespace))

	clusterClient, err := getClusterClientFromTargetCluster(ctx, targetCluster, p.clientFactory)
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
		p.logger.Error("failed to delete ConfigMap",
			zap.Error(err),
			zap.String("operation", "delete_configmap"),
			zap.String("namespace", namespace),
			zap.String("configMap", configMapName))
		return fmt.Errorf("failed to delete ConfigMap %s/%s: %w",
			namespace, configMapName, err)
	}

	p.logger.Info("Model ConfigMap deleted successfully", zap.String("configMap", configMapName))
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

	if err := clusterClient.Update(ctx, configMap); err != nil {
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

func getClusterClientFromTargetCluster(ctx context.Context, targetCluster *v2pb.ClusterTarget, clientFactory clientfactory.ClientFactory) (client.Client, error) {
	var clusterClient client.Client
	var err error
	switch targetCluster.GetConfig().(type) {
	case *v2pb.ClusterTarget_Kubernetes:
		connectionSpec := targetCluster.GetKubernetes()
		clusterClient, err = clientFactory.GetClient(ctx, connectionSpec)
		if err != nil {
			return nil, fmt.Errorf("failed to get client for cluster %s: %w", targetCluster.ClusterId, err)
		}
	default:
		return nil, fmt.Errorf("unsupported cluster type: %T", targetCluster.GetConfig())
	}
	return clusterClient, nil
}

func generateConfigMapName(inferenceServerName string) string {
	return fmt.Sprintf("%s-%s", inferenceServerName, modelConfigSuffix)
}
