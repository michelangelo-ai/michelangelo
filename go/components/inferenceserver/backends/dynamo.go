package backends

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
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

	// Default Dynamo container images from NGC
	defaultDynamoVLLMImage = "nvcr.io/nvidia/ai-dynamo/vllm-runtime:0.8.1"
	// defaultDynamoSGLangImage = "nvcr.io/nvidia/ai-dynamo/sglang-runtime:0.8.1"

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
	logger.Info("Checking Dynamo model status",
		zap.String("model", modelName),
		zap.String("server", inferenceServerName))

	// Dynamo uses OpenAI-compatible API at /v1/models
	endpoint := b.generateDynamoEndpoint(inferenceServerName, namespace)
	serviceURL := fmt.Sprintf("%s/v1/models", endpoint)

	req, err := http.NewRequestWithContext(ctx, "GET", serviceURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request for Dynamo models endpoint: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		logger.Debug("failed to reach Dynamo models endpoint",
			zap.Error(err),
			zap.String("url", serviceURL))
		return false, nil // Not an error, just not ready yet
	}
	defer resp.Body.Close()

	// If we can reach the endpoint and get 200, the server is serving
	if resp.StatusCode == http.StatusOK {
		logger.Info("Dynamo model endpoint is available",
			zap.String("model", modelName),
			zap.String("server", inferenceServerName))
		return true, nil
	}

	logger.Debug("Dynamo model not ready",
		zap.String("model", modelName),
		zap.Int("statusCode", resp.StatusCode))
	return false, nil
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
						"replicas": int64(1),
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
								"args": []interface{}{
									"python3",
									"-m",
									"dynamo.frontend",
									"--http-port=8000",
								},
							},
							// Allow scheduling on GPU-tainted nodes
							// todo: ghosharitra: this is temporary and should be removed.
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
								"args": []interface{}{
									"python3",
									"-m",
									"dynamo.vllm",
									fmt.Sprintf("--model=%s", modelName),
									"--kv-events-config={\"enable_kv_cache_events\": false}",
								},
							},
							// Allow scheduling on GPU-tainted nodes
							// todo: ghosharitra: this is temporary and should be removed.
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
// Dynamo creates deployments with specific naming patterns.
func (b *dynamoBackend) checkDynamoDeploymentsHealth(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (bool, error) {
	dgdName := generateDynamoDGDName(inferenceServerName)

	// List deployments with the Dynamo labels
	deployments := &appsv1.DeploymentList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{
			dynamoManagedByLabel: dynamoManagedByValue,
		},
	}

	if err := kubeClient.List(ctx, deployments, listOpts...); err != nil {
		logger.Debug("failed to list Dynamo deployments",
			zap.Error(err),
			zap.String("dgd", dgdName))
		return false, nil
	}

	if len(deployments.Items) == 0 {
		logger.Debug("No Dynamo deployments found yet",
			zap.String("dgd", dgdName))
		return false, nil
	}

	// Check if all deployments are ready
	for _, deployment := range deployments.Items {
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
