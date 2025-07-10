package inferenceserver

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Dynamo Infrastructure Management

func (g *gateway) createDynamoInfrastructure(ctx context.Context, logger logr.Logger, request InfrastructureRequest) (*InfrastructureResponse, error) {
	logger.Info("Creating Dynamo infrastructure", "server", request.InferenceServer.Name)

	// Create VirtualService first for fixed endpoint routing
	if err := g.createInferenceServerVirtualService(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to create VirtualService: %w", err)
	}

	// First, ensure platform dependencies are available (NATS, ETCD)
	if err := g.ensureDynamoPlatformDependencies(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to ensure platform dependencies: %w", err)
	}

	// Create DynamoGraphDeployment CRD
	if err := g.createDynamoGraphDeployment(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to create DynamoGraphDeployment: %w", err)
	}

	// Create Service for external access
	if err := g.createDynamoService(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to create Service: %w", err)
	}

	return &InfrastructureResponse{
		State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
		Message: "Dynamo infrastructure creation initiated",
		Endpoints: []string{
			fmt.Sprintf("/%s-endpoint/%s/production", request.InferenceServer.Name, request.InferenceServer.Name),
		},
		Details: map[string]interface{}{
			"backend":   "dynamo",
			"namespace": request.Namespace,
			"model":     getDynamoModelFromConfig(request),
		},
	}, nil
}

func (g *gateway) getDynamoInfrastructureStatus(ctx context.Context, logger logr.Logger, request InfrastructureStatusRequest) (*InfrastructureStatus, error) {
	logger.Info("Getting Dynamo infrastructure status", "server", request.InferenceServer)

	// Check DynamoGraphDeployment status
	gvr := schema.GroupVersionResource{
		Group:    "dynamo.ai",
		Version:  "v1",
		Resource: "dynamographdeployments",
	}

	deployment, err := g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Get(ctx, request.InferenceServer, metav1.GetOptions{})
	if err != nil {
		return &InfrastructureStatus{
			State:   v2pb.INFERENCE_SERVER_STATE_FAILED,
			Message: fmt.Sprintf("DynamoGraphDeployment not found: %v", err),
			Ready:   false,
		}, nil
	}

	// Extract status from the DynamoGraphDeployment
	status, err := g.extractDynamoStatus(deployment)
	if err != nil {
		return &InfrastructureStatus{
			State:   v2pb.INFERENCE_SERVER_STATE_FAILED,
			Message: fmt.Sprintf("Failed to extract status: %v", err),
			Ready:   false,
		}, nil
	}

	return status, nil
}

func (g *gateway) deleteDynamoInfrastructure(ctx context.Context, logger logr.Logger, request InfrastructureDeleteRequest) error {
	logger.Info("Deleting Dynamo infrastructure", "server", request.InferenceServer)

	// Delete VirtualService
	vsGvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}
	virtualServiceName := fmt.Sprintf("%s-virtualservice", request.InferenceServer)
	if err := g.dynamicClient.Resource(vsGvr).Namespace(request.Namespace).Delete(ctx, virtualServiceName, metav1.DeleteOptions{}); err != nil {
		logger.Error(err, "Failed to delete VirtualService")
	}

	// Delete DynamoGraphDeployment
	gvr := schema.GroupVersionResource{
		Group:    "dynamo.ai",
		Version:  "v1",
		Resource: "dynamographdeployments",
	}

	err := g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Delete(ctx, request.InferenceServer, metav1.DeleteOptions{})
	if err != nil {
		logger.Error(err, "Failed to delete DynamoGraphDeployment")
	}

	// Delete Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-service", request.InferenceServer),
			Namespace: request.Namespace,
		},
	}
	if err := g.kubeClient.Delete(ctx, service); err != nil {
		logger.Error(err, "Failed to delete service")
	}

	return nil
}

func (g *gateway) ensureDynamoPlatformDependencies(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	logger.Info("Ensuring Dynamo platform dependencies")

	// Check if NATS is available
	if err := g.ensureNATSDeployment(ctx, logger, request.Namespace); err != nil {
		return fmt.Errorf("failed to ensure NATS: %w", err)
	}

	// Check if ETCD is available
	if err := g.ensureETCDDeployment(ctx, logger, request.Namespace); err != nil {
		return fmt.Errorf("failed to ensure ETCD: %w", err)
	}

	return nil
}

func (g *gateway) ensureNATSDeployment(ctx context.Context, logger logr.Logger, namespace string) error {
	// Check if NATS deployment exists
	deployment := &appsv1.Deployment{}
	deploymentKey := client.ObjectKey{Name: "dynamo-platform-nats", Namespace: namespace}

	if err := g.kubeClient.Get(ctx, deploymentKey, deployment); err != nil {
		// Create NATS deployment if it doesn't exist
		logger.Info("Creating NATS deployment")
		return g.createNATSDeployment(ctx, logger, namespace)
	}

	return nil
}

func (g *gateway) ensureETCDDeployment(ctx context.Context, logger logr.Logger, namespace string) error {
	// Check if ETCD deployment exists
	deployment := &appsv1.Deployment{}
	deploymentKey := client.ObjectKey{Name: "dynamo-platform-etcd", Namespace: namespace}

	if err := g.kubeClient.Get(ctx, deploymentKey, deployment); err != nil {
		// Create ETCD deployment if it doesn't exist
		logger.Info("Creating ETCD deployment")
		return g.createETCDDeployment(ctx, logger, namespace)
	}

	return nil
}

func (g *gateway) createNATSDeployment(ctx context.Context, logger logr.Logger, namespace string) error {
	replicas := int32(1)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dynamo-platform-nats",
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "dynamo-platform-nats",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "dynamo-platform-nats",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nats",
							Image: "nats:2.10.11-alpine",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 4222, Name: "client"},
								{ContainerPort: 6222, Name: "cluster"},
								{ContainerPort: 8222, Name: "monitor"},
							},
							Args: []string{
								"--cluster", "nats://0.0.0.0:6222",
								"--http_port", "8222",
								"--port", "4222",
							},
						},
					},
				},
			},
		},
	}

	if err := g.kubeClient.Create(ctx, deployment); err != nil {
		return err
	}

	// Create NATS service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dynamo-platform-nats",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "dynamo-platform-nats",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "client",
					Port:       4222,
					TargetPort: intstr.FromInt(4222),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	return g.kubeClient.Create(ctx, service)
}

func (g *gateway) createETCDDeployment(ctx context.Context, logger logr.Logger, namespace string) error {
	replicas := int32(1)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dynamo-platform-etcd",
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "dynamo-platform-etcd",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "dynamo-platform-etcd",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "etcd",
							Image: "quay.io/coreos/etcd:v3.5.9",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 2379, Name: "client"},
								{ContainerPort: 2380, Name: "peer"},
							},
							Env: []corev1.EnvVar{
								{Name: "ETCD_NAME", Value: "dynamo-etcd"},
								{Name: "ETCD_LISTEN_CLIENT_URLS", Value: "http://0.0.0.0:2379"},
								{Name: "ETCD_ADVERTISE_CLIENT_URLS", Value: "http://0.0.0.0:2379"},
								{Name: "ETCD_LISTEN_PEER_URLS", Value: "http://0.0.0.0:2380"},
								{Name: "ETCD_INITIAL_ADVERTISE_PEER_URLS", Value: "http://0.0.0.0:2380"},
								{Name: "ETCD_INITIAL_CLUSTER", Value: "dynamo-etcd=http://0.0.0.0:2380"},
								{Name: "ETCD_INITIAL_CLUSTER_STATE", Value: "new"},
							},
						},
					},
				},
			},
		},
	}

	if err := g.kubeClient.Create(ctx, deployment); err != nil {
		return err
	}

	// Create ETCD service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dynamo-platform-etcd",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "dynamo-platform-etcd",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "client",
					Port:       2379,
					TargetPort: intstr.FromInt(2379),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	return g.kubeClient.Create(ctx, service)
}

func (g *gateway) createDynamoGraphDeployment(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	modelName := getDynamoModelFromConfig(request)
	if modelName == "" {
		modelName = "deepseek-ai/DeepSeek-R1-Distill-Llama-8B" // Default model
	}

	gpuCount := request.Resources.GPU
	if gpuCount == 0 {
		gpuCount = 1 // Default to 1 GPU
	}

	// Create DynamoGraphDeployment CRD
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "dynamo.ai/v1",
			"kind":       "DynamoGraphDeployment",
			"metadata": map[string]interface{}{
				"name":      request.InferenceServer.Name,
				"namespace": request.Namespace,
			},
			"spec": map[string]interface{}{
				"Common": map[string]interface{}{
					"model":              modelName,
					"block-size":         64,
					"max-model-len":      16384,
					"kv-transfer-config": `{"kv_connector":"DynamoNixlConnector"}`,
				},
				"Frontend": map[string]interface{}{
					"served_model_name": modelName,
					"endpoint":          "dynamo.Processor.chat/completions",
					"port":              8000,
					"ServiceArgs": map[string]interface{}{
						"workers": 1,
						"resources": map[string]interface{}{
							"cpu":    request.Resources.CPU,
							"memory": request.Resources.Memory,
						},
					},
				},
				"VllmWorker": map[string]interface{}{
					"enforce-eager":          true,
					"max-num-batched-tokens": 16384,
					"enable-prefix-caching":  true,
					"ServiceArgs": map[string]interface{}{
						"workers": 1,
						"resources": map[string]interface{}{
							"gpu":    fmt.Sprintf("%d", gpuCount),
							"cpu":    request.Resources.CPU,
							"memory": request.Resources.Memory,
						},
					},
				},
				"nats": map[string]interface{}{
					"url": "nats://dynamo-platform-nats:4222",
				},
				"etcd": map[string]interface{}{
					"url": "dynamo-platform-etcd:2379",
				},
			},
		},
	}

	gvr := schema.GroupVersionResource{
		Group:    "dynamo.ai",
		Version:  "v1",
		Resource: "dynamographdeployments",
	}

	_, err := g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
	return err
}

func (g *gateway) createDynamoService(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-service", request.InferenceServer.Name),
			Namespace: request.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"dynamo.ai/deployment": request.InferenceServer.Name,
				"dynamo.ai/component":  "Frontend",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8000),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "metrics",
					Port:       9090,
					TargetPort: intstr.FromInt(9090),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	return g.kubeClient.Create(ctx, service)
}

func (g *gateway) extractDynamoStatus(deployment *unstructured.Unstructured) (*InfrastructureStatus, error) {
	// Extract status from DynamoGraphDeployment
	status, found, err := unstructured.NestedMap(deployment.Object, "status")
	if err != nil || !found {
		return &InfrastructureStatus{
			State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
			Message: "Status not yet available",
			Ready:   false,
		}, nil
	}

	// Check conditions for overall health
	conditions, found, _ := unstructured.NestedSlice(status, "conditions")
	ready := false
	message := "Initializing"

	if found {
		for _, condition := range conditions {
			if condMap, ok := condition.(map[string]interface{}); ok {
				if condType, ok := condMap["type"].(string); ok && condType == "Ready" {
					if condStatus, ok := condMap["status"].(string); ok && condStatus == "True" {
						ready = true
						message = "All components are ready and serving"
						break
					} else if reason, ok := condMap["reason"].(string); ok {
						message = reason
					}
				}
			}
		}
	}

	state := v2pb.INFERENCE_SERVER_STATE_CREATING
	if ready {
		state = v2pb.INFERENCE_SERVER_STATE_SERVING
	}

	return &InfrastructureStatus{
		State:   state,
		Message: message,
		Ready:   ready,
		Endpoints: []string{
			fmt.Sprintf("http://%s-service.%s.svc.cluster.local:80", deployment.GetName(), deployment.GetNamespace()),
		},
	}, nil
}

// Dynamo Model Management

func (g *gateway) loadDynamoModel(ctx context.Context, logger logr.Logger, request ModelLoadRequest) error {
	logger.Info("Loading Dynamo model", "model", request.ModelName)

	// Dynamo models are loaded during deployment creation
	// The model is specified in the DynamoGraphDeployment spec
	return nil
}

func (g *gateway) checkDynamoModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (bool, error) {
	logger.Info("Checking Dynamo model status", "model", request.ModelName)

	// Check if the DynamoGraphDeployment is ready
	gvr := schema.GroupVersionResource{
		Group:    "dynamo.ai",
		Version:  "v1",
		Resource: "dynamographdeployments",
	}

	deployment, err := g.dynamicClient.Resource(gvr).Namespace(request.InferenceServer).Get(ctx, request.InferenceServer, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	status, err := g.extractDynamoStatus(deployment)
	if err != nil {
		return false, err
	}

	return status.Ready, nil
}

func (g *gateway) getDynamoModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (*ModelStatus, error) {
	logger.Info("Getting Dynamo model status", "model", request.ModelName)

	ready, err := g.checkDynamoModelStatus(ctx, logger, request)
	if err != nil {
		return &ModelStatus{
			State:   v2pb.INFERENCE_SERVER_STATE_FAILED,
			Message: fmt.Sprintf("Failed to check status: %v", err),
			Ready:   false,
		}, nil
	}

	if ready {
		return &ModelStatus{
			State:   v2pb.INFERENCE_SERVER_STATE_SERVING,
			Message: "Model is loaded and ready",
			Ready:   true,
		}, nil
	}

	return &ModelStatus{
		State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
		Message: "Model is still loading",
		Ready:   false,
	}, nil
}

func (g *gateway) isDynamoHealthy(ctx context.Context, logger logr.Logger, serverName string) (bool, error) {
	logger.Info("Checking Dynamo health", "server", serverName)

	// For now, assume healthy if DynamoGraphDeployment exists and is ready
	// In a real implementation, this would call the Dynamo health API endpoints
	return true, nil
}

// Helper functions

func getDynamoModelFromConfig(request InfrastructureRequest) string {
	if modelConfig, ok := request.Resources.ModelConfig["model"]; ok {
		return modelConfig
	}
	return ""
}

func getDynamoImageTag(tag string) string {
	if tag == "" {
		return "latest" // Default Dynamo image tag
	}
	return tag
}
