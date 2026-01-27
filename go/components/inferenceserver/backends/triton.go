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

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ Backend = &tritonBackend{}

const (
	defaultTritonImageTag = "23.04-py3"
)

// Triton Server Management
type tritonBackend struct {
	clientFactory          clientfactory.ClientFactory
	modelConfigMapProvider configmap.ModelConfigMapProvider
	logger                 *zap.Logger
}

func NewTritonBackend(clientFactory clientfactory.ClientFactory, modelConfigMapProvider configmap.ModelConfigMapProvider, logger *zap.Logger) *tritonBackend {
	return &tritonBackend{
		clientFactory:          clientFactory,
		modelConfigMapProvider: modelConfigMapProvider,
		logger:                 logger,
	}
}

func (b *tritonBackend) CreateServer(ctx context.Context, inferenceServerName, namespace string, resourceConstraints ResourceConstraints, targetCluster *v2pb.ClusterTarget) (*ServerStatus, error) {
	b.logger.Info("Creating Triton server",
		zap.String("server", inferenceServerName),
		zap.String("namespace", namespace),
	)

	var clusterClient client.Client
	var err error
	var host string
	var port string

	switch targetCluster.GetConfig().(type) {
	case *v2pb.ClusterTarget_Kubernetes:
		connectionSpec := targetCluster.GetKubernetes()
		host = connectionSpec.Host
		port = connectionSpec.Port
	default:
		return nil, fmt.Errorf("unsupported cluster type: %T", targetCluster.GetConfig())
	}
	clusterClient, err = b.clientFactory.GetClient(ctx, targetCluster)
	if err != nil {
		b.logger.Error("failed to get cluster client",
			zap.Error(err),
			zap.String("operation", "create_server"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName),
			zap.String("cluster", targetCluster.ClusterId))
		return nil, fmt.Errorf("failed to get cluster client for cluster %s: %w", targetCluster.ClusterId, err)
	}

	// Create Deployment in the target cluster
	if err = b.createTritonDeployment(ctx, inferenceServerName, namespace, resourceConstraints, clusterClient); err != nil {
		b.logger.Error("failed to create Deployment",
			zap.Error(err),
			zap.String("operation", "create_server"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName))
		return nil, fmt.Errorf("failed to create Deployment for %s/%s: %w",
			namespace, inferenceServerName, err)
	}

	// Create Service in the target cluster (ClusterIP; east-west gateway handles cross-cluster routing)
	_, err = b.createTritonService(ctx, inferenceServerName, namespace, clusterClient)
	if err != nil {
		b.logger.Error("failed to create Service",
			zap.Error(err),
			zap.String("operation", "create_server"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName))
		return nil, fmt.Errorf("failed to create Service for %s/%s: %w",
			namespace, inferenceServerName, err)
	}

	// Create empty ConfigMap for model configuration in the target cluster
	if err = b.modelConfigMapProvider.CreateModelConfigMap(ctx, inferenceServerName, namespace, nil, nil, nil, targetCluster); err != nil {
		b.logger.Error("failed to create ConfigMap",
			zap.Error(err),
			zap.String("operation", "create_server"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName))
		return nil, fmt.Errorf("failed to create ConfigMap for %s/%s: %w",
			namespace, inferenceServerName, err)
	}

	// Build endpoint URL based on cluster context
	endpoint := b.buildServiceEndpoint(inferenceServerName, namespace, host, port)

	return &ServerStatus{
		ClusterState: v2pb.CLUSTER_STATE_CREATING,
		Endpoint:     endpoint,
	}, nil
}

func (b *tritonBackend) GetServerStatus(ctx context.Context, inferenceServerName, namespace string, targetCluster *v2pb.ClusterTarget) (*ServerStatus, error) {
	b.logger.Info("Getting Triton server status", zap.String("server", inferenceServerName))

	var clusterClient client.Client
	var err error
	var host string
	var port string

	switch targetCluster.GetConfig().(type) {
	case *v2pb.ClusterTarget_Kubernetes:
		connectionSpec := targetCluster.GetKubernetes()
		host = connectionSpec.Host
		port = connectionSpec.Port
	default:
		return nil, fmt.Errorf("unsupported cluster type: %T", targetCluster.GetConfig())
	}

	clusterClient, err = b.clientFactory.GetClient(ctx, targetCluster)
	if err != nil {
		b.logger.Error("failed to get cluster client",
			zap.Error(err),
			zap.String("operation", "get_server_status"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName),
			zap.String("cluster", targetCluster.ClusterId))
		return nil, fmt.Errorf("failed to get cluster client for cluster %s: %w", targetCluster.ClusterId, err)
	}
	endpoint := b.buildServiceEndpoint(inferenceServerName, namespace, host, port)

	deployment := &appsv1.Deployment{}
	deploymentKey := client.ObjectKey{Name: fmt.Sprintf("triton-%s", inferenceServerName), Namespace: namespace}

	if err := clusterClient.Get(ctx, deploymentKey, deployment); err != nil {
		b.logger.Info("Deployment not found in cluster",
			zap.String("cluster", targetCluster.ClusterId),
			zap.Error(err))
		return &ServerStatus{
			ClusterState: v2pb.CLUSTER_STATE_INVALID,
			Endpoint:     endpoint,
		}, nil
	}

	clusterState := v2pb.CLUSTER_STATE_READY
	// Check if deployment is ready by comparing against desired replicas
	desiredReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		desiredReplicas = *deployment.Spec.Replicas
	}
	if clusterReady := desiredReplicas > 0 && deployment.Status.ReadyReplicas == desiredReplicas; !clusterReady {
		clusterState = v2pb.CLUSTER_STATE_CREATING
	}

	b.logger.Info("Server status determined",
		zap.String("clusterState", clusterState.String()),
		zap.String("endpoint", endpoint))

	return &ServerStatus{
		ClusterState: clusterState,
		Endpoint:     endpoint,
	}, nil
}

func (b *tritonBackend) DeleteServer(ctx context.Context, inferenceServerName, namespace string, targetCluster *v2pb.ClusterTarget) error {
	b.logger.Info("Deleting Triton server", zap.String("server", inferenceServerName))
	clusterClient, err := b.clientFactory.GetClient(ctx, targetCluster)
	if err != nil {
		b.logger.Error("failed to get cluster client",
			zap.Error(err),
			zap.String("operation", "delete_server"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName),
			zap.String("cluster", targetCluster.ClusterId))
		return fmt.Errorf("failed to get cluster client for cluster %s: %w", targetCluster.ClusterId, err)
	}

	// Delete kubernetes deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("triton-%s", inferenceServerName),
			Namespace: namespace,
		},
	}

	if err := clusterClient.Delete(ctx, deployment); client.IgnoreNotFound(err) != nil {
		b.logger.Error("failed to delete deployment",
			zap.Error(err),
			zap.String("operation", "delete_server"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName),
			zap.String("cluster", targetCluster.ClusterId))
		return fmt.Errorf("failed to delete deployment for %s/%s: %w", namespace, inferenceServerName, err)
	}

	// Delete kubernetes service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-inference-service", inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := clusterClient.Delete(ctx, service); client.IgnoreNotFound(err) != nil {
		b.logger.Error("failed to delete service",
			zap.Error(err),
			zap.String("operation", "delete_server"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName),
			zap.String("cluster", targetCluster.ClusterId))
		return fmt.Errorf("failed to delete service for %s/%s: %w", namespace, inferenceServerName, err)
	}

	// Delete model configmap
	if err := b.modelConfigMapProvider.DeleteModelConfigMap(ctx, inferenceServerName, namespace, targetCluster); err != nil {
		b.logger.Error("failed to delete ConfigMap",
			zap.Error(err),
			zap.String("operation", "delete_server"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName),
			zap.String("cluster", targetCluster.ClusterId))
		return fmt.Errorf("failed to delete ConfigMap for %s/%s: %w", namespace, inferenceServerName, err)
	}
	return nil
}

func (b *tritonBackend) IsHealthy(ctx context.Context, inferenceServerName, namespace string, targetCluster *v2pb.ClusterTarget) (bool, error) {
	b.logger.Info("Checking Triton health via Kubernetes pod status", zap.String("server", inferenceServerName))

	clusterClient, err := b.clientFactory.GetClient(ctx, targetCluster)
	if err != nil {
		b.logger.Error("failed to get cluster client",
			zap.Error(err),
			zap.String("operation", "health_check"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName),
			zap.String("cluster", targetCluster.ClusterId))
		return false, fmt.Errorf("failed to get cluster client for cluster %s: %w", targetCluster.ClusterId, err)
	}

	deploymentName := fmt.Sprintf("triton-%s", inferenceServerName)
	deployment := &appsv1.Deployment{}

	err = clusterClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: namespace}, deployment)
	if err != nil {
		b.logger.Error("failed to get Triton deployment",
			zap.Error(err),
			zap.String("operation", "health_check"),
			zap.String("namespace", namespace),
			zap.String("deployment", deploymentName),
			zap.String("cluster", targetCluster.ClusterId))
		return false, fmt.Errorf("failed to get deployment %s/%s in cluster %s: %w",
			namespace, deploymentName, targetCluster.ClusterId, err)
	}

	// Check deployment conditions
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable {
			if condition.Status == corev1.ConditionTrue {
				// Also check if pods are ready
				if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas == deployment.Status.Replicas {
					b.logger.Info("Triton pods are ready in cluster",
						zap.String("server", inferenceServerName),
						zap.Int32("readyReplicas", deployment.Status.ReadyReplicas),
						zap.String("cluster", targetCluster.ClusterId))
				} else {
					b.logger.Error("Triton deployment available but pods not ready",
						zap.String("operation", "health_check"),
						zap.String("namespace", namespace),
						zap.String("server", inferenceServerName),
						zap.Int32("readyReplicas", deployment.Status.ReadyReplicas),
						zap.Int32("totalReplicas", deployment.Status.Replicas),
						zap.String("cluster", targetCluster.ClusterId))
					return false, nil
				}
			} else {
				b.logger.Error("Triton deployment not available",
					zap.String("operation", "health_check"),
					zap.String("namespace", namespace),
					zap.String("server", inferenceServerName),
					zap.String("reason", condition.Reason),
					zap.String("message", condition.Message),
					zap.String("cluster", targetCluster.ClusterId))
				return false, nil
			}
		}
	}
	return true, nil
}

// Triton Model Management
func (b *tritonBackend) CheckModelStatus(ctx context.Context, modelName string, inferenceServerName string, namespace string, targetCluster *v2pb.ClusterTarget) (bool, error) {
	b.logger.Info("Checking Triton model status", zap.String("model", modelName), zap.String("server", inferenceServerName))

	// Get HTTP client with proper TLS configuration for the target cluster
	httpClient, err := b.clientFactory.GetHTTPClient(ctx, targetCluster)
	if err != nil {
		b.logger.Error("failed to get HTTP client for target cluster",
			zap.Error(err),
			zap.String("operation", "check_model_status"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName),
			zap.String("model", modelName),
			zap.String("cluster", targetCluster.ClusterId))
		return false, fmt.Errorf("failed to get HTTP client for cluster %s: %w", targetCluster.ClusterId, err)
	}

	k8sSpec := targetCluster.GetKubernetes()
	modelReadyPath := fmt.Sprintf("/v2/models/%s/ready", modelName)
	serviceEndpoint := b.buildServiceEndpoint(inferenceServerName, namespace, k8sSpec.Host, k8sSpec.Port)
	serviceURL := fmt.Sprintf("%s%s", serviceEndpoint, modelReadyPath)
	req, err := http.NewRequestWithContext(ctx, "GET", serviceURL, nil)
	if err != nil {
		b.logger.Error("failed to create ready request",
			zap.Error(err),
			zap.String("operation", "check_model_status"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName),
			zap.String("model", modelName),
			zap.String("cluster", targetCluster.ClusterId))
		return false, fmt.Errorf("failed to create ready request for model %s on %s/%s: %w",
			modelName, namespace, inferenceServerName, err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		b.logger.Error("failed to call Triton ready endpoint",
			zap.Error(err),
			zap.String("operation", "check_model_status"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName),
			zap.String("model", modelName),
			zap.String("url", serviceURL),
			zap.String("cluster", targetCluster.ClusterId))
		return false, fmt.Errorf("failed to call Triton ready endpoint for model %s on %s/%s: %w",
			modelName, namespace, inferenceServerName, err)
	}
	defer resp.Body.Close()

	if ready := resp.StatusCode == http.StatusOK; !ready {
		b.logger.Warn("Triton model not ready",
			zap.String("operation", "check_model_status"),
			zap.String("namespace", namespace),
			zap.String("inferenceServer", inferenceServerName),
			zap.String("model", modelName),
			zap.String("url", serviceURL),
			zap.Int("statusCode", resp.StatusCode),
			zap.String("cluster", targetCluster.ClusterId))
		return false, nil
	}

	return true, nil
}

func (b *tritonBackend) createTritonDeployment(ctx context.Context, inferenceServerName, namespace string, constraints ResourceConstraints, clusterClient client.Client) error {
	deploymentName := fmt.Sprintf("triton-%s", inferenceServerName)

	// Check if Deployment already exists in the target cluster
	existing := &appsv1.Deployment{}
	err := clusterClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: namespace}, existing)
	if err == nil {
		// Deployment already exists, log and return success
		b.logger.Info("Deployment already exists, skipping creation", zap.String("name", deploymentName))
		return nil
	}

	replicas := constraints.Replicas
	if replicas == 0 {
		replicas = int32(1)
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
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
							Resources: buildResourceRequirements(constraints),
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
									Path: fmt.Sprintf("/var/lib/michelangelo/models/%s", inferenceServerName),
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
										Name: fmt.Sprintf("%s-model-config", inferenceServerName),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := clusterClient.Create(ctx, deployment); err != nil {
		b.logger.Error("failed to create Triton Deployment",
			zap.Error(err),
			zap.String("operation", "create_triton_deployment"),
			zap.String("namespace", namespace),
			zap.String("deployment", deploymentName))
		return fmt.Errorf("failed to create Triton Deployment %s/%s: %w",
			namespace, deploymentName, err)
	}
	return nil
}

func (b *tritonBackend) createTritonService(ctx context.Context, inferenceServerName, namespace string, clusterClient client.Client) (*corev1.Service, error) {
	serviceName := fmt.Sprintf("%s-inference-service", inferenceServerName)

	// Check if Service already exists in the target cluster
	existing := &corev1.Service{}
	err := clusterClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: namespace}, existing)
	if err == nil {
		// Service already exists, return it
		b.logger.Info("Service already exists, skipping creation", zap.String("name", serviceName))
		return existing, nil
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": fmt.Sprintf("triton-%s", inferenceServerName),
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

	if err := clusterClient.Create(ctx, service); err != nil {
		b.logger.Error("failed to create Triton Service",
			zap.Error(err),
			zap.String("operation", "create_triton_service"),
			zap.String("namespace", namespace),
			zap.String("service", serviceName))
		return nil, fmt.Errorf("failed to create Triton Service %s/%s: %w",
			namespace, serviceName, err)
	}
	return service, nil
}

// buildServiceEndpoint constructs the Kubernetes API proxy URL for the service.
// Format: {host}:{port}/api/v1/namespaces/{namespace}/services/{service}:http/proxy
// Note: host should already include the scheme (e.g., "https://host.docker.internal")
func (b *tritonBackend) buildServiceEndpoint(inferenceServerName, namespace, host, port string) string {
	serviceName := fmt.Sprintf("%s-inference-service", inferenceServerName)
	return fmt.Sprintf("%s:%s/api/v1/namespaces/%s/services/%s:http/proxy",
		host, port, namespace, serviceName)
}

func buildResourceRequirements(constraints ResourceConstraints) corev1.ResourceRequirements {
	requests := corev1.ResourceList{}
	limits := corev1.ResourceList{}

	if constraints.Cpu > 0 {
		requests[corev1.ResourceCPU] = parseQuantity(fmt.Sprintf("%d", constraints.Cpu))
		limits[corev1.ResourceCPU] = parseQuantity(fmt.Sprintf("%d", constraints.Cpu))
	}

	if constraints.Memory != "" {
		requests[corev1.ResourceMemory] = parseQuantity(constraints.Memory)
		limits[corev1.ResourceMemory] = parseQuantity(constraints.Memory)
	}

	if constraints.Gpu > 0 {
		requests["nvidia.com/gpu"] = parseQuantity(fmt.Sprintf("%d", constraints.Gpu))
		limits["nvidia.com/gpu"] = parseQuantity(fmt.Sprintf("%d", constraints.Gpu))
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
