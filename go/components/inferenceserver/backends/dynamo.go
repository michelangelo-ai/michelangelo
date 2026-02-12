package backends

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ Backend = &dynamoBackend{}

const (
	// DynamoGraphDeployment CRD details
	dynamoAPIGroup   = "nvidia.com"
	dynamoAPIVersion = "v1alpha1"
	dynamoKind       = "DynamoGraphDeployment"

	// DynamoModel CRD details
	dynamoModelKind = "DynamoModel"

	// Default Dynamo container images from NGC
	defaultDynamoVLLMImage = "nvcr.io/nvidia/ai-dynamo/vllm-runtime:0.8.1"
	// defaultDynamoSGLangImage = "nvcr.io/nvidia/ai-dynamo/sglang-runtime:0.8.1"

	// Default base model - used for LoRA adapter loading
	// TODO: Make this configurable via InferenceServer spec
	defaultBaseModelName = "Qwen/Qwen3-0.6B"

	// Labels
	dynamoManagedByLabel = "app.kubernetes.io/managed-by"
	dynamoManagedByValue = "michelangelo"
)

// dynamoGVK is the GroupVersionKind for DynamoGraphDeployment
var dynamoGVK = schema.GroupVersionKind{
	Group:   dynamoAPIGroup,
	Version: dynamoAPIVersion,
	Kind:    dynamoKind,
}

// dynamoModelGVK is the GroupVersionKind for DynamoModel
var dynamoModelGVK = schema.GroupVersionKind{
	Group:   dynamoAPIGroup,
	Version: dynamoAPIVersion,
	Kind:    dynamoModelKind,
}

// dynamoBackend implements the Backend interface for NVIDIA Dynamo.
// It acts as a proxy to the Dynamo operator, creating DynamoGraphDeployment CRs
// instead of directly managing Pods/Services.
type dynamoBackend struct{}

// NewDynamoBackend creates a new Dynamo backend instance.
func NewDynamoBackend() *dynamoBackend {
	return &dynamoBackend{}
}

// CreateServer creates a DynamoGraphDeployment CR which the Dynamo operator reconciles
// into the actual Kubernetes resources (Deployments, Services, etc.).
func (b *dynamoBackend) CreateServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServer *v2pb.InferenceServer) (*ServerStatus, error) {
	logger.Info("Creating Dynamo inference server",
		zap.String("name", inferenceServer.Name),
		zap.String("namespace", inferenceServer.Namespace))

	// Check if DynamoGraphDeployment already exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(dynamoGVK)
	err := kubeClient.Get(ctx, client.ObjectKey{
		Name:      generateDynamoDGDName(inferenceServer.Name),
		Namespace: inferenceServer.Namespace,
	}, existing)
	if err == nil {
		logger.Info("DynamoGraphDeployment already exists, skipping creation",
			zap.String("name", generateDynamoDGDName(inferenceServer.Name)))
		return b.GetServerStatus(ctx, logger, kubeClient, inferenceServer.Name, inferenceServer.Namespace)
	}
	if !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to check existing DynamoGraphDeployment: %w", err)
	}

	// Build the DynamoGraphDeployment CR
	dgd := b.buildDynamoGraphDeployment(inferenceServer)

	// Create the DynamoGraphDeployment
	if err := kubeClient.Create(ctx, dgd); err != nil {
		logger.Error("failed to create DynamoGraphDeployment",
			zap.Error(err),
			zap.String("name", inferenceServer.Name),
			zap.String("namespace", inferenceServer.Namespace))
		return nil, fmt.Errorf("failed to create DynamoGraphDeployment for %s/%s: %w",
			inferenceServer.Namespace, inferenceServer.Name, err)
	}

	logger.Info("Successfully created DynamoGraphDeployment",
		zap.String("name", generateDynamoDGDName(inferenceServer.Name)),
		zap.String("namespace", inferenceServer.Namespace))

	return &ServerStatus{
		State:     v2pb.INFERENCE_SERVER_STATE_CREATING,
		Endpoints: []string{b.generateDynamoEndpoint(inferenceServer.Name, inferenceServer.Namespace)},
	}, nil
}

// GetServerStatus queries the DynamoGraphDeployment status and maps it to InferenceServerState.
func (b *dynamoBackend) GetServerStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (*ServerStatus, error) {
	dgdName := generateDynamoDGDName(inferenceServerName)

	// Get the DynamoGraphDeployment
	dgd := &unstructured.Unstructured{}
	dgd.SetGroupVersionKind(dynamoGVK)

	if err := kubeClient.Get(ctx, client.ObjectKey{Name: dgdName, Namespace: namespace}, dgd); err != nil {
		if errors.IsNotFound(err) {
			return &ServerStatus{
				State: v2pb.INFERENCE_SERVER_STATE_CREATE_PENDING,
			}, nil
		}
		return nil, fmt.Errorf("failed to get DynamoGraphDeployment %s/%s: %w", namespace, dgdName, err)
	}

	// Extract status from the DynamoGraphDeployment
	state := b.extractStateFromDGD(logger, dgd)

	return &ServerStatus{
		State:     state,
		Endpoints: []string{b.generateDynamoEndpoint(inferenceServerName, namespace)},
	}, nil
}

// DeleteServer deletes the DynamoGraphDeployment CR, which triggers the Dynamo operator
// to clean up all associated resources.
func (b *dynamoBackend) DeleteServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) error {
	dgdName := generateDynamoDGDName(inferenceServerName)
	logger.Info("Deleting DynamoGraphDeployment",
		zap.String("name", dgdName),
		zap.String("namespace", namespace))

	dgd := &unstructured.Unstructured{}
	dgd.SetGroupVersionKind(dynamoGVK)
	dgd.SetName(dgdName)
	dgd.SetNamespace(namespace)

	if err := kubeClient.Delete(ctx, dgd); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("DynamoGraphDeployment not found, already deleted",
				zap.String("name", dgdName))
			return nil
		}
		logger.Warn("failed to delete DynamoGraphDeployment",
			zap.Error(err),
			zap.String("name", dgdName),
			zap.String("namespace", namespace))
		return fmt.Errorf("failed to delete DynamoGraphDeployment %s/%s: %w", namespace, dgdName, err)
	}

	logger.Info("Successfully deleted DynamoGraphDeployment",
		zap.String("name", dgdName),
		zap.String("namespace", namespace))
	return nil
}

// IsHealthy checks if the Dynamo inference server is healthy by checking the underlying
// deployments created by the Dynamo operator.
func (b *dynamoBackend) IsHealthy(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (bool, error) {
	dgdName := generateDynamoDGDName(inferenceServerName)

	// Get the DynamoGraphDeployment
	dgd := &unstructured.Unstructured{}
	dgd.SetGroupVersionKind(dynamoGVK)

	if err := kubeClient.Get(ctx, client.ObjectKey{Name: dgdName, Namespace: namespace}, dgd); err != nil {
		if errors.IsNotFound(err) {
			logger.Debug("DynamoGraphDeployment not found",
				zap.String("name", dgdName))
			return false, nil
		}
		return false, fmt.Errorf("failed to get DynamoGraphDeployment: %w", err)
	}

	// Check the status conditions of the DGD
	status, found, err := unstructured.NestedMap(dgd.Object, "status")
	if err != nil || !found {
		logger.Debug("DynamoGraphDeployment status not found, not healthy yet",
			zap.String("name", dgdName))
		return false, nil
	}

	// Check conditions for ready state
	conditions, found, err := unstructured.NestedSlice(status, "conditions")
	if err != nil || !found {
		logger.Debug("DynamoGraphDeployment conditions not found",
			zap.String("name", dgdName))
		return false, nil
	}

	for _, c := range conditions {
		condition, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _, _ := unstructured.NestedString(condition, "type")
		condStatus, _, _ := unstructured.NestedString(condition, "status")

		if condType == "Ready" && condStatus == "True" {
			return true, nil
		}
	}

	// Also check the underlying deployments created by Dynamo
	return b.checkDynamoDeploymentsHealth(ctx, logger, kubeClient, inferenceServerName, namespace)
}

// CheckModelStatus checks if a model is available on the Dynamo inference server.
// Dynamo uses OpenAI-compatible APIs, so we check /v1/models endpoint.
func (b *dynamoBackend) CheckModelStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, httpClient *http.Client, inferenceServerName string, namespace string, modelName string) (bool, error) {
	logger.Info("Checking Dynamo model status via headless service",
		zap.String("model", modelName),
		zap.String("server", inferenceServerName))

	// Compute the headless service name from the base model name
	// Service name format: dynamo-model-{sha256(baseModelName)[:8]}
	serviceName := generateDynamoModelServiceName(defaultBaseModelName)

	// Get the Endpoints resource for this service (same name as service)
	endpoints := &corev1.Endpoints{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: namespace}, endpoints)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debug("Dynamo model endpoints not found",
				zap.String("serviceName", serviceName))
			return false, nil
		}
		return false, fmt.Errorf("failed to get endpoints %s: %w", serviceName, err)
	}

	// Count total and ready endpoints from subsets
	var totalEndpoints, readyEndpoints int
	for _, subset := range endpoints.Subsets {
		// Ready addresses
		readyEndpoints += len(subset.Addresses)
		totalEndpoints += len(subset.Addresses)
		// NotReady addresses
		totalEndpoints += len(subset.NotReadyAddresses)
	}

	logger.Debug("Dynamo model endpoint status",
		zap.String("serviceName", serviceName),
		zap.Int("totalEndpoints", totalEndpoints),
		zap.Int("readyEndpoints", readyEndpoints))

	// Model is ready if we have at least one endpoint and all endpoints are ready
	if totalEndpoints > 0 && readyEndpoints == totalEndpoints {
		logger.Info("Dynamo model endpoints are ready",
			zap.String("model", modelName),
			zap.Int("readyEndpoints", readyEndpoints))
		return true, nil
	}

	logger.Debug("Dynamo model not fully ready",
		zap.String("model", modelName),
		zap.Int("total", totalEndpoints),
		zap.Int("ready", readyEndpoints))
	return false, nil
}

// generateDynamoModelServiceName computes the headless service name for a model.
// The service name follows the pattern: dynamo-model-{sha256(baseModelName)[:8]}
func generateDynamoModelServiceName(baseModelName string) string {
	hash := sha256.Sum256([]byte(baseModelName))
	hashStr := hex.EncodeToString(hash[:4]) // First 4 bytes = 8 hex chars
	return fmt.Sprintf("dynamo-model-%s", hashStr)
}

// buildDynamoGraphDeployment creates an unstructured DynamoGraphDeployment CR.
// This uses the inline configuration format for vLLM aggregated deployment.
func (b *dynamoBackend) buildDynamoGraphDeployment(inferenceServer *v2pb.InferenceServer) *unstructured.Unstructured {
	replicas := int64(inferenceServer.Spec.InitSpec.NumInstances)
	if replicas == 0 {
		replicas = 1
	}

	// Build resource requirements from InitSpec
	gpuCount := int64(1) // Add GPU by default
	if inferenceServer.Spec.InitSpec.ResourceSpec.Gpu > 0 {
		gpuCount = int64(inferenceServer.Spec.InitSpec.ResourceSpec.Gpu)
	}

	cpuCount := "4"
	if inferenceServer.Spec.InitSpec.ResourceSpec.Cpu > 0 {
		cpuCount = fmt.Sprintf("%d", inferenceServer.Spec.InitSpec.ResourceSpec.Cpu)
	}

	memory := "4Gi"
	if inferenceServer.Spec.InitSpec.ResourceSpec.Memory != "" {
		memory = inferenceServer.Spec.InitSpec.ResourceSpec.Memory
	}

	// Default model for demo - Qwen3-0.6B is small enough for sandbox testing
	// todo: ghosharitra: remove when dynamo model is integrated.
	modelName := "Qwen/Qwen3-0.6B"

	dgd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", dynamoAPIGroup, dynamoAPIVersion),
			"kind":       dynamoKind,
			"metadata": map[string]interface{}{
				"name":      generateDynamoDGDName(inferenceServer.Name),
				"namespace": inferenceServer.Namespace,
				"labels": map[string]interface{}{
					dynamoManagedByLabel:          dynamoManagedByValue,
					"michelangelo.ai/server-name": inferenceServer.Name,
				},
			},
			"spec": map[string]interface{}{
				// Aggregated vLLM deployment configuration
				// This deploys Frontend + VllmDecodeWorker as a simple serving setup
				"services": map[string]interface{}{
					// Frontend: OpenAI-compatible HTTP server
					"Frontend": map[string]interface{}{
						"componentType": "frontend", // Required for Dynamo operator discovery
						"replicas":      int64(1),
						"resources": map[string]interface{}{
							"requests": map[string]interface{}{
								"cpu":    "2",
								"memory": "4Gi",
							},
							"limits": map[string]interface{}{
								"cpu":    "2",
								"memory": "4Gi",
							},
						},
						"extraPodSpec": map[string]interface{}{
							"mainContainer": map[string]interface{}{
								"image": defaultDynamoVLLMImage,
								// Required for GKE GPU nodes - sets CUDA library paths
								"env": []interface{}{
									map[string]interface{}{
										"name":  "LD_LIBRARY_PATH",
										"value": "/usr/local/nvidia/lib64:/usr/local/cuda/lib64",
									},
								},
							},
							// Allow scheduling on GPU-tainted nodes
							"tolerations": []interface{}{
								map[string]interface{}{
									"key":      "nvidia.com/gpu",
									"operator": "Exists",
									"effect":   "NoSchedule",
								},
							},
						},
					},
					// VllmDecodeWorker: vLLM inference worker (aggregated mode)
					"VllmDecodeWorker": map[string]interface{}{
						"componentType":    "worker", // Required for Dynamo operator discovery
						"subComponentType": "decode", // Decode worker type
						// modelRef links this worker to DynamoModel for LoRA adapter support
						"modelRef": map[string]interface{}{
							"name": modelName,
						},
						"replicas": replicas,
						"resources": map[string]interface{}{
							"requests": map[string]interface{}{
								"cpu":            cpuCount,
								"memory":         memory,
								"nvidia.com/gpu": fmt.Sprintf("%d", gpuCount),
							},
							"limits": map[string]interface{}{
								"cpu":            cpuCount,
								"memory":         memory,
								"nvidia.com/gpu": fmt.Sprintf("%d", gpuCount),
							},
						},
						"extraPodSpec": map[string]interface{}{
							"mainContainer": map[string]interface{}{
								"image": defaultDynamoVLLMImage,
								// Use command (not args) to avoid /bin/sh -c wrapper issues
								"command": []interface{}{
									"python3",
									"-m",
									"dynamo.vllm",
									fmt.Sprintf("--model=%s", modelName),
									"--connector=none", // Disable NIXL connector (requires UCX/RDMA not available on standard GKE)
									"--kv-events-config={\"enable_kv_cache_events\": false}",
								},
								// Required for GKE GPU nodes - sets CUDA library paths and GPU visibility
								"env": []interface{}{
									map[string]interface{}{
										"name":  "LD_LIBRARY_PATH",
										"value": "/usr/local/nvidia/lib64:/usr/local/cuda/lib64",
									},
									map[string]interface{}{
										"name":  "NVIDIA_VISIBLE_DEVICES",
										"value": "all",
									},
									map[string]interface{}{
										"name":  "NVIDIA_DRIVER_CAPABILITIES",
										"value": "compute,utility",
									},
								},
								// GPU resource for the container (required for nvidia driver injection)
								"resources": map[string]interface{}{
									"limits": map[string]interface{}{
										"nvidia.com/gpu": fmt.Sprintf("%d", gpuCount),
									},
									"requests": map[string]interface{}{
										"nvidia.com/gpu": fmt.Sprintf("%d", gpuCount),
									},
								},
							},
							// Allow scheduling on GPU-tainted nodes
							"tolerations": []interface{}{
								map[string]interface{}{
									"key":      "nvidia.com/gpu",
									"operator": "Exists",
									"effect":   "NoSchedule",
								},
							},
						},
					},
				},
			},
		},
	}

	return dgd
}

// LoadModel creates a DynamoModel CR to load a LoRA adapter onto the inference server.
// modelName is the identifier used in inference requests, sourcePath is the model location.
func (b *dynamoBackend) LoadModel(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string, modelName string, sourcePath string) error {
	logger.Info("Loading LoRA adapter via DynamoModel CR",
		zap.String("inferenceServer", inferenceServerName),
		zap.String("modelName", modelName),
		zap.String("baseModelName", defaultBaseModelName),
		zap.String("sourcePath", sourcePath))

	// Generate a deterministic name for the DynamoModel CR
	dynamoModelName := generateDynamoModelName(inferenceServerName, modelName)

	// Build the DynamoModel CR for LoRA type
	dynamoModel := b.buildDynamoModel(dynamoModelName, namespace, modelName, sourcePath)

	// Check if DynamoModel already exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(dynamoModelGVK)
	err := kubeClient.Get(ctx, client.ObjectKey{Name: dynamoModelName, Namespace: namespace}, existing)
	if err == nil {
		// DynamoModel already exists, update it
		dynamoModel.SetResourceVersion(existing.GetResourceVersion())
		if updateErr := kubeClient.Update(ctx, dynamoModel); updateErr != nil {
			return fmt.Errorf("failed to update DynamoModel %s: %w", dynamoModelName, updateErr)
		}
		logger.Info("Updated existing DynamoModel",
			zap.String("name", dynamoModelName),
			zap.String("modelName", modelName))
		return nil
	}

	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check existing DynamoModel %s: %w", dynamoModelName, err)
	}

	// Create the DynamoModel CR
	if err := kubeClient.Create(ctx, dynamoModel); err != nil {
		return fmt.Errorf("failed to create DynamoModel %s: %w", dynamoModelName, err)
	}

	logger.Info("Created DynamoModel CR for LoRA adapter",
		zap.String("name", dynamoModelName),
		zap.String("modelName", modelName))

	return nil
}

// UnloadModel deletes the DynamoModel CR to unload a model or adapter from the inference server.
func (b *dynamoBackend) UnloadModel(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string, modelName string) error {
	logger.Info("Unloading model via DynamoModel CR deletion",
		zap.String("inferenceServer", inferenceServerName),
		zap.String("modelName", modelName))

	// Generate the DynamoModel CR name
	dynamoModelName := generateDynamoModelName(inferenceServerName, modelName)

	// Get the existing DynamoModel
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(dynamoModelGVK)
	err := kubeClient.Get(ctx, client.ObjectKey{Name: dynamoModelName, Namespace: namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("DynamoModel not found, already unloaded",
				zap.String("name", dynamoModelName))
			return nil
		}
		return fmt.Errorf("failed to get DynamoModel %s: %w", dynamoModelName, err)
	}

	// Delete the DynamoModel CR
	if err := kubeClient.Delete(ctx, existing); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("DynamoModel already deleted",
				zap.String("name", dynamoModelName))
			return nil
		}
		return fmt.Errorf("failed to delete DynamoModel %s: %w", dynamoModelName, err)
	}

	logger.Info("Deleted DynamoModel CR",
		zap.String("name", dynamoModelName),
		zap.String("modelName", modelName))

	return nil
}

// buildDynamoModel constructs a DynamoModel CR for a LoRA adapter.
func (b *dynamoBackend) buildDynamoModel(name string, namespace string, modelName string, sourcePath string) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"modelName":     modelName,
		"baseModelName": defaultBaseModelName,
		"modelType":     "lora",
		"source": map[string]interface{}{
			"uri": sourcePath,
		},
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", dynamoAPIGroup, dynamoAPIVersion),
			"kind":       dynamoModelKind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					dynamoManagedByLabel:          dynamoManagedByValue,
					"michelangelo.ai/server-name": name,
				},
			},
			"spec": spec,
		},
	}
}

// generateDynamoModelName generates a deterministic name for a DynamoModel CR.
func generateDynamoModelName(inferenceServerName string, modelName string) string {
	// Sanitize model name for Kubernetes naming (replace / with -)
	sanitized := strings.ReplaceAll(modelName, "/", "-")
	sanitized = strings.ToLower(sanitized)
	return fmt.Sprintf("%s-%s", inferenceServerName, sanitized)
}

func (b *dynamoBackend) GetFrontEndSvc(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) (string, error) {
	return generateDynamoFrontendServiceName(inferenceServerName), nil
}

// extractStateFromDGD extracts the InferenceServerState from a DynamoGraphDeployment.
func (b *dynamoBackend) extractStateFromDGD(logger *zap.Logger, dgd *unstructured.Unstructured) v2pb.InferenceServerState {
	// Check if being deleted
	if dgd.GetDeletionTimestamp() != nil {
		return v2pb.INFERENCE_SERVER_STATE_DELETING
	}

	// Try to get status
	status, found, err := unstructured.NestedMap(dgd.Object, "status")
	if err != nil || !found {
		logger.Debug("DynamoGraphDeployment status not found, assuming creating",
			zap.String("name", dgd.GetName()))
		return v2pb.INFERENCE_SERVER_STATE_CREATING
	}

	// Check phase field if available
	phase, found, _ := unstructured.NestedString(status, "phase")
	if found {
		switch phase {
		case "Ready", "Running", "Serving":
			return v2pb.INFERENCE_SERVER_STATE_SERVING
		case "Failed", "Error":
			return v2pb.INFERENCE_SERVER_STATE_FAILED
		case "Pending", "Creating", "Progressing":
			return v2pb.INFERENCE_SERVER_STATE_CREATING
		case "Deleting", "Terminating":
			return v2pb.INFERENCE_SERVER_STATE_DELETING
		}
	}

	// Check conditions
	conditions, found, _ := unstructured.NestedSlice(status, "conditions")
	if found {
		for _, c := range conditions {
			condition, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _, _ := unstructured.NestedString(condition, "type")
			condStatus, _, _ := unstructured.NestedString(condition, "status")

			if condType == "Ready" && condStatus == "True" {
				return v2pb.INFERENCE_SERVER_STATE_SERVING
			}
			if condType == "Failed" && condStatus == "True" {
				return v2pb.INFERENCE_SERVER_STATE_FAILED
			}
		}
	}

	// Default to creating if we can't determine state
	return v2pb.INFERENCE_SERVER_STATE_CREATING
}

// checkDynamoDeploymentsHealth checks the health of deployments created by Dynamo.
// Dynamo creates deployments with specific labels set by the Dynamo operator.
func (b *dynamoBackend) checkDynamoDeploymentsHealth(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (bool, error) {
	dgdName := generateDynamoDGDName(inferenceServerName)

	// List deployments with the Dynamo operator labels
	// The Dynamo operator uses nvidia.com/dynamo-graph-deployment-name label
	deployments := &appsv1.DeploymentList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{
			"nvidia.com/dynamo-graph-deployment-name": dgdName,
		},
	}

	if err := kubeClient.List(ctx, deployments, listOpts...); err != nil {
		logger.Debug("failed to list Dynamo deployments",
			zap.Error(err),
			zap.String("dgd", dgdName))
		return false, nil
	}

	// We expect at least 2 deployments: Frontend and VllmDecodeWorker
	if len(deployments.Items) < 2 {
		logger.Debug("Not all Dynamo deployments found yet",
			zap.String("dgd", dgdName),
			zap.Int("found", len(deployments.Items)))
		return false, nil
	}

	// Check if all deployments are ready
	for _, deployment := range deployments.Items {
		if deployment.Spec.Replicas == nil {
			continue
		}
		if deployment.Status.ReadyReplicas < *deployment.Spec.Replicas {
			logger.Debug("Dynamo deployment not fully ready",
				zap.String("deployment", deployment.Name),
				zap.Int32("ready", deployment.Status.ReadyReplicas),
				zap.Int32("desired", *deployment.Spec.Replicas))
			return false, nil
		}
	}

	return true, nil
}

// generateDynamoDGDName generates the DynamoGraphDeployment name from the InferenceServer name.
func generateDynamoDGDName(inferenceServerName string) string {
	return fmt.Sprintf("dynamo-%s", inferenceServerName)
}

// generateDynamoEndpoint generates the service endpoint for a Dynamo deployment.
// The Dynamo operator creates a frontend service that we can use.
func (b *dynamoBackend) generateDynamoEndpoint(inferenceServerName string, namespace string) string {
	// Dynamo frontend service naming convention
	return fmt.Sprintf("http://%s-frontend.%s.svc.cluster.local:8000", generateDynamoDGDName(inferenceServerName), namespace)
}

// generateDynamoFrontendServiceName generates the frontend service name.
func generateDynamoFrontendServiceName(inferenceServerName string) string {
	return fmt.Sprintf("%s-frontend", generateDynamoDGDName(inferenceServerName))
}
