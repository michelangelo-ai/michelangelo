package llmd

import (
	"context"
	"encoding/json"
	"fmt"
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

type LLMDProvider struct {
	DynamicClient dynamic.Interface
}

var _ serving.Provider = &LLMDProvider{}

func (r LLMDProvider) CreateInferenceServer(ctx context.Context, log logr.Logger, name, namespace string, configMapName string) error {
	// First, we need to get the InferenceServer to access its deployable artifact URI
	// For now, we'll need to fetch it using the dynamic client
	inferenceServerGVR := schema.GroupVersionResource{
		Group:    "michelangelo.ai",
		Version:  "v2",
		Resource: "inferenceservers",
	}

	inferenceServerObj, err := r.DynamicClient.Resource(inferenceServerGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to get InferenceServer for deployable artifact URI")
		return err
	}

	// Extract deployable artifact URI from the InferenceServer
	modelURI, err := r.getDeployableArtifactURI(inferenceServerObj)
	if err != nil {
		log.Error(err, "Failed to get deployable artifact URI")
		return err
	}

	log.Info("Using model URI for LLM-D deployment", "modelURI", modelURI)

	err = r.createLLMDModelService(ctx, log, name, namespace, configMapName, modelURI)
	if err != nil {
		return err
	}

	log.Info("LLM-D ModelService created successfully")
	return nil
}

func (r LLMDProvider) getDeployableArtifactURI(inferenceServerObj *unstructured.Unstructured) (string, error) {
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
		modelObj, err := r.DynamicClient.Resource(modelGVR).Namespace(namespace).Get(context.Background(), modelName, metav1.GetOptions{})
		if err != nil {
			if utils.IsNotFoundError(err) {
				// Model doesn't exist yet - this is expected when InferenceServer is created before training completes
				// We'll use the fallback URI pattern
				fallbackURI := fmt.Sprintf("s3://deploy-models/%s", modelName)
				// Note: This is logged in the calling function, so we don't log here to avoid duplicate logs
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

func (r LLMDProvider) createLLMDModelService(ctx context.Context, log logr.Logger, name, namespace string, configMapName string, modelURI string) error {
	gvr := schema.GroupVersionResource{
		Group:    "llm-d.ai",
		Version:  "v1alpha1",
		Resource: "modelservices",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "llm-d.ai/v1alpha1",
			"kind":       "ModelService",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"michelangelo.ai/inference": name,
					"michelangelo.ai/provider":  "llm-d",
				},
			},
			"spec": map[string]interface{}{
				"decoupleScaling": false,
				"baseConfigMapRef": map[string]interface{}{
					"name": "basic-sim-preset",
				},
				"routing": map[string]interface{}{
					"modelName": name,
				},
				"modelArtifacts": map[string]interface{}{
					"uri": modelURI,
				},
				"endpointPicker": map[string]interface{}{
					"replicas": 1,
					"containers": []map[string]interface{}{
						{
							"name":  "epp",
							"image": "ghcr.io/llm-d/llm-d-inference-scheduler:dev",
							"env": []map[string]interface{}{
								{"name": "ENABLE_KVCACHE_AWARE_SCORER", "value": "false"},
								{"name": "ENABLE_PREFIX_AWARE_SCORER", "value": "true"},
								{"name": "PREFIX_AWARE_SCORER_WEIGHT", "value": "2"},
								{"name": "ENABLE_LOAD_AWARE_SCORER", "value": "true"},
								{"name": "LOAD_AWARE_SCORER_WEIGHT", "value": "1"},
								{"name": "ENABLE_SESSION_AWARE_SCORER", "value": "false"},
								{"name": "PD_ENABLED", "value": "false"},
								{"name": "PD_PROMPT_LEN_THRESHOLD", "value": "10"},
							},
						},
					},
				},
				"decode": map[string]interface{}{
					"replicas": 1,
					"containers": []map[string]interface{}{
						{
							"name": "vllm",
							"args": []string{
								"--model",
								modelURI,
							},
						},
					},
				},
				"prefill": map[string]interface{}{
					"replicas": 1,
					"containers": []map[string]interface{}{
						{
							"name": "vllm",
							"args": []string{
								"--model",
								modelURI,
							},
						},
					},
				},
			},
		},
	}

	_, err := r.DynamicClient.Resource(gvr).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		log.Error(err, "Failed to create LLM-D ModelService")
		return err
	}

	log.Info("LLM-D ModelService created successfully")
	return nil
}

func (r LLMDProvider) UpdateInferenceServer(ctx context.Context, log logr.Logger, name, namespace string) error {
	log.Info("Starting model update for LLM-D ModelService", "name", name, "namespace", namespace)

	// Get the ConfigMap to read current model configuration
	configMapName := fmt.Sprintf("%s-model-config", name)
	configMapGVR := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	configMapObj, err := r.DynamicClient.Resource(configMapGVR).Namespace(namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to get ConfigMap for model update")
		return err
	}

	// Extract model list from ConfigMap
	data, found, err := unstructured.NestedString(configMapObj.Object, "data", "model-list.json")
	if err != nil || !found {
		log.Error(err, "Failed to get model-list.json from ConfigMap")
		return fmt.Errorf("model-list.json not found in ConfigMap")
	}

	var modelList []map[string]interface{}
	if parseErr := json.Unmarshal([]byte(data), &modelList); parseErr != nil {
		log.Error(parseErr, "Failed to parse model list JSON")
		return parseErr
	}

	if len(modelList) == 0 {
		log.Info("No models to load in ConfigMap")
		return nil
	}

	// Get the target model to deploy (most recent)
	targetModel := modelList[0]
	targetModelURI, ok := targetModel["s3_path"].(string)
	if !ok {
		return fmt.Errorf("invalid model URI in ConfigMap")
	}

	targetModelName, ok := targetModel["name"].(string)
	if !ok {
		return fmt.Errorf("invalid model name in ConfigMap")
	}

	// Check if we're already serving this model
	currentModelName, err := r.getCurrentModel(ctx, log, name, namespace)
	if err != nil {
		log.Error(err, "Failed to get current model")
		return err
	}

	if currentModelName == targetModelName {
		log.Info("Already serving target model", "model", targetModelName)
		return nil
	}

	log.Info("Deploying new model", "from", currentModelName, "to", targetModelName, "uri", targetModelURI)

	// Deploy new model instance
	err = r.deployNewModelInstance(ctx, log, name, namespace, targetModelName, targetModelURI)
	if err != nil {
		log.Error(err, "Failed to deploy new model instance")
		return err
	}

	// Wait for new model to be ready
	err = r.waitForModelReady(ctx, log, name, namespace, targetModelName)
	if err != nil {
		log.Error(err, "New model failed to become ready")
		// Cleanup failed deployment
		r.cleanupModelInstance(ctx, log, name, namespace, targetModelName)
		return err
	}

	// Switch traffic to new model
	err = r.switchTrafficToNewModel(ctx, log, name, namespace, targetModelName)
	if err != nil {
		log.Error(err, "Failed to switch traffic to new model")
		return err
	}

	// Cleanup old model instance
	if currentModelName != "" {
		err = r.cleanupModelInstance(ctx, log, name, namespace, currentModelName)
		if err != nil {
			log.Error(err, "Failed to cleanup old model instance", "oldModel", currentModelName)
			// Don't fail the deployment for cleanup errors
		}
	}

	log.Info("Model update completed successfully", "newModel", targetModelName)
	return nil
}

func (r LLMDProvider) getCurrentModel(ctx context.Context, log logr.Logger, name, namespace string) (string, error) {
	// Check VirtualService to see which model is currently being served
	vsGVR := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	vsName := fmt.Sprintf("%s-virtualservice", name)
	vs, err := r.DynamicClient.Resource(vsGVR).Namespace(namespace).Get(ctx, vsName, metav1.GetOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			log.Info("VirtualService not found, no current model")
			return "", nil
		}
		return "", err
	}

	// Extract current destination from VirtualService
	routes, found, err := unstructured.NestedSlice(vs.Object, "spec", "http", "0", "route")
	if err != nil || !found || len(routes) == 0 {
		return "", nil
	}

	route := routes[0].(map[string]interface{})
	destination, found, err := unstructured.NestedMap(route, "destination")
	if err != nil || !found {
		return "", nil
	}

	host, found, err := unstructured.NestedString(destination, "host")
	if err != nil || !found {
		return "", nil
	}

	// Extract model name from host (format: {name}-{model}.{namespace}.svc.cluster.local)
	if strings.Contains(host, fmt.Sprintf(".%s.svc.cluster.local", namespace)) {
		serviceName := strings.Split(host, ".")[0]
		if strings.HasPrefix(serviceName, name+"-") {
			modelName := strings.TrimPrefix(serviceName, name+"-")
			return modelName, nil
		}
	}

	return "", nil
}

func (r LLMDProvider) deployNewModelInstance(ctx context.Context, log logr.Logger, name, namespace, modelName, modelURI string) error {
	log.Info("Deploying new model instance", "model", modelName)

	// Create a new ModelService for the new model
	modelServiceName := fmt.Sprintf("%s-%s", name, modelName)
	
	gvr := schema.GroupVersionResource{
		Group:    "llm-d.ai",
		Version:  "v1alpha1",
		Resource: "modelservices",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "llm-d.ai/v1alpha1",
			"kind":       "ModelService",
			"metadata": map[string]interface{}{
				"name":      modelServiceName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"michelangelo.ai/inference": name,
					"michelangelo.ai/provider":  "llm-d",
					"michelangelo.ai/model":     modelName,
				},
				"annotations": map[string]interface{}{
					"michelangelo.ai/model-name": modelName,
					"michelangelo.ai/model-uri":  modelURI,
					"michelangelo.ai/created":    fmt.Sprintf("%d", time.Now().Unix()),
				},
			},
			"spec": map[string]interface{}{
				"decoupleScaling": false,
				"baseConfigMapRef": map[string]interface{}{
					"name": "basic-sim-preset",
				},
				"routing": map[string]interface{}{
					"modelName": modelName,
				},
				"modelArtifacts": map[string]interface{}{
					"uri": modelURI,
				},
				"endpointPicker": map[string]interface{}{
					"replicas": 1,
					"containers": []map[string]interface{}{
						{
							"name":  "epp",
							"image": "ghcr.io/llm-d/llm-d-inference-scheduler:dev",
							"env": []map[string]interface{}{
								{"name": "ENABLE_KVCACHE_AWARE_SCORER", "value": "false"},
								{"name": "ENABLE_PREFIX_AWARE_SCORER", "value": "true"},
								{"name": "PREFIX_AWARE_SCORER_WEIGHT", "value": "2"},
								{"name": "ENABLE_LOAD_AWARE_SCORER", "value": "true"},
								{"name": "LOAD_AWARE_SCORER_WEIGHT", "value": "1"},
								{"name": "ENABLE_SESSION_AWARE_SCORER", "value": "false"},
								{"name": "PD_ENABLED", "value": "false"},
								{"name": "PD_PROMPT_LEN_THRESHOLD", "value": "10"},
							},
						},
					},
				},
				"decode": map[string]interface{}{
					"replicas": 1,
					"containers": []map[string]interface{}{
						{
							"name": "vllm",
							"args": []string{
								"--model",
								modelURI,
							},
						},
					},
				},
				"prefill": map[string]interface{}{
					"replicas": 1,
					"containers": []map[string]interface{}{
						{
							"name": "vllm",
							"args": []string{
								"--model",
								modelURI,
							},
						},
					},
				},
			},
		},
	}

	_, err := r.DynamicClient.Resource(gvr).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create ModelService for new model: %w", err)
	}

	log.Info("New model instance created", "modelService", modelServiceName)
	return nil
}

func (r LLMDProvider) waitForModelReady(ctx context.Context, log logr.Logger, name, namespace, modelName string) error {
	log.Info("Waiting for new model to be ready", "model", modelName)

	modelServiceName := fmt.Sprintf("%s-%s", name, modelName)
	
	// Wait for ModelService to be ready
	timeout := time.After(10 * time.Minute) // 10 minute timeout
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for model to be ready: %s", modelName)
		case <-ticker.C:
			ready, err := r.checkModelServiceReady(ctx, log, modelServiceName, namespace)
			if err != nil {
				log.Error(err, "Error checking model readiness")
				continue
			}
			if ready {
				// Additional check: ping the model endpoint
				modelReady, err := r.pingModelEndpoint(ctx, log, modelServiceName, namespace, modelName)
				if err != nil {
					log.Error(err, "Error pinging model endpoint")
					continue
				}
				if modelReady {
					log.Info("Model is ready and responding", "model", modelName)
					return nil
				}
			}
			log.Info("Still waiting for model to be ready", "model", modelName)
		}
	}
}

func (r LLMDProvider) checkModelServiceReady(ctx context.Context, log logr.Logger, modelServiceName, namespace string) (bool, error) {
	gvr := schema.GroupVersionResource{
		Group:    "llm-d.ai",
		Version:  "v1alpha1",
		Resource: "modelservices",
	}

	modelService, err := r.DynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, modelServiceName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	phase, found, err := unstructured.NestedString(modelService.Object, "status", "phase")
	if err != nil || !found {
		return false, nil
	}

	return phase == "Ready", nil
}

func (r LLMDProvider) pingModelEndpoint(ctx context.Context, log logr.Logger, modelServiceName, namespace, modelName string) (bool, error) {
	// Construct the model endpoint URL
	// Format: http://{modelServiceName}.{namespace}.svc.cluster.local:8000/v1/models/{modelName}
	url := fmt.Sprintf("http://%s.%s.svc.cluster.local:8000/v1/models/%s", modelServiceName, namespace, modelName)
	
	log.Info("Pinging model endpoint", "url", url)
	
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return false, fmt.Errorf("failed to ping model endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Info("Model endpoint is healthy", "model", modelName, "status", resp.StatusCode)
		return true, nil
	}

	log.Info("Model endpoint not ready yet", "model", modelName, "status", resp.StatusCode)
	return false, nil
}

func (r LLMDProvider) switchTrafficToNewModel(ctx context.Context, log logr.Logger, name, namespace, modelName string) error {
	log.Info("Switching traffic to new model", "model", modelName)

	modelServiceName := fmt.Sprintf("%s-%s", name, modelName)
	
	// Update VirtualService to route to new model
	vsGVR := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	vsName := fmt.Sprintf("%s-virtualservice", name)
	vs, err := r.DynamicClient.Resource(vsGVR).Namespace(namespace).Get(ctx, vsName, metav1.GetOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			// Create new VirtualService
			return r.createVirtualServiceForModel(ctx, log, name, namespace, modelServiceName)
		}
		return fmt.Errorf("failed to get VirtualService: %w", err)
	}

	// Update existing VirtualService to point to new model service
	newDestination := map[string]interface{}{
		"host": fmt.Sprintf("%s.%s.svc.cluster.local", modelServiceName, namespace),
		"port": map[string]interface{}{
			"number": int64(8000),
		},
	}

	err = unstructured.SetNestedField(vs.Object, newDestination, "spec", "http", "0", "route", "0", "destination")
	if err != nil {
		return fmt.Errorf("failed to update VirtualService destination: %w", err)
	}

	// Add metadata about the switch
	metadata := vs.Object["metadata"].(map[string]interface{})
	if metadata["annotations"] == nil {
		metadata["annotations"] = make(map[string]interface{})
	}
	annotations := metadata["annotations"].(map[string]interface{})
	annotations["michelangelo.ai/current-model"] = modelName
	annotations["michelangelo.ai/last-updated"] = fmt.Sprintf("%d", time.Now().Unix())

	_, err = r.DynamicClient.Resource(vsGVR).Namespace(namespace).Update(ctx, vs, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update VirtualService: %w", err)
	}

	log.Info("Traffic switched to new model successfully", "model", modelName, "service", modelServiceName)
	return nil
}

func (r LLMDProvider) createVirtualServiceForModel(ctx context.Context, log logr.Logger, name, namespace, modelServiceName string) error {
	vsGVR := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1beta1",
			"kind":       "VirtualService",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("%s-virtualservice", name),
				"namespace": namespace,
				"labels": map[string]interface{}{
					"michelangelo.ai/inference": name,
					"michelangelo.ai/provider":  "llm-d",
				},
			},
			"spec": map[string]interface{}{
				"hosts":    []interface{}{"*"},
				"gateways": []interface{}{"ma-gateway"},
				"http": []interface{}{
					map[string]interface{}{
						"match": []interface{}{
							map[string]interface{}{
								"uri": map[string]interface{}{
									"prefix": fmt.Sprintf("/%s-endpoint/", name),
								},
							},
						},
						"rewrite": map[string]interface{}{
							"uri": "/",
						},
						"route": []interface{}{
							map[string]interface{}{
								"destination": map[string]interface{}{
									"host": fmt.Sprintf("%s.%s.svc.cluster.local", modelServiceName, namespace),
									"port": map[string]interface{}{
										"number": int64(8000),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := r.DynamicClient.Resource(vsGVR).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create VirtualService: %w", err)
	}

	log.Info("VirtualService created for new model", "service", modelServiceName)
	return nil
}

func (r LLMDProvider) cleanupModelInstance(ctx context.Context, log logr.Logger, name, namespace, modelName string) error {
	log.Info("Cleaning up old model instance", "model", modelName)

	modelServiceName := fmt.Sprintf("%s-%s", name, modelName)
	
	gvr := schema.GroupVersionResource{
		Group:    "llm-d.ai",
		Version:  "v1alpha1",
		Resource: "modelservices",
	}

	err := r.DynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, modelServiceName, metav1.DeleteOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			log.Info("Model instance already cleaned up", "model", modelName)
			return nil
		}
		return fmt.Errorf("failed to delete old model instance: %w", err)
	}

	log.Info("Old model instance cleaned up successfully", "model", modelName)
	return nil
}


func (r LLMDProvider) GetStatus(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	name := inferenceServer.GetMetadata().GetName()
	namespace := inferenceServer.GetMetadata().GetNamespace()

	logger.Info("Getting LLM-D ModelService status", "name", name, "namespace", namespace)

	gvr := schema.GroupVersionResource{
		Group:    "llm-d.ai",
		Version:  "v1alpha1",
		Resource: "modelservices",
	}

	modelService, err := r.DynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			logger.Info("LLM-D ModelService not found", "name", name)
			inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_INITIALIZED
			inferenceServer.Status.UpdateTime = fmt.Sprintf("%d", time.Now().Unix())
			return nil
		}
		logger.Error(err, "Failed to get LLM-D ModelService")
		inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_FAILED
		inferenceServer.Status.UpdateTime = fmt.Sprintf("%d", time.Now().Unix())
		return err
	}

	phase, found, err := unstructured.NestedString(modelService.Object, "status", "phase")
	if err != nil || !found {
		phase = "Unknown"
	}

	switch phase {
	case "Ready":
		inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_SERVING
	case "Creating", "Pending":
		inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_CREATING
	case "Failed", "Error":
		inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_FAILED
	default:
		inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_CREATING
	}

	inferenceServer.Status.UpdateTime = fmt.Sprintf("%d", time.Now().Unix())
	logger.Info("LLM-D ModelService status", "name", name, "phase", phase, "state", inferenceServer.Status.State)
	return nil
}

func (r LLMDProvider) DeleteInferenceServer(ctx context.Context, log logr.Logger, name, namespace string) error {
	gvr := schema.GroupVersionResource{
		Group:    "llm-d.ai",
		Version:  "v1alpha1",
		Resource: "modelservices",
	}

	err := r.DynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			log.Info("LLM-D ModelService already deleted", "name", name)
			return nil
		}
		log.Error(err, "Failed to delete LLM-D ModelService")
		return err
	}

	log.Info("LLM-D ModelService deleted successfully")
	return nil
}
