package dynamo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/provider/serving"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// DynamoInferenceServerProvider implements the Provider interface for NVIDIA Dynamo
type DynamoInferenceServerProvider struct {
	DynamicClient dynamic.Interface
	Config        *DynamoConfig
}

var _ serving.Provider = &DynamoInferenceServerProvider{}

// DynamoConfig holds configuration for Dynamo deployments
type DynamoConfig struct {
	Backend         string // "vllm", "sglang", "tensorrtllm", "mistralrs"
	ImageRegistry   string
	DefaultReplicas int
	ComponentTag    string // Dynamo component tag
}



// === Provider Interface Implementation ===

func (d DynamoInferenceServerProvider) CreateInferenceServer(ctx context.Context, log logr.Logger, name, namespace string, configMapName string) error {
	log.Info("Creating Dynamo InferenceServer for LLM serving", "name", name, "namespace", namespace, "backend", d.Config.Backend)

	// First, we need to get the InferenceServer to access its deployable artifact URI
	// For now, we'll need to fetch it using the dynamic client
	inferenceServerGVR := schema.GroupVersionResource{
		Group:    "michelangelo.ai",
		Version:  "v2",
		Resource: "inferenceservers",
	}

	inferenceServerObj, err := d.DynamicClient.Resource(inferenceServerGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to get InferenceServer for deployable artifact URI")
		return err
	}

	// Extract deployable artifact URI from the InferenceServer
	modelURI, err := d.getDeployableArtifactURI(inferenceServerObj)
	if err != nil {
		log.Error(err, "Failed to get deployable artifact URI")
		return err
	}

	log.Info("Using model URI for Dynamo deployment", "modelURI", modelURI)

	// Create Dynamo infrastructure with model deployment
	err = d.createDynamoInfrastructureWithModel(ctx, log, name, namespace, configMapName, modelURI)
	if err != nil {
		return fmt.Errorf("failed to create Dynamo infrastructure: %w", err)
	}

	log.Info("Dynamo InferenceServer created successfully")
	return nil
}

func (d DynamoInferenceServerProvider) UpdateInferenceServer(ctx context.Context, log logr.Logger, name, namespace string) error {
	log.Info("Starting Dynamo model update", "name", name, "namespace", namespace)

	// Get the InferenceServer to check for model updates
	inferenceServerGVR := schema.GroupVersionResource{
		Group:    "michelangelo.ai",
		Version:  "v2",
		Resource: "inferenceservers",
	}

	inferenceServerObj, err := d.DynamicClient.Resource(inferenceServerGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to get InferenceServer for model update")
		return err
	}

	// Extract model information from protobuf
	modelURI, err := d.getDeployableArtifactURI(inferenceServerObj)
	if err != nil {
		log.Error(err, "Failed to get deployable artifact URI")
		return err
	}

	modelName := d.extractModelNameFromURI(modelURI, name)

	// Check if we're already serving this model
	currentModelName, err := d.getCurrentModel(ctx, log, name, namespace)
	if err != nil {
		log.Error(err, "Failed to get current model")
		return err
	}

	if currentModelName == modelName {
		log.Info("Already serving target model", "model", modelName)
		return nil
	}

	log.Info("Deploying new model", "from", currentModelName, "to", modelName, "uri", modelURI)

	// Deploy new model instance
	err = d.deployNewModelInstance(ctx, log, name, namespace, modelName, modelURI)
	if err != nil {
		log.Error(err, "Failed to deploy new model instance")
		return err
	}

	log.Info("Model update completed successfully", "newModel", modelName)
	return nil
}

func (d DynamoInferenceServerProvider) GetStatus(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	name := inferenceServer.GetMetadata().GetName()
	namespace := inferenceServer.GetMetadata().GetNamespace()

	logger.Info("Getting Dynamo InferenceServer status", "name", name, "namespace", namespace)

	// Check DynamoGraphDeployment status
	deploymentExists, err := d.checkDynamoDeploymentExists(ctx, logger, name, namespace)
	if err != nil {
		logger.Error(err, "Failed to check Dynamo deployment")
		inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_FAILED
		inferenceServer.Status.UpdateTime = fmt.Sprintf("%d", time.Now().Unix())
		return err
	}

	if !deploymentExists {
		logger.Info("Dynamo deployment not found", "name", name)
		inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_INITIALIZED
		inferenceServer.Status.UpdateTime = fmt.Sprintf("%d", time.Now().Unix())
		return nil
	}

	// Check if models are deployed and ready
	ready, err := d.checkModelServingStatus(ctx, logger, name, namespace)
	if err != nil {
		logger.Error(err, "Failed to check model serving status")
		inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_CREATING
	} else if ready {
		logger.Info("Dynamo deployment is serving models", "name", name)
		inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_SERVING
	} else {
		logger.Info("Dynamo deployment is creating", "name", name)
		inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_CREATING
	}

	inferenceServer.Status.UpdateTime = fmt.Sprintf("%d", time.Now().Unix())
	return nil
}

func (d DynamoInferenceServerProvider) DeleteInferenceServer(ctx context.Context, log logr.Logger, name, namespace string) error {
	log.Info("Deleting Dynamo InferenceServer", "name", name, "namespace", namespace)

	// Delete DynamoGraphDeployment
	err := d.deleteDynamoDeployment(ctx, log, name, namespace)
	if err != nil && !utils.IsNotFoundError(err) {
		log.Error(err, "Failed to delete Dynamo deployment")
		return err
	}

	// Clean up any associated DynamoComponents
	err = d.cleanupDynamoComponents(ctx, log, name, namespace)
	if err != nil {
		log.Error(err, "Failed to cleanup Dynamo components")
		// Don't fail the deletion for cleanup errors
	}

	log.Info("Dynamo InferenceServer deleted successfully")
	return nil
}

// === Dynamo-Specific Methods ===

// getDeployableArtifactURI extracts model path from InferenceServer protobuf, following llmd provider pattern
func (d DynamoInferenceServerProvider) getDeployableArtifactURI(inferenceServerObj *unstructured.Unstructured) (string, error) {
	// Try to get the deployable artifact URI from the InferenceServer spec
	// The path should be something like spec.deployable_artifact_uri or from a related Model resource

	inferenceServerName := inferenceServerObj.GetName()

	// First check if there's a model name in the spec that we can use to lookup the Model resource
	modelName, found, err := unstructured.NestedString(inferenceServerObj.Object, "spec", "model_name")
	if err != nil {
		return "", fmt.Errorf("error reading model_name from spec: %w", err)
	}

	if found && modelName != "" {
		// Try to get Model resource for the specified model_name
		// Get the Model resource to find the deployable artifact URI
		modelGVR := schema.GroupVersionResource{
			Group:    "michelangelo.ai",
			Version:  "v2",
			Resource: "models",
		}

		namespace := inferenceServerObj.GetNamespace()
		modelObj, err := d.DynamicClient.Resource(modelGVR).Namespace(namespace).Get(context.Background(), modelName, metav1.GetOptions{})
		if err != nil {
			if utils.IsNotFoundError(err) {
				// Model doesn't exist yet - this is expected when InferenceServer is created before training completes
				// We'll use the fallback URI pattern
				fallbackURI := fmt.Sprintf("s3://deploy-models/%s", modelName)
				return fallbackURI, nil
			}
			return "", fmt.Errorf("failed to get Model resource %s: %w", modelName, err)
		}

		// Extract deployable_artifact_uri from the Model spec
		artifactURIs, found, err := unstructured.NestedStringSlice(modelObj.Object, "spec", "deployable_artifact_uri")
		if err != nil {
			return "", fmt.Errorf("error reading deployable_artifact_uri from Model: %w", err)
		}

		if found && len(artifactURIs) > 0 {
			// Use the first URI (should be the S3 path based on our pusher)
			return artifactURIs[0], nil
		}

		// Model exists but doesn't have deployable_artifact_uri yet
		// Use fallback pattern based on model name
		fallbackURI := fmt.Sprintf("s3://deploy-models/%s", modelName)
		return fallbackURI, nil
	}

	// No model_name specified in InferenceServer spec
	// Fallback: use a default URI based on InferenceServer name
	// This matches the pattern from our pusher: s3://deploy-models/{name}
	fallbackURI := fmt.Sprintf("s3://deploy-models/%s", inferenceServerName)
	return fallbackURI, nil
}

// === Helper Functions ===

func (d DynamoInferenceServerProvider) createDynamoInfrastructureWithModel(ctx context.Context, log logr.Logger, name, namespace, configMapName, modelURI string) error {
	log.Info("Creating Dynamo infrastructure with model deployment", "name", name, "modelURI", modelURI)
	
	// Extract model name from URI or use inference server name
	modelName := d.extractModelNameFromURI(modelURI, name)
	
	// Create Dynamo component for building
	err := d.createDynamoComponentWithModel(ctx, log, name, namespace, modelName, modelURI)
	if err != nil {
		return fmt.Errorf("failed to create Dynamo component: %w", err)
	}

	log.Info("Dynamo infrastructure with model created", "name", name, "model", modelName)
	return nil
}

func (d DynamoInferenceServerProvider) createDynamoConfigMap(ctx context.Context, log logr.Logger, name, namespace, configMapName string) error {
	configMapGVR := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	dynamoConfigMapName := fmt.Sprintf("%s-dynamo-config", name)

	configMap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      dynamoConfigMapName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"michelangelo.ai/inference": name,
					"michelangelo.ai/provider":  "dynamo",
					"michelangelo.ai/component": "config",
				},
			},
			"data": map[string]interface{}{
				"backend":          d.Config.Backend,
				"imageRegistry":    d.Config.ImageRegistry,
				"defaultReplicas":  fmt.Sprintf("%d", d.Config.DefaultReplicas),
				"componentTag":     d.Config.ComponentTag,
				"originalConfig":   configMapName,
			},
		},
	}

	_, err := d.DynamicClient.Resource(configMapGVR).Namespace(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Dynamo config map: %w", err)
	}

	log.Info("Dynamo config map created", "configMap", dynamoConfigMapName)
	return nil
}

func (d DynamoInferenceServerProvider) extractModelNameFromURI(modelURI, fallbackName string) string {
	// Extract model name from various URI formats
	if strings.Contains(modelURI, "/") {
		parts := strings.Split(modelURI, "/")
		return parts[len(parts)-1]
	}
	return fallbackName
}

func (d DynamoInferenceServerProvider) createDynamoComponentWithModel(ctx context.Context, log logr.Logger, name, namespace, modelName, modelURI string) error {
	log.Info("Creating Dynamo component with model", "model", modelName, "backend", d.Config.Backend)

	// Create DynamoComponent for building
	componentGVR := schema.GroupVersionResource{
		Group:    "nvidia.com",
		Version:  "v1alpha1",
		Resource: "dynamocomponents",
	}

	componentID := fmt.Sprintf("frontend-%s-%d", strings.ToLower(strings.ReplaceAll(modelName, "/", "-")), time.Now().Unix())

	component := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "nvidia.com/v1alpha1",
			"kind":       "DynamoComponent",
			"metadata": map[string]interface{}{
				"name":      componentID,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"michelangelo.ai/inference": name,
					"michelangelo.ai/provider":  "dynamo",
					"michelangelo.ai/model":     modelName,
				},
			},
			"spec": map[string]interface{}{
				"build": map[string]interface{}{
					"framework": d.Config.Backend,
					"model":     modelName,
					"modelPath": modelURI,
				},
				"image": map[string]interface{}{
					"registry": d.Config.ImageRegistry,
					"tag":      fmt.Sprintf("%s-latest", d.Config.Backend),
				},
			},
		},
	}

	_, err := d.DynamicClient.Resource(componentGVR).Namespace(namespace).Create(ctx, component, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Dynamo component: %w", err)
	}

	log.Info("Dynamo component created", "componentId", componentID)
	
	// Create DynamoGraphDeployment immediately
	return d.createDynamoGraphDeploymentForComponent(ctx, log, name, namespace, componentID, modelName)
}



// === Status Check Functions ===

func (d DynamoInferenceServerProvider) checkDynamoDeploymentExists(ctx context.Context, log logr.Logger, name, namespace string) (bool, error) {
	deploymentGVR := schema.GroupVersionResource{
		Group:    "nvidia.com",
		Version:  "v1alpha1",
		Resource: "dynamographdeployments",
	}

	deploymentName := fmt.Sprintf("%s-deployment", name)
	_, err := d.DynamicClient.Resource(deploymentGVR).Namespace(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (d DynamoInferenceServerProvider) checkModelServingStatus(ctx context.Context, log logr.Logger, name, namespace string) (bool, error) {
	// Check if Dynamo deployment is serving models by querying the frontend service
	serviceName := fmt.Sprintf("%s-frontend", name)
	url := fmt.Sprintf("http://%s.%s.svc.cluster.local/v1/models", serviceName, namespace)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		log.Info("Model serving status check failed - service not ready", "error", err.Error())
		return false, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Info("Model serving status check failed", "status", resp.StatusCode)
		return false, nil
	}

	// Parse response to check if models are loaded
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response body: %w", err)
	}

	var models map[string]interface{}
	if err := json.Unmarshal(body, &models); err != nil {
		return false, fmt.Errorf("failed to parse models response: %w", err)
	}

	// Check if any models are loaded
	if data, ok := models["data"].([]interface{}); ok && len(data) > 0 {
		log.Info("Models are being served", "modelCount", len(data))
		return true, nil
	}

	log.Info("No models currently loaded")
	return false, nil
}

// === Stub Functions (to be implemented based on Dynamo operator APIs) ===

func (d DynamoInferenceServerProvider) deployNewModelInstance(ctx context.Context, log logr.Logger, name, namespace, modelName, modelURI string) error {
	log.Info("Deploying new model instance", "model", modelName)

	// Create a new DynamoComponent for the new model
	componentID := fmt.Sprintf("%s-%s-%d", name, strings.ToLower(strings.ReplaceAll(modelName, "/", "-")), time.Now().Unix())
	
	componentGVR := schema.GroupVersionResource{
		Group:    "nvidia.com",
		Version:  "v1alpha1",
		Resource: "dynamocomponents",
	}

	component := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "nvidia.com/v1alpha1",
			"kind":       "DynamoComponent",
			"metadata": map[string]interface{}{
				"name":      componentID,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"michelangelo.ai/inference": name,
					"michelangelo.ai/provider":  "dynamo",
					"michelangelo.ai/model":     modelName,
				},
				"annotations": map[string]interface{}{
					"michelangelo.ai/model-name": modelName,
					"michelangelo.ai/model-uri":  modelURI,
					"michelangelo.ai/created":    fmt.Sprintf("%d", time.Now().Unix()),
				},
			},
			"spec": map[string]interface{}{
				"build": map[string]interface{}{
					"framework": d.Config.Backend,
					"model":     modelName,
					"modelPath": modelURI,
				},
				"image": map[string]interface{}{
					"registry": d.Config.ImageRegistry,
					"tag":      fmt.Sprintf("%s-latest", d.Config.Backend),
				},
			},
		},
	}

	_, err := d.DynamicClient.Resource(componentGVR).Namespace(namespace).Create(ctx, component, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create DynamoComponent for new model: %w", err)
	}

	// Update the DynamoGraphDeployment to use the new component
	err = d.updateDynamoGraphDeploymentComponent(ctx, log, name, namespace, componentID)
	if err != nil {
		return fmt.Errorf("failed to update DynamoGraphDeployment: %w", err)
	}

	log.Info("New model instance deployed", "componentId", componentID)
	return nil
}

func (d DynamoInferenceServerProvider) getCurrentModel(ctx context.Context, log logr.Logger, name, namespace string) (string, error) {
	// Check DynamoGraphDeployment to see which component/model is currently being served
	deploymentGVR := schema.GroupVersionResource{
		Group:    "nvidia.com",
		Version:  "v1alpha1",
		Resource: "dynamographdeployments",
	}

	deploymentName := fmt.Sprintf("%s-deployment", name)
	deployment, err := d.DynamicClient.Resource(deploymentGVR).Namespace(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			log.Info("DynamoGraphDeployment not found, no current model")
			return "", nil
		}
		return "", err
	}

	// Extract current component from deployment spec
	componentID, found, err := unstructured.NestedString(deployment.Object, "spec", "dynamoComponent")
	if err != nil || !found {
		return "", nil
	}

	// Get the component to extract model name
	componentGVR := schema.GroupVersionResource{
		Group:    "nvidia.com",
		Version:  "v1alpha1",
		Resource: "dynamocomponents",
	}

	component, err := d.DynamicClient.Resource(componentGVR).Namespace(namespace).Get(ctx, componentID, metav1.GetOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			return "", nil
		}
		return "", err
	}

	// Extract model name from component spec
	modelName, found, err := unstructured.NestedString(component.Object, "spec", "build", "model")
	if err != nil || !found {
		// Try to get from annotations
		modelName, found, err = unstructured.NestedString(component.Object, "metadata", "annotations", "michelangelo.ai/model-name")
		if err != nil || !found {
			return "", nil
		}
	}

	return modelName, nil
}

func (d DynamoInferenceServerProvider) updateDynamoGraphDeploymentComponent(ctx context.Context, log logr.Logger, name, namespace, newComponentID string) error {
	deploymentGVR := schema.GroupVersionResource{
		Group:    "nvidia.com",
		Version:  "v1alpha1",
		Resource: "dynamographdeployments",
	}

	deploymentName := fmt.Sprintf("%s-deployment", name)
	deployment, err := d.DynamicClient.Resource(deploymentGVR).Namespace(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			// Create new deployment if it doesn't exist
			return d.createDynamoGraphDeploymentForComponent(ctx, log, name, namespace, newComponentID, "")
		}
		return fmt.Errorf("failed to get DynamoGraphDeployment: %w", err)
	}

	// Update the deployment to use the new component
	err = unstructured.SetNestedField(deployment.Object, newComponentID, "spec", "dynamoComponent")
	if err != nil {
		return fmt.Errorf("failed to update DynamoGraphDeployment component: %w", err)
	}

	// Add metadata about the update
	metadata := deployment.Object["metadata"].(map[string]interface{})
	if metadata["annotations"] == nil {
		metadata["annotations"] = make(map[string]interface{})
	}
	annotations := metadata["annotations"].(map[string]interface{})
	annotations["michelangelo.ai/component-id"] = newComponentID
	annotations["michelangelo.ai/last-updated"] = fmt.Sprintf("%d", time.Now().Unix())

	_, err = d.DynamicClient.Resource(deploymentGVR).Namespace(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update DynamoGraphDeployment: %w", err)
	}

	log.Info("DynamoGraphDeployment updated with new component", "componentId", newComponentID)
	return nil
}


func (d DynamoInferenceServerProvider) createDynamoGraphDeploymentForComponent(ctx context.Context, log logr.Logger, name, namespace, componentID, modelName string) error {
	// Create DynamoGraphDeployment CRD
	deploymentGVR := schema.GroupVersionResource{
		Group:    "nvidia.com",
		Version:  "v1alpha1",
		Resource: "dynamographdeployments",
	}

	deploymentID := fmt.Sprintf("%s-deployment", name)
	replicas := d.Config.DefaultReplicas

	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "nvidia.com/v1alpha1",
			"kind":       "DynamoGraphDeployment",
			"metadata": map[string]interface{}{
				"name":      deploymentID,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"michelangelo.ai/inference": name,
					"michelangelo.ai/provider":  "dynamo",
					"michelangelo.ai/model":     modelName,
				},
			},
			"spec": map[string]interface{}{
				"dynamoComponent": componentID,
				"services": map[string]interface{}{
					"Frontend": map[string]interface{}{
						"replicas": 1,
					},
					"Processor": map[string]interface{}{
						"replicas": 1,
					},
					fmt.Sprintf("%sWorker", strings.Title(d.Config.Backend)): map[string]interface{}{
						"replicas": replicas,
						"environment": map[string]interface{}{
							"MODEL_NAME": modelName,
						},
					},
				},
			},
		},
	}

	_, err := d.DynamicClient.Resource(deploymentGVR).Namespace(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Dynamo graph deployment: %w", err)
	}

	log.Info("DynamoGraphDeployment created", "deploymentId", deploymentID)
	return nil
}

func (d DynamoInferenceServerProvider) cleanupDynamoComponents(ctx context.Context, log logr.Logger, name, namespace string) error {
	log.Info("Cleaning up Dynamo components", "inferenceServer", name)

	componentGVR := schema.GroupVersionResource{
		Group:    "nvidia.com",
		Version:  "v1alpha1",
		Resource: "dynamocomponents",
	}

	// List all DynamoComponents associated with this inference server
	components, err := d.DynamicClient.Resource(componentGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("michelangelo.ai/inference=%s,michelangelo.ai/provider=dynamo", name),
	})
	if err != nil {
		if utils.IsNotFoundError(err) {
			log.Info("No Dynamo components found to cleanup")
			return nil
		}
		return fmt.Errorf("failed to list Dynamo components: %w", err)
	}

	// Delete each component
	for _, component := range components.Items {
		componentName := component.GetName()
		err := d.DynamicClient.Resource(componentGVR).Namespace(namespace).Delete(ctx, componentName, metav1.DeleteOptions{})
		if err != nil && !utils.IsNotFoundError(err) {
			log.Error(err, "Failed to delete Dynamo component", "component", componentName)
			continue
		}
		log.Info("Dynamo component deleted", "component", componentName)
	}

	log.Info("Dynamo components cleanup completed")
	return nil
}

func (d DynamoInferenceServerProvider) checkDynamoGraphDeploymentStatus(ctx context.Context, log logr.Logger, deploymentID, namespace string) (bool, error) {
	deploymentGVR := schema.GroupVersionResource{
		Group:    "nvidia.com",
		Version:  "v1alpha1",
		Resource: "dynamographdeployments",
	}

	deployment, err := d.DynamicClient.Resource(deploymentGVR).Namespace(namespace).Get(ctx, deploymentID, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	// Check deployment status
	status, found, err := unstructured.NestedString(deployment.Object, "status", "phase")
	if err != nil || !found {
		return false, nil // No status yet
	}

	return status == "Ready", nil
}

func (d DynamoInferenceServerProvider) deleteDynamoDeployment(ctx context.Context, log logr.Logger, name, namespace string) error {
	deploymentGVR := schema.GroupVersionResource{
		Group:    "nvidia.com",
		Version:  "v1alpha1",
		Resource: "dynamographdeployments",
	}

	deploymentName := fmt.Sprintf("%s-deployment", name)
	err := d.DynamicClient.Resource(deploymentGVR).Namespace(namespace).Delete(ctx, deploymentName, metav1.DeleteOptions{})
	if err != nil && !utils.IsNotFoundError(err) {
		return fmt.Errorf("failed to delete Dynamo deployment: %w", err)
	}

	log.Info("DynamoGraphDeployment deleted", "deployment", deploymentName)
	return nil
}