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

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ConfigMapProvider manages ConfigMaps for inference servers
type DefaultConfigMapProvider struct {
	kubeClient client.Client
	logger     *zap.Logger
}

// NewDefaultConfigMapProvider creates a new DefaultConfigMapProvider instance
func NewDefaultConfigMapProvider(client client.Client, logger *zap.Logger) *DefaultConfigMapProvider {
	return &DefaultConfigMapProvider{
		kubeClient: client,
		logger:     logger,
	}
}

// ModelConfigEntry represents a model configuration entry
type ModelConfigEntry struct {
	Name   string `json:"name"`
	S3Path string `json:"s3_path"`
}

// DeploymentModelRegistry tracks which models are used by which deployments (Uber UCS-style)
// This enables safe cleanup by only removing models not used by any deployment
type DeploymentModelRegistry struct {
	InferenceServer string                      `json:"inference_server"`
	Deployments     map[string]DeploymentModels `json:"deployments"` // deployment-name -> models
}

// DeploymentModels tracks the models for a specific deployment (mimics Uber's Current/Candidate/Shadow)
type DeploymentModels struct {
	Current   string `json:"current,omitempty"`
	Candidate string `json:"candidate,omitempty"`
	Shadow    string `json:"shadow,omitempty"`
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

// CreateModelConfigMap creates a ConfigMap for model configuration
func (p *DefaultConfigMapProvider) CreateModelConfigMap(ctx context.Context, request ConfigMapRequest) error {
	configMapName := fmt.Sprintf("%s-model-config", request.InferenceServer)

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
		return fmt.Errorf("failed to marshal model configs: %w", err)
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
			"model-list.json": string(modelListJSON),
		},
	}

	if err := p.kubeClient.Create(ctx, configMap); err != nil {
		return fmt.Errorf("failed to create ConfigMap: %w", err)
	}

	p.logger.Info("Model ConfigMap created successfully", zap.String("configMap", configMapName), zap.Int("modelCount", len(request.ModelConfigs)))
	return nil
}

// UpdateModelConfigMap updates an existing ConfigMap with new model configurations
// This follows the correct pattern from PR #188: Get -> Parse -> Modify -> Marshal -> Update
func (p *DefaultConfigMapProvider) UpdateModelConfigMap(ctx context.Context, request ConfigMapRequest) error {
	configMapName := fmt.Sprintf("%s-model-config", request.InferenceServer)

	p.logger.Info("Updating model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", request.Namespace))

	// Get existing ConfigMap
	configMap := &corev1.ConfigMap{}
	err := p.kubeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, configMap)
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap %s: %w", configMapName, err)
	}

	// Initialize data map if needed
	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}

	// Parse existing model list if it exists
	var existingModelList []ModelConfigEntry
	if data, exists := configMap.Data["model-list.json"]; exists && data != "" {
		if parseErr := json.Unmarshal([]byte(data), &existingModelList); parseErr != nil {
			p.logger.Error("Failed to parse existing model list, starting fresh", zap.Error(parseErr))
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
		return fmt.Errorf("failed to marshal model configs: %w", err)
	}

	// Update the ConfigMap data
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

	// Apply the atomic update operation
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := p.kubeClient.Update(ctx, configMap); err != nil {
		return fmt.Errorf("failed to update ConfigMap %s: %w", configMapName, err)
	}

	p.logger.Info("Model ConfigMap updated successfully", zap.String("configMap", configMapName), zap.Int("modelCount", len(updatedModelList)))
	return nil
}

// DeleteModelConfigMap deletes a ConfigMap for model configuration
func (p *DefaultConfigMapProvider) DeleteModelConfigMap(ctx context.Context, inferenceServerName, namespace string) error {
	configMapName := fmt.Sprintf("%s-model-config", inferenceServerName)

	p.logger.Info("Deleting model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", namespace))

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
	}

	if err := p.kubeClient.Delete(ctx, configMap); err != nil {
		return fmt.Errorf("failed to delete ConfigMap %s: %w", configMapName, err)
	}

	p.logger.Info("Model ConfigMap deleted successfully", zap.String("configMap", configMapName))
	return nil
}

// GetModelConfigMap retrieves a ConfigMap and parses its model configurations
func (p *DefaultConfigMapProvider) GetModelConfigMap(ctx context.Context, inferenceServerName, namespace string) ([]ModelConfigEntry, error) {
	configMapName := fmt.Sprintf("%s-model-config", inferenceServerName)

	p.logger.Info("Getting model ConfigMap", zap.String("configMap", configMapName), zap.String("namespace", namespace))

	configMap := &corev1.ConfigMap{}
	err := p.kubeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap %s: %w", configMapName, err)
	}

	modelListJSON, exists := configMap.Data["model-list.json"]
	if !exists {
		p.logger.Info("No model-list.json found in ConfigMap", zap.String("configMap", configMapName))
		return []ModelConfigEntry{}, nil
	}

	var modelConfigs []ModelConfigEntry
	if err := json.Unmarshal([]byte(modelListJSON), &modelConfigs); err != nil {
		return nil, fmt.Errorf("failed to parse model-list.json from ConfigMap %s: %w", configMapName, err)
	}

	p.logger.Info("Model ConfigMap retrieved successfully", zap.String("configMap", configMapName), zap.Int("modelCount", len(modelConfigs)))
	return modelConfigs, nil
}

// CreateDefaultModelConfig creates a default model configuration for an inference server
func (p *DefaultConfigMapProvider) CreateDefaultModelConfig(defaultModelName, defaultModelPath string) []ModelConfigEntry {
	return []ModelConfigEntry{
		{
			Name:   defaultModelName,
			S3Path: defaultModelPath,
		},
	}
}

// AddModelToConfig adds a new model to existing configuration
func (p *DefaultConfigMapProvider) AddModelToConfig(ctx context.Context, inferenceServerName, namespace, modelName, modelPath string) error {
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
func (p *DefaultConfigMapProvider) RemoveModelFromConfig(ctx context.Context, inferenceServerName, namespace, modelName string) error {
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

// UpdateDeploymentModel updates the model for a specific deployment using simplified shared ConfigMap approach
// This adds entries directly to the shared model-config ConfigMap (no deployment-registry)
func (p *DefaultConfigMapProvider) UpdateDeploymentModel(ctx context.Context, inferenceServerName, namespace, deploymentName, modelName string, roleType string) error {
	p.logger.Info("Updating deployment model directly in shared ConfigMap",
		zap.String("inferenceServer", inferenceServerName),
		zap.String("deployment", deploymentName),
		zap.String("model", modelName),
		zap.String("role", roleType))

	// Simply add the model to the shared ConfigMap
	if modelName != "" {
		if err := p.AddModelToConfig(ctx, inferenceServerName, namespace, modelName, fmt.Sprintf("s3://deploy-models/%s/", modelName)); err != nil {
			p.logger.Error("Failed to add model to shared ConfigMap", zap.String("model", modelName), zap.Error(err))
			return fmt.Errorf("failed to add model to shared ConfigMap: %w", err)
		}
	}

	p.logger.Info("Successfully updated deployment model in shared ConfigMap", zap.String("deployment", deploymentName), zap.String("model", modelName), zap.String("role", roleType))
	return nil
}

// RemoveDeploymentFromRegistry removes a deployment from the deployment registry (UCS cleanup pattern)
func (p *DefaultConfigMapProvider) RemoveDeploymentFromRegistry(ctx context.Context, inferenceServerName, namespace, deploymentName string) error {
	registryName := fmt.Sprintf("%s-deployment-registry", inferenceServerName)

	p.logger.Info("Removing deployment from registry",
		zap.String("registry", registryName),
		zap.String("deployment", deploymentName))

	// Get deployment registry
	registry, err := p.getDeploymentRegistry(ctx, registryName, namespace)
	if err != nil {
		// If registry doesn't exist, deployment is already "removed"
		if client.IgnoreNotFound(err) == nil {
			p.logger.Info("Deployment registry not found, deployment already removed",
				zap.String("registry", registryName), zap.String("deployment", deploymentName))
			return nil
		}
		return fmt.Errorf("failed to get deployment registry: %w", err)
	}

	// Remove deployment from registry
	if registry.Deployments != nil {
		if _, exists := registry.Deployments[deploymentName]; exists {
			delete(registry.Deployments, deploymentName)
			p.logger.Info("Deployment removed from registry", zap.String("deployment", deploymentName))

			// Save updated registry back to ConfigMap
			if err := p.saveDeploymentRegistry(ctx, registryName, namespace, registry); err != nil {
				return fmt.Errorf("failed to save deployment registry after removal: %w", err)
			}
		} else {
			p.logger.Info("Deployment not found in registry, no removal needed", zap.String("deployment", deploymentName))
		}
	}

	p.logger.Info("Successfully removed deployment from UCS registry", zap.String("deployment", deploymentName))
	return nil
}

// GetActiveModelsForInferenceServer returns all models currently used by any deployment
// This enables safe cleanup by identifying which models are still needed
func (p *DefaultConfigMapProvider) GetActiveModelsForInferenceServer(ctx context.Context, inferenceServerName, namespace string) ([]string, error) {
	registryName := fmt.Sprintf("%s-deployment-registry", inferenceServerName)

	registry, err := p.getDeploymentRegistry(ctx, registryName, namespace)
	if err != nil {
		// If registry doesn't exist, no active models
		return []string{}, nil
	}

	// Collect all unique models across all deployments
	activeModels := make(map[string]bool)
	for deploymentName, models := range registry.Deployments {
		p.logger.Info("Checking deployment models", zap.String("deployment", deploymentName), zap.String("current", models.Current), zap.String("candidate", models.Candidate), zap.String("shadow", models.Shadow))

		if models.Current != "" {
			activeModels[models.Current] = true
		}
		if models.Candidate != "" {
			activeModels[models.Candidate] = true
		}
		if models.Shadow != "" {
			activeModels[models.Shadow] = true
		}
	}

	// Convert to slice
	var result []string
	for model := range activeModels {
		result = append(result, model)
	}

	p.logger.Info("Active models for inference server", zap.String("inferenceServer", inferenceServerName), zap.Int("activeModels", len(result)))
	return result, nil
}

// CleanupUnusedModels removes models from ConfigMap that are not used by any deployment
// This is the safe cleanup function that mimics Uber's asset lifecycle management
func (p *DefaultConfigMapProvider) CleanupUnusedModels(ctx context.Context, inferenceServerName, namespace string) error {
	p.logger.Info("Starting cleanup of unused models", zap.String("inferenceServer", inferenceServerName))

	// Get currently active models across all deployments
	activeModels, err := p.GetActiveModelsForInferenceServer(ctx, inferenceServerName, namespace)
	if err != nil {
		return fmt.Errorf("failed to get active models: %w", err)
	}

	// Get current models in ConfigMap
	currentConfigs, err := p.GetModelConfigMap(ctx, inferenceServerName, namespace)
	if err != nil {
		return fmt.Errorf("failed to get current model config: %w", err)
	}

	// Build active models map for fast lookup
	activeModelMap := make(map[string]bool)
	for _, model := range activeModels {
		activeModelMap[model] = true
	}

	// Filter out unused models
	var cleanedConfigs []ModelConfigEntry
	var removedModels []string

	for _, config := range currentConfigs {
		if activeModelMap[config.Name] {
			// Keep active models
			cleanedConfigs = append(cleanedConfigs, config)
		} else {
			// Mark for removal
			removedModels = append(removedModels, config.Name)
		}
	}

	if len(removedModels) == 0 {
		p.logger.Info("No unused models found, cleanup complete", zap.String("inferenceServer", inferenceServerName))
		return nil
	}

	p.logger.Info("Removing unused models from ConfigMap",
		zap.String("inferenceServer", inferenceServerName),
		zap.Int("removedModels", len(removedModels)),
		zap.Int("activeModels", len(activeModels)))

	// Update ConfigMap with cleaned model list
	request := ConfigMapRequest{
		InferenceServer: inferenceServerName,
		Namespace:       namespace,
		ModelConfigs:    cleanedConfigs,
	}

	return p.UpdateModelConfigMap(ctx, request)
}

// getOrCreateDeploymentRegistry gets existing registry or creates new one
func (p *DefaultConfigMapProvider) getOrCreateDeploymentRegistry(ctx context.Context, registryName, namespace, inferenceServerName string) (*DeploymentModelRegistry, error) {
	registry, err := p.getDeploymentRegistry(ctx, registryName, namespace)
	if err != nil {
		// Create new registry if it doesn't exist
		registry = &DeploymentModelRegistry{
			InferenceServer: inferenceServerName,
			Deployments:     make(map[string]DeploymentModels),
		}

		// Create the ConfigMap for the registry
		if err := p.createDeploymentRegistryConfigMap(ctx, registryName, namespace, registry); err != nil {
			return nil, fmt.Errorf("failed to create deployment registry: %w", err)
		}
	}
	return registry, nil
}

// getDeploymentRegistry retrieves deployment registry from ConfigMap
func (p *DefaultConfigMapProvider) getDeploymentRegistry(ctx context.Context, registryName, namespace string) (*DeploymentModelRegistry, error) {
	configMap := &corev1.ConfigMap{}
	err := p.kubeClient.Get(ctx, client.ObjectKey{Name: registryName, Namespace: namespace}, configMap)
	if err != nil {
		return nil, err
	}

	registryJSON, exists := configMap.Data["deployment-registry.json"]
	if !exists {
		return nil, fmt.Errorf("deployment-registry.json not found in ConfigMap %s", registryName)
	}

	var registry DeploymentModelRegistry
	if err := json.Unmarshal([]byte(registryJSON), &registry); err != nil {
		return nil, fmt.Errorf("failed to parse deployment registry: %w", err)
	}

	return &registry, nil
}

// createDeploymentRegistryConfigMap creates ConfigMap for deployment registry
func (p *DefaultConfigMapProvider) createDeploymentRegistryConfigMap(ctx context.Context, registryName, namespace string, registry *DeploymentModelRegistry) error {
	registryJSON, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal deployment registry: %w", err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      registryName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component":      "deployment-registry",
				"app.kubernetes.io/part-of":        "michelangelo",
				"michelangelo.ai/inference-server": registry.InferenceServer,
			},
		},
		Data: map[string]string{
			"deployment-registry.json": string(registryJSON),
		},
	}

	return p.kubeClient.Create(ctx, configMap)
}

// saveDeploymentRegistry saves deployment registry to ConfigMap
func (p *DefaultConfigMapProvider) saveDeploymentRegistry(ctx context.Context, registryName, namespace string, registry *DeploymentModelRegistry) error {
	configMap := &corev1.ConfigMap{}
	err := p.kubeClient.Get(ctx, client.ObjectKey{Name: registryName, Namespace: namespace}, configMap)
	if err != nil {
		return fmt.Errorf("failed to get deployment registry ConfigMap: %w", err)
	}

	registryJSON, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal deployment registry: %w", err)
	}

	configMap.Data["deployment-registry.json"] = string(registryJSON)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return p.kubeClient.Update(ctx, configMap)
}

// FlushMergedStateToModelConfig implements Uber's UCS flush pattern
// This method merges the deployment registry state with the model config,
// ensuring the model-config ConfigMap reflects all active models from all deployments
func (p *DefaultConfigMapProvider) FlushMergedStateToModelConfig(ctx context.Context, inferenceServerName, namespace string) error {
	p.logger.Info("UCS FLUSH: Merging deployment registry state to model config", zap.String("inferenceServer", inferenceServerName))

	// Get active models from deployment registry (like Uber's generateTrackedModels)
	activeModels, err := p.GetActiveModelsForInferenceServer(ctx, inferenceServerName, namespace)
	if err != nil {
		p.logger.Error("Failed to get active models from deployment registry", zap.Error(err))
		return fmt.Errorf("failed to get active models: %w", err)
	}

	// Get current model config
	currentConfigs, err := p.GetModelConfigMap(ctx, inferenceServerName, namespace)
	if err != nil {
		p.logger.Error("Failed to get current model config", zap.Error(err))
		return fmt.Errorf("failed to get current model config: %w", err)
	}

	// UCS MERGE LOGIC: Build merged model config based on active models
	// This replicates Uber's UCS cache flush pattern from background.go:208-341
	activeModelMap := make(map[string]bool)
	for _, model := range activeModels {
		activeModelMap[model] = true
	}

	var mergedConfigs []ModelConfigEntry

	// Keep existing configs for active models
	for _, config := range currentConfigs {
		if activeModelMap[config.Name] {
			mergedConfigs = append(mergedConfigs, config)
			delete(activeModelMap, config.Name) // Mark as processed
		}
	}

	// Add new configs for active models not in current config
	for model := range activeModelMap {
		if model != "" {
			mergedConfigs = append(mergedConfigs, ModelConfigEntry{
				Name:   model,
				S3Path: fmt.Sprintf("s3://deploy-models/%s/", model),
			})
		}
	}

	p.logger.Info("UCS FLUSH: Merged model configuration",
		zap.String("inferenceServer", inferenceServerName),
		zap.Int("activeModels", len(activeModels)),
		zap.Int("currentConfigs", len(currentConfigs)),
		zap.Int("mergedConfigs", len(mergedConfigs)))

	// Update model config with merged state (like Uber's UCS flush)
	request := ConfigMapRequest{
		InferenceServer: inferenceServerName,
		Namespace:       namespace,
		ModelConfigs:    mergedConfigs,
	}

	if err := p.UpdateModelConfigMap(ctx, request); err != nil {
		return fmt.Errorf("failed to flush merged state to model config: %w", err)
	}

	p.logger.Info("UCS FLUSH: Successfully flushed merged state to model config", zap.String("inferenceServer", inferenceServerName))
	return nil
}
