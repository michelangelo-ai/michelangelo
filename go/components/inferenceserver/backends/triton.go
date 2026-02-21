package backends

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ Backend = &tritonBackend{}

const (
	defaultTritonImageTag = "23.04-py3"
)

// Triton Server Management
type tritonBackend struct{}

func NewTritonBackend() *tritonBackend {
	return &tritonBackend{}
}

func (b *tritonBackend) CreateServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServer *v2pb.InferenceServer) (*ServerStatus, error) {
	// Create Deployment
	if err := b.createTritonDeployment(ctx, logger, kubeClient, inferenceServer); err != nil {
		return nil, fmt.Errorf("failed to create Deployment for %s/%s: %w",
			inferenceServer.Namespace, inferenceServer.Name, err)
	}

	// Create Service
	if err := b.createTritonService(ctx, logger, kubeClient, inferenceServer); err != nil {
		return nil, fmt.Errorf("failed to create Service for %s/%s: %w",
			inferenceServer.Namespace, inferenceServer.Name, err)
	}

	return &ServerStatus{
		State:     v2pb.INFERENCE_SERVER_STATE_CREATING,
		Endpoints: []string{fmt.Sprintf("http://%s.%s.svc.cluster.local:80", generateK8sServiceName(inferenceServer.Name), inferenceServer.Namespace)},
	}, nil
}

func (b *tritonBackend) GetServerStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (*ServerStatus, error) {
	deploymentName := generateK8sDeploymentName(inferenceServerName)

	// Check deployment exists
	deployment := &appsv1.Deployment{}
	deploymentKey := client.ObjectKey{Name: deploymentName, Namespace: namespace}

	if err := kubeClient.Get(ctx, deploymentKey, deployment); err != nil {
		// When deployment doesn't exist, return CREATE_PENDING to indicate resources need to be created
		return &ServerStatus{
			State: v2pb.INFERENCE_SERVER_STATE_CREATE_PENDING,
		}, nil
	}

	// Check if service exists
	service := &corev1.Service{}
	serviceKey := client.ObjectKey{Name: generateK8sServiceName(inferenceServerName), Namespace: namespace}

	if err := kubeClient.Get(ctx, serviceKey, service); err != nil {
		// Service doesn't exist, return CREATE_PENDING to indicate resources need to be created
		return &ServerStatus{
			State: v2pb.INFERENCE_SERVER_STATE_CREATE_PENDING,
		}, nil
	}

	// Determine state from deployment status and conditions
	state := b.getStateFromDeployment(logger, deployment, deploymentName)

	return &ServerStatus{
		State: state,
		Endpoints: []string{
			fmt.Sprintf("http://%s.%s.svc.cluster.local:80", generateK8sServiceName(inferenceServerName), namespace),
		},
	}, nil
}

func (b *tritonBackend) DeleteServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) error {
	// Delete Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateK8sDeploymentName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := kubeClient.Delete(ctx, deployment); err != nil {
		logger.Warn("failed to delete deployment",
			zap.Error(err),
			zap.String("operation", "delete_server"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName))
	}

	// Delete Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateK8sServiceName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := kubeClient.Delete(ctx, service); err != nil {
		logger.Warn("failed to delete service",
			zap.Error(err),
			zap.String("operation", "delete_server"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName))
	}

	return nil
}

func (b *tritonBackend) IsHealthy(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (bool, error) {
	// Check Kubernetes resource status instead of HTTP endpoints
	// Get the Triton deployment status from Kubernetes
	deploymentName := generateK8sDeploymentName(inferenceServerName)

	deployment := &appsv1.Deployment{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: namespace}, deployment)
	if err != nil {
		return false, fmt.Errorf("failed to get deployment %s/%s: %w", namespace, deploymentName, err)
	}

	// Check deployment conditions following Uber's pattern
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable {
			if condition.Status == corev1.ConditionTrue {
				// Also check if pods are ready (additional safety check)
				if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas == deployment.Status.Replicas {
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

func (b *tritonBackend) CheckModelStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, httpClient *http.Client, inferenceServerName string, namespace string, modelName string) (bool, error) {
	logger.Info("Checking Triton model status", zap.String("model", modelName), zap.String("server", inferenceServerName))

	// Format: http://{service-name}.{namespace}.svc.cluster.local/v2/models/{model}/ready
	serviceName := generateK8sServiceName(inferenceServerName)
	serviceURL := fmt.Sprintf("http://%s.%s.svc.cluster.local/v2/models/%s/ready", serviceName, namespace, modelName)

	req, err := http.NewRequestWithContext(ctx, "GET", serviceURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create ready request for model %s on %s/%s: %w",
			modelName, namespace, inferenceServerName, err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to call Triton ready endpoint for model %s on %s/%s: %w",
			modelName, namespace, inferenceServerName, err)
	}
	defer resp.Body.Close()

	// Model is ready if status is 200
	ready := resp.StatusCode == http.StatusOK

	if !ready {
		logger.Warn("Triton model not ready",
			zap.String("model", modelName),
			zap.String("url", serviceURL),
			zap.Int("statusCode", resp.StatusCode),
			zap.String("status", resp.Status))
	}

	return ready, nil
}

func (b *tritonBackend) createTritonDeployment(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServer *v2pb.InferenceServer) error {
	deploymentName := generateK8sDeploymentName(inferenceServer.Name)

	// Check if Deployment already exists
	existing := &appsv1.Deployment{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: inferenceServer.Namespace}, existing)
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
						// todo: ghosharitra: check if this is needed
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

	if err := kubeClient.Create(ctx, deployment); err != nil {
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

func (b *tritonBackend) createTritonService(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServer *v2pb.InferenceServer) error {
	serviceName := generateK8sServiceName(inferenceServer.Name)

	// Check if Service already exists
	existing := &corev1.Service{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: inferenceServer.Namespace}, existing)
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
				"app": generateK8sDeploymentName(inferenceServer.Name),
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

	if err := kubeClient.Create(ctx, service); err != nil {
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

// getStateFromDeployment determines the server state by checking the Kubernetes Deployment status.
func (b *tritonBackend) getStateFromDeployment(logger *zap.Logger, deployment *appsv1.Deployment, deploymentName string) v2pb.InferenceServerState {
	desiredReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		desiredReplicas = *deployment.Spec.Replicas
	}

	// Check deployment conditions
	var availableCondition, progressingCondition *appsv1.DeploymentCondition
	for i := range deployment.Status.Conditions {
		cond := &deployment.Status.Conditions[i]
		switch cond.Type {
		case appsv1.DeploymentAvailable:
			availableCondition = cond
		case appsv1.DeploymentProgressing:
			progressingCondition = cond
		}
	}

	logger.Debug("Deployment status",
		zap.String("deployment", deploymentName),
		zap.Int32("desiredReplicas", desiredReplicas),
		zap.Int32("readyReplicas", deployment.Status.ReadyReplicas),
		zap.Int32("availableReplicas", deployment.Status.AvailableReplicas),
		zap.Int32("updatedReplicas", deployment.Status.UpdatedReplicas))

	// Check if deployment has failed (Progressing condition is False with a failure reason)
	if progressingCondition != nil && progressingCondition.Status == corev1.ConditionFalse {
		if progressingCondition.Reason == "ProgressDeadlineExceeded" {
			logger.Warn("Deployment progress deadline exceeded",
				zap.String("deployment", deploymentName),
				zap.String("message", progressingCondition.Message))
			return v2pb.INFERENCE_SERVER_STATE_FAILED
		}
	}

	// Check if deployment is available and all replicas are ready
	if availableCondition != nil && availableCondition.Status == corev1.ConditionTrue {
		if deployment.Status.ReadyReplicas >= desiredReplicas && desiredReplicas > 0 {
			return v2pb.INFERENCE_SERVER_STATE_SERVING
		}
	}

	// Deployment is still progressing
	return v2pb.INFERENCE_SERVER_STATE_CREATING
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

func generateK8sDeploymentName(inferenceServerName string) string {
	return fmt.Sprintf("triton-%s", inferenceServerName)
}

func generateK8sServiceName(inferenceServerName string) string {
	return fmt.Sprintf("%s-inference-service", inferenceServerName)
}
