package backends

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	defaultTritonImageTag = "23.04-py3"
)

// Triton Server Management
type tritonBackend struct {
	kubeClient             client.Client
	modelConfigMapProvider configmap.ModelConfigMapProvider
	logger                 *zap.Logger
}

func NewTritonBackend(kubeClient client.Client, modelConfigMapProvider configmap.ModelConfigMapProvider, logger *zap.Logger) *tritonBackend {
	return &tritonBackend{
		kubeClient:             kubeClient,
		modelConfigMapProvider: modelConfigMapProvider,
		logger:                 logger,
	}
}

func (b *tritonBackend) CreateServer(ctx context.Context, logger *zap.Logger, inferenceServer *v2pb.InferenceServer) (*ServerStatus, error) {
	logger.Info("Creating Triton server", zap.String("server", inferenceServer.Name))

	// Create Deployment
	if err := b.createTritonDeployment(ctx, logger, inferenceServer); err != nil {
		logger.Error("failed to create Deployment",
			zap.Error(err),
			zap.String("operation", "create_server"),
			zap.String("namespace", inferenceServer.Namespace),
			zap.String("inferenceServer", inferenceServer.Name))
		return nil, fmt.Errorf("failed to create Deployment for %s/%s: %w",
			inferenceServer.Namespace, inferenceServer.Name, err)
	}

	// Create Service
	if err := b.createTritonService(ctx, logger, inferenceServer); err != nil {
		logger.Error("failed to create Service",
			zap.Error(err),
			zap.String("operation", "create_server"),
			zap.String("namespace", inferenceServer.Namespace),
			zap.String("inferenceServer", inferenceServer.Name))
		return nil, fmt.Errorf("failed to create Service for %s/%s: %w",
			inferenceServer.Namespace, inferenceServer.Name, err)
	}

	// Create empty ConfigMap for model configuration
	if err := b.modelConfigMapProvider.CreateModelConfigMap(ctx, inferenceServer.Name, inferenceServer.Namespace, nil, nil, nil); err != nil {
		logger.Error("failed to create ConfigMap",
			zap.Error(err),
			zap.String("operation", "create_server"),
			zap.String("namespace", inferenceServer.Namespace),
			zap.String("inferenceServer", inferenceServer.Name))
		return nil, fmt.Errorf("failed to create ConfigMap for %s/%s: %w",
			inferenceServer.Namespace, inferenceServer.Name, err)
	}

	return &ServerStatus{
		State:     v2pb.INFERENCE_SERVER_STATE_CREATING,
		Message:   "Triton Server creation initiated with empty ConfigMap",
		Endpoints: []string{fmt.Sprintf("http://%s-inference-service.%s.svc.cluster.local:80", inferenceServer.Name, inferenceServer.Namespace)},
	}, nil
}

func (b *tritonBackend) GetServerStatus(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) (*ServerStatus, error) {
	logger.Info("Getting Triton server status", zap.String("server", inferenceServerName))

	// Check deployment status
	deployment := &appsv1.Deployment{}
	deploymentKey := client.ObjectKey{Name: fmt.Sprintf("triton-%s", inferenceServerName), Namespace: namespace}

	if err := b.kubeClient.Get(ctx, deploymentKey, deployment); err != nil {
		// When deployment doesn't exist, return CREATING state to trigger server creation
		return &ServerStatus{
			State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
			Message: fmt.Sprintf("Deployment not found, needs creation: %v", err),
			Ready:   false,
		}, nil
	}

	// Check if ConfigMap exists
	configMapName := fmt.Sprintf("%s-model-config", inferenceServerName)
	configMap := &corev1.ConfigMap{}
	configMapKey := client.ObjectKey{Name: configMapName, Namespace: namespace}

	if err := b.kubeClient.Get(ctx, configMapKey, configMap); err != nil {
		// ConfigMap doesn't exist, server is incomplete
		logger.Info("ConfigMap not found, server incomplete", zap.String("configMap", configMapName))
		return &ServerStatus{
			State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
			Message: fmt.Sprintf("ConfigMap %s not found, server incomplete", configMapName),
			Ready:   false,
		}, nil
	}

	// Check if deployment is ready
	ready := deployment.Status.ReadyReplicas == deployment.Status.Replicas && deployment.Status.Replicas > 0

	state := v2pb.INFERENCE_SERVER_STATE_CREATING
	if ready {
		state = v2pb.INFERENCE_SERVER_STATE_SERVING
	}

	return &ServerStatus{
		State:   state,
		Message: fmt.Sprintf("Deployment status: %d/%d replicas ready", deployment.Status.ReadyReplicas, deployment.Status.Replicas),
		Ready:   ready,
		Endpoints: []string{
			fmt.Sprintf("http://%s-inference-service.%s.svc.cluster.local:80", inferenceServerName, namespace),
		},
	}, nil
}

func (b *tritonBackend) DeleteServer(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) error {
	logger.Info("Deleting Triton server", zap.String("server", inferenceServerName))

	// Delete Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("triton-%s", inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := b.kubeClient.Delete(ctx, deployment); err != nil {
		logger.Error("failed to delete deployment",
			zap.Error(err),
			zap.String("operation", "delete_server"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName))
	}

	// Delete Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-inference-service", inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := b.kubeClient.Delete(ctx, service); err != nil {
		logger.Error("failed to delete service",
			zap.Error(err),
			zap.String("operation", "delete_server"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName))
	}

	// Delete ConfigMap using the modelConfigMapProvider
	if err := b.modelConfigMapProvider.DeleteModelConfigMap(ctx, inferenceServerName, namespace); err != nil {
		logger.Error("failed to delete ConfigMap",
			zap.Error(err),
			zap.String("operation", "delete_server"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName))
	} else {
		logger.Info("ConfigMap deleted successfully", zap.String("name", fmt.Sprintf("%s-model-config", inferenceServerName)))
	}

	return nil
}

func (b *tritonBackend) IsHealthy(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) (bool, error) {
	logger.Info("Checking Triton health via Kubernetes pod status", zap.String("server", inferenceServerName))

	// Check Kubernetes resource status instead of HTTP endpoints
	// Get the Triton deployment status from Kubernetes
	deploymentName := fmt.Sprintf("triton-%s", inferenceServerName)

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
				logger.Info("Triton deployment is available", zap.String("server", inferenceServerName))

				// Also check if pods are ready (additional safety check)
				if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas == deployment.Status.Replicas {
					logger.Info("Triton pods are ready", zap.String("server", inferenceServerName), zap.Int("readyReplicas", int(deployment.Status.ReadyReplicas)))
					return true, nil
				} else {
					logger.Warn("Triton deployment available but pods not ready",
						zap.String("operation", "health_check"),
						zap.String("namespace", namespace),
						zap.String("server", inferenceServerName),
						zap.Int("readyReplicas", int(deployment.Status.ReadyReplicas)),
						zap.Int("totalReplicas", int(deployment.Status.Replicas)))
					return false, nil
				}
			} else {
				logger.Warn("Triton deployment not available",
					zap.String("operation", "health_check"),
					zap.String("namespace", namespace),
					zap.String("server", inferenceServerName),
					zap.String("reason", condition.Reason),
					zap.String("message", condition.Message))
				return false, nil
			}
		}
	}

	logger.Warn("Triton deployment status unclear",
		zap.String("operation", "health_check"),
		zap.String("namespace", namespace),
		zap.String("server", inferenceServerName))
	return false, nil
}

// Triton Model Management

func (b *tritonBackend) CheckModelStatus(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string) (bool, error) {
	logger.Info("Checking Triton model status", zap.String("model", modelName), zap.String("server", inferenceServerName))

	// Format: http://{service-name}.{namespace}.svc.cluster.local/v2/models/{model}/ready
	serviceName := fmt.Sprintf("%s-inference-service", inferenceServerName)
	serviceURL := fmt.Sprintf("http://%s.%s.svc.cluster.local/v2/models/%s/ready", serviceName, namespace, modelName)

	logger.Info("Checking Triton model ready endpoint via direct service access",
		zap.String("url", serviceURL),
		zap.String("model", modelName),
		zap.String("service", serviceName),
		zap.String("namespace", namespace),
		zap.String("server", inferenceServerName))

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", serviceURL, nil)
	if err != nil {
		logger.Error("failed to create ready request",
			zap.Error(err),
			zap.String("operation", "check_model_status"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName),
			zap.String("model", modelName))
		return false, fmt.Errorf("failed to create ready request for model %s on %s/%s: %w",
			modelName, namespace, inferenceServerName, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("failed to call Triton ready endpoint",
			zap.Error(err),
			zap.String("operation", "check_model_status"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName),
			zap.String("model", modelName),
			zap.String("url", serviceURL))
		return false, fmt.Errorf("failed to call Triton ready endpoint for model %s on %s/%s: %w",
			modelName, namespace, inferenceServerName, err)
	}
	defer resp.Body.Close()

	// Model is ready if status is 200
	ready := resp.StatusCode == http.StatusOK

	if ready {
		logger.Info("Triton model is ready",
			zap.String("model", modelName),
			zap.String("url", serviceURL),
			zap.Int("statusCode", resp.StatusCode))
	} else {
		logger.Warn("Triton model not ready",
			zap.String("model", modelName),
			zap.String("url", serviceURL),
			zap.Int("statusCode", resp.StatusCode),
			zap.String("status", resp.Status))
	}

	return ready, nil
}

func (b *tritonBackend) createTritonDeployment(ctx context.Context, logger *zap.Logger, inferenceServer *v2pb.InferenceServer) error {
	deploymentName := fmt.Sprintf("triton-%s", inferenceServer.Name)

	// Check if Deployment already exists
	existing := &appsv1.Deployment{}
	err := b.kubeClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: inferenceServer.Namespace}, existing)
	if err == nil {
		// Deployment already exists, log and return success
		logger.Info("Deployment already exists, skipping creation", zap.String("name", deploymentName))
		return nil
	}

	replicas := inferenceServer.Spec.InitSpec.NumInstances
	if replicas == 0 {
		replicas = 1
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: inferenceServer.Namespace,
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
							Image: fmt.Sprintf("nvcr.io/nvidia/tritonserver:%s", defaultTritonImageTag),
							Ports: []corev1.ContainerPort{
								{ContainerPort: 8000, Name: "http"},
								{ContainerPort: 8001, Name: "grpc"},
								{ContainerPort: 8002, Name: "metrics"},
							},
							Resources: buildResourceRequirements(inferenceServer.Spec.InitSpec),
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
									Path: fmt.Sprintf("/var/lib/michelangelo/models/%s", inferenceServer.Name),
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
										Name: fmt.Sprintf("%s-model-config", inferenceServer.Name),
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
			zap.String("namespace", inferenceServer.Namespace),
			zap.String("deployment", deploymentName))
		return fmt.Errorf("failed to create Triton Deployment %s/%s: %w",
			inferenceServer.Namespace, deploymentName, err)
	}
	return nil
}

func (b *tritonBackend) createTritonService(ctx context.Context, logger *zap.Logger, inferenceServer *v2pb.InferenceServer) error {
	serviceName := fmt.Sprintf("%s-inference-service", inferenceServer.Name)

	// Check if Service already exists
	existing := &corev1.Service{}
	err := b.kubeClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: inferenceServer.Namespace}, existing)
	if err == nil {
		// Service already exists, log and return success
		logger.Info("Service already exists, skipping creation", zap.String("name", serviceName))
		return nil
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: inferenceServer.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": fmt.Sprintf("triton-%s", inferenceServer.Name),
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
			zap.String("namespace", inferenceServer.Namespace),
			zap.String("service", serviceName))
		return fmt.Errorf("failed to create Triton Service %s/%s: %w",
			inferenceServer.Namespace, serviceName, err)
	}
	return nil
}

func buildResourceRequirements(initSpec *v2pb.InitSpec) corev1.ResourceRequirements {
	requests := corev1.ResourceList{}
	limits := corev1.ResourceList{}

	if initSpec.ResourceSpec.Cpu > 0 {
		requests[corev1.ResourceCPU] = parseQuantity(fmt.Sprintf("%d", initSpec.ResourceSpec.Cpu))
		limits[corev1.ResourceCPU] = parseQuantity(fmt.Sprintf("%d", initSpec.ResourceSpec.Cpu))
	}

	if initSpec.ResourceSpec.Memory != "" {
		requests[corev1.ResourceMemory] = parseQuantity(initSpec.ResourceSpec.Memory)
		limits[corev1.ResourceMemory] = parseQuantity(initSpec.ResourceSpec.Memory)
	}

	if initSpec.ResourceSpec.Gpu > 0 {
		requests["nvidia.com/gpu"] = parseQuantity(fmt.Sprintf("%d", initSpec.ResourceSpec.Gpu))
		limits["nvidia.com/gpu"] = parseQuantity(fmt.Sprintf("%d", initSpec.ResourceSpec.Gpu))
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
