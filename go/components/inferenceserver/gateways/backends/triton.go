package backends

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Triton Infrastructure Management
type tritonBackend struct {
	kubeClient             client.Client
	dynamicClient          dynamic.Interface
	modelConfigMapProvider configmap.ModelConfigMapProvider
	serviceEndpoint        string // Base URL for inference service (e.g., "http://localhost:8889")
	logger                 *zap.Logger
}

func NewTritonBackend(kubeClient client.Client, dynamicClient dynamic.Interface, modelConfigMapProvider configmap.ModelConfigMapProvider, serviceEndpoint string, logger *zap.Logger) *tritonBackend {
	return &tritonBackend{
		kubeClient:             kubeClient,
		dynamicClient:          dynamicClient,
		modelConfigMapProvider: modelConfigMapProvider,
		serviceEndpoint:        serviceEndpoint,
		logger:                 logger,
	}
}

func (b *tritonBackend) CreateInfrastructure(ctx context.Context, logger *zap.Logger, request gateways.CreateInfrastructureRequest) (*gateways.CreateInfrastructureResponse, error) {
	logger.Info("Creating Triton infrastructure", zap.String("server", request.InferenceServer.Name))

	// Create routing resource - prefer HTTPRoute (Gateway API) over VirtualService (Istio-specific)
	// Check if Gateway API is available by trying to create HTTPRoute first
	if err := b.createInferenceServerHTTPRoute(ctx, logger, request); err != nil {
		logger.Info("HTTPRoute creation failed, falling back to VirtualService", zap.Error(err))
		// Fallback to VirtualService if HTTPRoute fails
		if err := b.createInferenceServerVirtualService(ctx, logger, request); err != nil {
			logger.Error("failed to create both HTTPRoute and VirtualService",
				zap.Error(err),
				zap.String("operation", "create_infrastructure"),
				zap.String("namespace", request.Namespace),
				zap.String("inferenceServer", request.InferenceServer.Name))
			return nil, fmt.Errorf("failed to create both HTTPRoute and VirtualService for %s/%s: %w",
				request.Namespace, request.InferenceServer.Name, err)
		}
	} else {
		logger.Info("Successfully created HTTPRoute for inference server", zap.String("server", request.InferenceServer.Name))
	}

	// Create Deployment
	if err := b.createTritonDeployment(ctx, logger, request); err != nil {
		logger.Error("failed to create Deployment",
			zap.Error(err),
			zap.String("operation", "create_infrastructure"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer.Name))
		return nil, fmt.Errorf("failed to create Deployment for %s/%s: %w",
			request.Namespace, request.InferenceServer.Name, err)
	}

	// Create Service
	if err := b.createTritonService(ctx, logger, request); err != nil {
		logger.Error("failed to create Service",
			zap.Error(err),
			zap.String("operation", "create_infrastructure"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer.Name))
		return nil, fmt.Errorf("failed to create Service for %s/%s: %w",
			request.Namespace, request.InferenceServer.Name, err)
	}

	// Create empty ConfigMap for model configuration
	if err := b.modelConfigMapProvider.CreateModelConfigMap(ctx, configmap.CreateModelConfigMapRequest{
		InferenceServer: request.InferenceServer.Name,
		Namespace:       request.Namespace,
		ModelConfigs:    []configmap.ModelConfigEntry{}, // Empty initially
	}); err != nil {
		logger.Error("failed to create ConfigMap",
			zap.Error(err),
			zap.String("operation", "create_infrastructure"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer.Name))
		return nil, fmt.Errorf("failed to create ConfigMap for %s/%s: %w",
			request.Namespace, request.InferenceServer.Name, err)
	}

	return &gateways.CreateInfrastructureResponse{
		State:     v2pb.INFERENCE_SERVER_STATE_CREATING,
		Message:   "Triton infrastructure creation initiated with empty ConfigMap",
		Endpoints: []string{fmt.Sprintf("/%s-endpoint/%s", request.InferenceServer.Name, request.InferenceServer.Name)},
		Details:   map[string]interface{}{"backend": "triton", "namespace": request.Namespace},
	}, nil
}

func (b *tritonBackend) GetInfrastructureStatus(ctx context.Context, logger *zap.Logger, request gateways.GetInfrastructureStatusRequest) (*gateways.GetInfrastructureStatusResponse, error) {
	logger.Info("Getting Triton infrastructure status", zap.String("server", request.InferenceServer))

	// Check deployment status
	deployment := &appsv1.Deployment{}
	deploymentKey := client.ObjectKey{Name: fmt.Sprintf("triton-%s", request.InferenceServer), Namespace: request.Namespace}

	if err := b.kubeClient.Get(ctx, deploymentKey, deployment); err != nil {
		// When deployment doesn't exist, return CREATING state to trigger infrastructure creation
		return &gateways.GetInfrastructureStatusResponse{
			Status: gateways.InfrastructureStatus{
				State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
				Message: fmt.Sprintf("Deployment not found, needs creation: %v", err),
				Ready:   false,
			},
		}, nil
	}

	// Check if ConfigMap exists - if not, infrastructure is incomplete
	configMapName := fmt.Sprintf("%s-model-config", request.InferenceServer)
	configMap := &corev1.ConfigMap{}
	configMapKey := client.ObjectKey{Name: configMapName, Namespace: request.Namespace}

	if err := b.kubeClient.Get(ctx, configMapKey, configMap); err != nil {
		// ConfigMap doesn't exist, infrastructure is incomplete
		logger.Info("ConfigMap not found, infrastructure incomplete", zap.String("configMap", configMapName))
		return &gateways.GetInfrastructureStatusResponse{
			Status: gateways.InfrastructureStatus{
				State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
				Message: fmt.Sprintf("ConfigMap %s not found, infrastructure incomplete", configMapName),
				Ready:   false,
			},
		}, nil
	}

	// Check if deployment is ready
	ready := deployment.Status.ReadyReplicas == deployment.Status.Replicas && deployment.Status.Replicas > 0

	state := v2pb.INFERENCE_SERVER_STATE_CREATING
	if ready {
		state = v2pb.INFERENCE_SERVER_STATE_SERVING
	}

	return &gateways.GetInfrastructureStatusResponse{
		Status: gateways.InfrastructureStatus{
			State:   state,
			Message: fmt.Sprintf("Deployment status: %d/%d replicas ready", deployment.Status.ReadyReplicas, deployment.Status.Replicas),
			Ready:   ready,
			Endpoints: []string{
				fmt.Sprintf("http://%s-inference-service.%s.svc.cluster.local:80", request.InferenceServer, request.Namespace),
			},
		},
	}, nil
}

func (b *tritonBackend) DeleteInfrastructure(ctx context.Context, logger *zap.Logger, request gateways.DeleteInfrastructureRequest) error {
	logger.Info("Deleting Triton infrastructure", zap.String("server", request.InferenceServer), zap.String("backend", request.BackendType.String()))

	// Delete HTTPRoute (Gateway API)
	httpRouteGVR := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}
	httpRouteName := fmt.Sprintf("%s-httproute", request.InferenceServer)
	if err := b.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Delete(ctx, httpRouteName, metav1.DeleteOptions{}); err != nil {
		logger.Info("Failed to delete HTTPRoute (may not exist)", zap.String("name", httpRouteName), zap.Error(err))
	} else {
		logger.Info("HTTPRoute deleted successfully", zap.String("name", httpRouteName))
	}

	// Delete VirtualService (fallback/legacy)
	virtualServiceGVR := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}
	virtualServiceName := fmt.Sprintf("%s-virtualservice", request.InferenceServer)
	if err := b.dynamicClient.Resource(virtualServiceGVR).Namespace(request.Namespace).Delete(ctx, virtualServiceName, metav1.DeleteOptions{}); err != nil {
		logger.Info("Failed to delete VirtualService (may not exist)", zap.String("name", virtualServiceName), zap.Error(err))
	} else {
		logger.Info("VirtualService deleted successfully", zap.String("name", virtualServiceName))
	}

	// Delete Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("triton-%s", request.InferenceServer),
			Namespace: request.Namespace,
		},
	}
	if err := b.kubeClient.Delete(ctx, deployment); err != nil {
		logger.Error("failed to delete deployment",
			zap.Error(err),
			zap.String("operation", "delete_infrastructure"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer))
	}

	// Delete Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-inference-service", request.InferenceServer),
			Namespace: request.Namespace,
		},
	}
	if err := b.kubeClient.Delete(ctx, service); err != nil {
		logger.Error("failed to delete service",
			zap.Error(err),
			zap.String("operation", "delete_infrastructure"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer))
	}

	// Delete ConfigMap using the gateway's ConfigMap deletion method
	if err := b.modelConfigMapProvider.DeleteModelConfigMap(ctx, configmap.DeleteModelConfigMapRequest{
		InferenceServer: request.InferenceServer,
		Namespace:       request.Namespace,
	}); err != nil {
		logger.Error("failed to delete ConfigMap",
			zap.Error(err),
			zap.String("operation", "delete_infrastructure"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer))
	} else {
		logger.Info("ConfigMap deleted successfully", zap.String("name", fmt.Sprintf("%s-model-config", request.InferenceServer)))
	}

	return nil
}

func (b *tritonBackend) IsHealthy(ctx context.Context, logger *zap.Logger, request gateways.HealthCheckRequest) (bool, error) {
	logger.Info("Checking Triton health via Kubernetes pod status", zap.String("server", request.InferenceServer))

	// Following Uber's approach: Check Kubernetes resource status instead of HTTP endpoints
	// Get the Triton deployment status from Kubernetes
	deploymentName := fmt.Sprintf("triton-%s", request.InferenceServer)
	namespace := request.Namespace

	deployment := &appsv1.Deployment{}
	err := b.kubeClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: namespace}, deployment)
	if err != nil {
		logger.Error("failed to get Triton deployment",
			zap.Error(err),
			zap.String("operation", "health_check"),
			zap.String("namespace", namespace),
			zap.String("deployment", deploymentName))
		return false, fmt.Errorf("failed to get deployment %s/%s: %w", namespace, deploymentName, err)
	}

	// Check deployment conditions following Uber's pattern
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable {
			if condition.Status == corev1.ConditionTrue {
				logger.Info("Triton deployment is available", zap.String("server", request.InferenceServer))

				// Also check if pods are ready (additional safety check)
				if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas == deployment.Status.Replicas {
					logger.Info("Triton pods are ready", zap.String("server", request.InferenceServer), zap.Int("readyReplicas", int(deployment.Status.ReadyReplicas)))
					return true, nil
				} else {
					logger.Error("Triton deployment available but pods not ready",
						zap.String("operation", "health_check"),
						zap.String("namespace", namespace),
						zap.String("server", request.InferenceServer),
						zap.Int("readyReplicas", int(deployment.Status.ReadyReplicas)),
						zap.Int("totalReplicas", int(deployment.Status.Replicas)))
					return false, fmt.Errorf("pods not ready for deployment %s/%s: %d/%d",
						namespace, deploymentName, deployment.Status.ReadyReplicas, deployment.Status.Replicas)
				}
			} else {
				logger.Error("Triton deployment not available",
					zap.String("operation", "health_check"),
					zap.String("namespace", namespace),
					zap.String("server", request.InferenceServer),
					zap.String("reason", condition.Reason),
					zap.String("message", condition.Message))
				return false, fmt.Errorf("deployment %s/%s not available: %s", namespace, deploymentName, condition.Message)
			}
		}
	}

	logger.Error("Triton deployment status unclear",
		zap.String("operation", "health_check"),
		zap.String("namespace", namespace),
		zap.String("server", request.InferenceServer))
	return false, fmt.Errorf("deployment status unclear for %s/%s", namespace, deploymentName)
}

// Triton Model Management

func (b *tritonBackend) LoadModel(ctx context.Context, logger *zap.Logger, request gateways.LoadModelRequest) error {
	logger.Info("Loading Triton model explicitly", zap.String("model", request.ModelName), zap.String("server", request.InferenceServer))

	// Use the service endpoint configured in the backend
	loadURL := fmt.Sprintf("%s/%s/v2/repository/models/%s/load", b.serviceEndpoint, request.InferenceServer, request.ModelName)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create load request body
	loadRequestBody := map[string]interface{}{}
	bodyBytes, err := json.Marshal(loadRequestBody)
	if err != nil {
		logger.Error("failed to marshal load request",
			zap.Error(err),
			zap.String("operation", "load_model"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer),
			zap.String("model", request.ModelName))
		return fmt.Errorf("failed to marshal load request for model %s on %s/%s: %w",
			request.ModelName, request.Namespace, request.InferenceServer, err)
	}

	// Make POST request to load model
	req, err := http.NewRequestWithContext(ctx, "POST", loadURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		logger.Error("failed to create load request",
			zap.Error(err),
			zap.String("operation", "load_model"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer),
			zap.String("model", request.ModelName))
		return fmt.Errorf("failed to create load request for model %s on %s/%s: %w",
			request.ModelName, request.Namespace, request.InferenceServer, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("failed to call Triton load endpoint",
			zap.Error(err),
			zap.String("operation", "load_model"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer),
			zap.String("model", request.ModelName))
		return fmt.Errorf("failed to call Triton load endpoint for model %s on %s/%s: %w",
			request.ModelName, request.Namespace, request.InferenceServer, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("Triton load failed",
			zap.String("operation", "load_model"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer),
			zap.String("model", request.ModelName),
			zap.Int("statusCode", resp.StatusCode),
			zap.String("responseBody", string(body)))
		return fmt.Errorf("Triton load failed for model %s on %s/%s with status %d: %s",
			request.ModelName, request.Namespace, request.InferenceServer, resp.StatusCode, string(body))
	}

	logger.Info("Triton model loaded successfully", zap.String("model", request.ModelName))
	return nil
}

func (b *tritonBackend) UnloadModel(ctx context.Context, logger *zap.Logger, request gateways.UnloadModelRequest) error {
	// Use the service endpoint configured in the backend
	unloadURL := fmt.Sprintf("%s/%s/v2/repository/models/%s/unload", b.serviceEndpoint, request.InferenceServer, request.ModelName)

	logger.Info("Calling Triton unload API", zap.String("url", unloadURL), zap.String("model", request.ModelName))

	// Create HTTP request to unload model
	req, err := http.NewRequestWithContext(ctx, "POST", unloadURL, nil)
	if err != nil {
		logger.Error("failed to create unload request",
			zap.Error(err),
			zap.String("operation", "unload_model"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer),
			zap.String("model", request.ModelName))
		return fmt.Errorf("failed to create unload request for model %s on %s/%s: %w",
			request.ModelName, request.Namespace, request.InferenceServer, err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("failed to call Triton unload API",
			zap.Error(err),
			zap.String("operation", "unload_model"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer),
			zap.String("model", request.ModelName))
		return fmt.Errorf("failed to call Triton unload API for model %s on %s/%s: %w",
			request.ModelName, request.Namespace, request.InferenceServer, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("Triton unload API returned non-OK status",
			zap.String("operation", "unload_model"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer),
			zap.String("model", request.ModelName),
			zap.Int("statusCode", resp.StatusCode))
		return fmt.Errorf("Triton unload API returned status %d for model %s on %s/%s",
			resp.StatusCode, request.ModelName, request.Namespace, request.InferenceServer)
	}

	logger.Info("Successfully called Triton unload API", zap.String("model", request.ModelName), zap.Int("status", resp.StatusCode))
	return nil
}

func (b *tritonBackend) CheckModelStatus(ctx context.Context, logger *zap.Logger, request gateways.CheckModelStatusRequest) (bool, error) {
	logger.Info("Checking Triton model status", zap.String("model", request.ModelName), zap.String("server", request.InferenceServer))

	// Use the service endpoint configured in the backend
	readyURL := fmt.Sprintf("%s/%s/v2/models/%s/ready", b.serviceEndpoint, request.InferenceServer, request.ModelName)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", readyURL, nil)
	if err != nil {
		logger.Error("failed to create ready request",
			zap.Error(err),
			zap.String("operation", "check_model_status"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer),
			zap.String("model", request.ModelName))
		return false, fmt.Errorf("failed to create ready request for model %s on %s/%s: %w",
			request.ModelName, request.Namespace, request.InferenceServer, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("failed to call Triton ready endpoint",
			zap.Error(err),
			zap.String("operation", "check_model_status"),
			zap.String("namespace", request.Namespace),
			zap.String("inferenceServer", request.InferenceServer),
			zap.String("model", request.ModelName))
		return false, fmt.Errorf("failed to call Triton ready endpoint for model %s on %s/%s: %w",
			request.ModelName, request.Namespace, request.InferenceServer, err)
	}
	defer resp.Body.Close()

	// Model is ready if status is 200
	ready := resp.StatusCode == http.StatusOK
	logger.Info("Triton model ready status", zap.String("model", request.ModelName), zap.Bool("ready", ready), zap.Int("statusCode", resp.StatusCode))
	return ready, nil
}

func (b *tritonBackend) createTritonDeployment(ctx context.Context, logger *zap.Logger, request gateways.CreateInfrastructureRequest) error {
	deploymentName := fmt.Sprintf("triton-%s", request.InferenceServer.Name)

	// Check if Deployment already exists
	existing := &appsv1.Deployment{}
	err := b.kubeClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: request.Namespace}, existing)
	if err == nil {
		// Deployment already exists, log and return success
		logger.Info("Deployment already exists, skipping creation", zap.String("name", deploymentName))
		return nil
	}

	replicas := request.Resources.Replicas
	if replicas == 0 {
		replicas = 1
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: request.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": deploymentName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": deploymentName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "triton",
							Image: fmt.Sprintf("nvcr.io/nvidia/tritonserver:%s", getTritonImageTag(request.Resources.ImageTag)),
							Ports: []corev1.ContainerPort{
								{ContainerPort: 8000, Name: "http"},
								{ContainerPort: 8001, Name: "grpc"},
								{ContainerPort: 8002, Name: "metrics"},
							},
							Resources: buildResourceRequirements(request.Resources),
							Args: []string{
								"tritonserver",
								"--model-store=/mnt/models",
								"--grpc-port=8001",
								"--http-port=8000",
								"--allow-grpc=true",
								"--allow-http=true",
								"--allow-metrics=true",
								"--metrics-port=8002",
								"--model-control-mode=explicit",
								"--strict-model-config=false",
								"--exit-on-error=true",
								"--log-error=true",
								"--log-warning=true",
								"--log-verbose=0",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "workdir",
									MountPath: "/mnt/models",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "workdir",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: fmt.Sprintf("/var/lib/michelangelo/models/%s", request.InferenceServer.Name),
									Type: func() *corev1.HostPathType {
										t := corev1.HostPathDirectoryOrCreate
										return &t
									}(),
								},
							},
						},
						{
							Name: "model-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-model-config", request.InferenceServer.Name),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := b.kubeClient.Create(ctx, deployment); err != nil {
		logger.Error("failed to create Triton Deployment",
			zap.Error(err),
			zap.String("operation", "create_triton_deployment"),
			zap.String("namespace", request.Namespace),
			zap.String("deployment", deploymentName))
		return fmt.Errorf("failed to create Triton Deployment %s/%s: %w",
			request.Namespace, deploymentName, err)
	}
	return nil
}

func (b *tritonBackend) createTritonService(ctx context.Context, logger *zap.Logger, request gateways.CreateInfrastructureRequest) error {
	serviceName := fmt.Sprintf("%s-inference-service", request.InferenceServer.Name)

	// Check if Service already exists
	existing := &corev1.Service{}
	err := b.kubeClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: request.Namespace}, existing)
	if err == nil {
		// Service already exists, log and return success
		logger.Info("Service already exists, skipping creation", zap.String("name", serviceName))
		return nil
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: request.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": fmt.Sprintf("triton-%s", request.InferenceServer.Name),
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8000),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "grpc",
					Port:       8001,
					TargetPort: intstr.FromInt(8001),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	if err := b.kubeClient.Create(ctx, service); err != nil {
		logger.Error("failed to create Triton Service",
			zap.Error(err),
			zap.String("operation", "create_triton_service"),
			zap.String("namespace", request.Namespace),
			zap.String("service", serviceName))
		return fmt.Errorf("failed to create Triton Service %s/%s: %w",
			request.Namespace, serviceName, err)
	}
	return nil
}

func (b *tritonBackend) createInferenceServerVirtualService(ctx context.Context, logger *zap.Logger, request gateways.CreateInfrastructureRequest) error {
	gvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	virtualServiceName := fmt.Sprintf("%s-virtualservice", request.InferenceServer.Name)

	// Check if VirtualService already exists
	_, err := b.dynamicClient.Resource(gvr).Namespace(request.Namespace).Get(ctx, virtualServiceName, metav1.GetOptions{})
	if err == nil {
		logger.Info("VirtualService already exists, skipping creation", zap.String("name", virtualServiceName))
		return nil
	}

	vs := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1beta1",
			"kind":       "VirtualService",
			"metadata": map[string]interface{}{
				"name":      virtualServiceName,
				"namespace": request.Namespace,
			},
			"spec": map[string]interface{}{
				"hosts": []string{"*"},
				"gateways": []string{
					"default/ma-gateway",
				},
				"http": []map[string]interface{}{
					{
						"match": []map[string]interface{}{
							{
								"uri": map[string]string{
									"prefix": fmt.Sprintf("/%s-endpoint/%s/",
										request.InferenceServer.Name,
										request.InferenceServer.Name),
								},
							},
						},
						"rewrite": map[string]interface{}{
							"uri": "/",
						},
						"route": []map[string]interface{}{
							{
								"destination": map[string]interface{}{
									"host": fmt.Sprintf("%s-service.%s.svc.cluster.local",
										request.InferenceServer.Name,
										request.Namespace),
									"port": map[string]int{
										"number": 80,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err = b.dynamicClient.Resource(gvr).Namespace(request.Namespace).Create(ctx, vs, metav1.CreateOptions{})
	if err != nil {
		logger.Error("failed to create VirtualService",
			zap.Error(err),
			zap.String("operation", "create_virtualservice"),
			zap.String("namespace", request.Namespace),
			zap.String("virtualService", virtualServiceName))
		return fmt.Errorf("failed to create VirtualService %s/%s: %w",
			request.Namespace, virtualServiceName, err)
	}

	logger.Info("VirtualService created successfully", zap.String("name", virtualServiceName))
	return nil
}

// createInferenceServerHTTPRoute creates a HTTPRoute for the inference server using generic Gateway API
func (b *tritonBackend) createInferenceServerHTTPRoute(ctx context.Context, logger *zap.Logger, request gateways.CreateInfrastructureRequest) error {
	gvr := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}

	httpRouteName := fmt.Sprintf("%s-httproute", request.InferenceServer.Name)

	// Check if HTTPRoute already exists
	existingHTTPRoute, err := b.dynamicClient.Resource(gvr).Namespace(request.Namespace).Get(ctx, httpRouteName, metav1.GetOptions{})
	if err == nil {
		logger.Info("HTTPRoute already exists, checking if update needed", zap.String("name", httpRouteName))

		// Check if the existing HTTPRoute has the correct configuration
		if needsHTTPRouteUpdate(existingHTTPRoute) {
			logger.Info("HTTPRoute configuration outdated, updating", zap.String("name", httpRouteName))
			return b.updateHTTPRoute(ctx, logger, existingHTTPRoute, request, gvr)
		}

		logger.Info("HTTPRoute configuration is up-to-date, skipping", zap.String("name", httpRouteName))
		return nil
	}

	hr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      httpRouteName,
				"namespace": request.Namespace,
				"labels": map[string]string{
					"app":                        "inference-server",
					"inference-server":           request.InferenceServer.Name,
					"michelangelo.ai/managed-by": "controller",
				},
			},
			"spec": map[string]interface{}{
				"parentRefs": []map[string]interface{}{
					{
						"name":      "ma-gateway",
						"namespace": "default",
					},
				},
				"rules": []map[string]interface{}{
					{
						// Baseline inference server endpoint - routes to whatever model is loaded in Triton
						"matches": []map[string]interface{}{
							{
								"path": map[string]interface{}{
									"type":  "PathPrefix",
									"value": fmt.Sprintf("/%s", request.InferenceServer.Name),
								},
							},
						},
						"filters": []map[string]interface{}{
							{
								"type": "URLRewrite",
								"urlRewrite": map[string]interface{}{
									"path": map[string]interface{}{
										"type":               "ReplacePrefixMatch",
										"replacePrefixMatch": "/",
									},
								},
							},
						},
						"backendRefs": []map[string]interface{}{
							{
								"name":   fmt.Sprintf("%s-inference-service", request.InferenceServer.Name),
								"port":   80,
								"weight": 100,
							},
						},
					},
				},
			},
		},
	}

	_, err = b.dynamicClient.Resource(gvr).Namespace(request.Namespace).Create(ctx, hr, metav1.CreateOptions{})
	if err != nil {
		logger.Error("failed to create HTTPRoute",
			zap.Error(err),
			zap.String("operation", "create_httproute"),
			zap.String("namespace", request.Namespace),
			zap.String("httpRoute", httpRouteName))
		return fmt.Errorf("failed to create HTTPRoute %s/%s: %w",
			request.Namespace, httpRouteName, err)
	}

	logger.Info("HTTPRoute created successfully", zap.String("name", httpRouteName))
	return nil
}

// updateHTTPRoute updates an existing HTTPRoute with the correct configuration
func (b *tritonBackend) updateHTTPRoute(ctx context.Context, logger *zap.Logger, existingHTTPRoute *unstructured.Unstructured, request gateways.CreateInfrastructureRequest, gvr schema.GroupVersionResource) error {
	// Delete the existing HTTPRoute and create a new one to avoid deep copy issues
	logger.Info("Deleting existing HTTPRoute to recreate with correct configuration", zap.String("name", existingHTTPRoute.GetName()))

	if err := b.dynamicClient.Resource(gvr).Namespace(request.Namespace).Delete(ctx, existingHTTPRoute.GetName(), metav1.DeleteOptions{}); err != nil {
		logger.Error("failed to delete existing HTTPRoute",
			zap.Error(err),
			zap.String("operation", "update_httproute"),
			zap.String("namespace", request.Namespace),
			zap.String("httpRoute", existingHTTPRoute.GetName()))
		return fmt.Errorf("failed to delete existing HTTPRoute %s/%s: %w",
			request.Namespace, existingHTTPRoute.GetName(), err)
	}

	// Wait a moment for deletion to complete
	time.Sleep(100 * time.Millisecond)

	// Create new HTTPRoute with baseline configuration
	logger.Info("Creating new HTTPRoute with baseline configuration", zap.String("name", existingHTTPRoute.GetName()))

	hr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      existingHTTPRoute.GetName(),
				"namespace": request.Namespace,
				"labels": map[string]string{
					"app":                        "inference-server",
					"inference-server":           request.InferenceServer.Name,
					"michelangelo.ai/managed-by": "controller",
				},
			},
			"spec": map[string]interface{}{
				"parentRefs": []map[string]interface{}{
					{
						"name":      "ma-gateway",
						"namespace": "default",
					},
				},
				"rules": []map[string]interface{}{
					{
						// Baseline inference server endpoint - routes to whatever model is loaded in Triton
						"matches": []map[string]interface{}{
							{
								"path": map[string]interface{}{
									"type":  "PathPrefix",
									"value": fmt.Sprintf("/%s", request.InferenceServer.Name),
								},
							},
						},
						"filters": []map[string]interface{}{
							{
								"type": "URLRewrite",
								"urlRewrite": map[string]interface{}{
									"path": map[string]interface{}{
										"type":               "ReplacePrefixMatch",
										"replacePrefixMatch": "/",
									},
								},
							},
						},
						"backendRefs": []map[string]interface{}{
							{
								"name":   fmt.Sprintf("%s-inference-service", request.InferenceServer.Name),
								"port":   80,
								"weight": 100,
							},
						},
					},
				},
			},
		},
	}

	_, err := b.dynamicClient.Resource(gvr).Namespace(request.Namespace).Create(ctx, hr, metav1.CreateOptions{})
	if err != nil {
		logger.Error("failed to create new HTTPRoute",
			zap.Error(err),
			zap.String("operation", "update_httproute"),
			zap.String("namespace", request.Namespace),
			zap.String("httpRoute", existingHTTPRoute.GetName()))
		return fmt.Errorf("failed to create new HTTPRoute %s/%s: %w",
			request.Namespace, existingHTTPRoute.GetName(), err)
	}

	logger.Info("HTTPRoute recreated successfully", zap.String("name", existingHTTPRoute.GetName()))
	return nil
}

func getTritonImageTag(tag string) string {
	if tag == "" {
		return "23.04-py3" // Default Triton image tag
	}
	return tag
}

func buildResourceRequirements(resources gateways.ResourceSpec) corev1.ResourceRequirements {
	requests := corev1.ResourceList{}
	limits := corev1.ResourceList{}

	if resources.CPU != "" {
		requests[corev1.ResourceCPU] = parseQuantity(resources.CPU)
		limits[corev1.ResourceCPU] = parseQuantity(resources.CPU)
	}

	if resources.Memory != "" {
		requests[corev1.ResourceMemory] = parseQuantity(resources.Memory)
		limits[corev1.ResourceMemory] = parseQuantity(resources.Memory)
	}

	if resources.GPU > 0 {
		requests["nvidia.com/gpu"] = parseQuantity(fmt.Sprintf("%d", resources.GPU))
		limits["nvidia.com/gpu"] = parseQuantity(fmt.Sprintf("%d", resources.GPU))
	}

	return corev1.ResourceRequirements{
		Requests: requests,
		Limits:   limits,
	}
}

func parseQuantity(value string) resource.Quantity {
	qty, _ := resource.ParseQuantity(value)
	return qty
}

// needsHTTPRouteUpdate checks if an existing HTTPRoute needs to be updated
func needsHTTPRouteUpdate(existingHTTPRoute *unstructured.Unstructured) bool {
	// Extract the existing rules
	rules, found, err := unstructured.NestedSlice(existingHTTPRoute.Object, "spec", "rules")
	if err != nil || !found || len(rules) == 0 {
		return true // Update if we can't parse rules
	}

	// Check health endpoint rule (first rule)
	if len(rules) > 0 {
		firstRule, ok := rules[0].(map[string]interface{})
		if !ok {
			return true
		}

		// Check if health endpoint has correct rewrite
		filters, found, _ := unstructured.NestedSlice(firstRule, "filters")
		if !found || len(filters) == 0 {
			return true
		}

		for _, filter := range filters {
			filterMap, ok := filter.(map[string]interface{})
			if !ok {
				continue
			}

			if filterType, ok := filterMap["type"]; ok && filterType == "URLRewrite" {
				urlRewrite, found, _ := unstructured.NestedMap(filterMap, "urlRewrite")
				if !found {
					return true
				}

				pathMap, found, _ := unstructured.NestedMap(urlRewrite, "path")
				if !found {
					return true
				}

				replacePrefixMatch, ok := pathMap["replacePrefixMatch"]
				if !ok {
					return true
				}

				// This is the key check - does it have the correct path?
				if replacePrefixMatch != "/v2/health/ready" {
					return true // Needs update
				}
			}
		}
	}

	return false // Configuration is up-to-date
}

// write unit tests for needsHTTPRouteUpdate, buildResourceRequirements
