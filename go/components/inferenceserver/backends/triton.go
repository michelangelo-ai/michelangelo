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

	backendCommon "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
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
	clusterClient, err := b.clientFactory.GetClient(ctx, targetCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster client for cluster %s: %w", common.GenerateClusterDisplayName(targetCluster), err)
	}

	// Create Deployment in the target cluster
	if err = b.createTritonDeployment(ctx, inferenceServerName, namespace, resourceConstraints, clusterClient); err != nil {
		return nil, fmt.Errorf("failed to create Deployment for %s/%s: %w",
			namespace, inferenceServerName, err)
	}

	// Create Service in the target cluster (ClusterIP; east-west gateway handles cross-cluster routing)
	_, err = b.createTritonService(ctx, inferenceServerName, namespace, clusterClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create Service for %s/%s: %w",
			namespace, inferenceServerName, err)
	}

	// Create empty ConfigMap for model configuration in the target cluster
	if err = b.modelConfigMapProvider.CreateModelConfigMap(ctx, inferenceServerName, namespace, nil, nil, nil, targetCluster); err != nil {
		return nil, fmt.Errorf("failed to create ConfigMap for %s/%s: %w",
			namespace, inferenceServerName, err)
	}

	// Build endpoint URL based on cluster context
	endpoint := b.getServiceEndpoint(inferenceServerName, namespace, targetCluster)

	return &ServerStatus{
		ClusterState: v2pb.CLUSTER_STATE_CREATING,
		Endpoint:     endpoint,
	}, nil
}

func (b *tritonBackend) GetServerStatus(ctx context.Context, inferenceServerName, namespace string, targetCluster *v2pb.ClusterTarget) (*ServerStatus, error) {
	clusterClient, err := b.clientFactory.GetClient(ctx, targetCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster client for cluster %s: %w", common.GenerateClusterDisplayName(targetCluster), err)
	}
	endpoint := b.getServiceEndpoint(inferenceServerName, namespace, targetCluster)

	deployment := &appsv1.Deployment{}
	deploymentKey := client.ObjectKey{Name: fmt.Sprintf("triton-%s", inferenceServerName), Namespace: namespace}

	if err := clusterClient.Get(ctx, deploymentKey, deployment); err != nil {
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

	return &ServerStatus{
		ClusterState: clusterState,
		Endpoint:     endpoint,
	}, nil
}

func (b *tritonBackend) DeleteServer(ctx context.Context, inferenceServerName, namespace string, targetCluster *v2pb.ClusterTarget) error {
	b.logger.Info("Deleting Triton server", zap.String("server", inferenceServerName))
	clusterClient, err := b.clientFactory.GetClient(ctx, targetCluster)
	if err != nil {
		return fmt.Errorf("failed to get cluster client for cluster %s: %w", common.GenerateClusterDisplayName(targetCluster), err)
	}

	// Delete kubernetes deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("triton-%s", inferenceServerName),
			Namespace: namespace,
		},
	}

	if err := clusterClient.Delete(ctx, deployment); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete deployment for %s/%s: %w", namespace, inferenceServerName, err)
	}

	// Delete kubernetes service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backendCommon.GenerateInferenceServiceName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := clusterClient.Delete(ctx, service); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete service for %s/%s: %w", namespace, inferenceServerName, err)
	}

	// Delete model configmap
	if err := b.modelConfigMapProvider.DeleteModelConfigMap(ctx, inferenceServerName, namespace, targetCluster); err != nil {
		return fmt.Errorf("failed to delete ConfigMap for %s/%s: %w", namespace, inferenceServerName, err)
	}
	return nil
}

func (b *tritonBackend) IsHealthy(ctx context.Context, inferenceServerName, namespace string, targetCluster *v2pb.ClusterTarget) (bool, error) {
	clusterClient, err := b.clientFactory.GetClient(ctx, targetCluster)
	if err != nil {
		return false, fmt.Errorf("failed to get cluster client for cluster %s: %w", common.GenerateClusterDisplayName(targetCluster), err)
	}

	deploymentName := fmt.Sprintf("triton-%s", inferenceServerName)
	deployment := &appsv1.Deployment{}

	err = clusterClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: namespace}, deployment)
	if err != nil {
		return false, fmt.Errorf("failed to get deployment %s/%s in cluster %s: %w",
			namespace, deploymentName, common.GenerateClusterDisplayName(targetCluster), err)
	}

	// Check deployment conditions
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable {
			if condition.Status == corev1.ConditionTrue {
				// Also check if pods are ready
				if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas == deployment.Status.Replicas {
				} else {
					return false, nil
				}
			} else {
				return false, nil
			}
		}
	}
	return true, nil
}

// Triton Model Management
func (b *tritonBackend) CheckModelStatus(ctx context.Context, modelName string, inferenceServerName string, namespace string, targetCluster *v2pb.ClusterTarget) (bool, error) {
	// Get HTTP client with proper TLS configuration for the target cluster
	httpClient, err := b.clientFactory.GetHTTPClient(ctx, targetCluster)
	if err != nil {
		return false, fmt.Errorf("failed to get HTTP client for cluster %s: %w", common.GenerateClusterDisplayName(targetCluster), err)
	}

	modelReadyPath := fmt.Sprintf("/v2/models/%s/ready", modelName)
	serviceEndpoint := b.getServiceEndpoint(inferenceServerName, namespace, targetCluster)
	serviceURL := fmt.Sprintf("%s%s", serviceEndpoint, modelReadyPath)
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

	if ready := resp.StatusCode == http.StatusOK; !ready {
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
		// Deployment already exists
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
		return fmt.Errorf("failed to create Triton Deployment %s/%s: %w",
			namespace, deploymentName, err)
	}
	return nil
}

func (b *tritonBackend) createTritonService(ctx context.Context, inferenceServerName, namespace string, clusterClient client.Client) (*corev1.Service, error) {
	serviceName := backendCommon.GenerateInferenceServiceName(inferenceServerName)

	// Check if Service already exists in the target cluster
	existing := &corev1.Service{}
	err := clusterClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: namespace}, existing)
	if err == nil {
		// Service already exists
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
		return nil, fmt.Errorf("failed to create Triton Service %s/%s: %w",
			namespace, serviceName, err)
	}
	return service, nil
}

// getServiceEndpoint returns the appropriate service endpoint based on cluster type.
// For remote clusters (with kubernetes config), uses the Kubernetes API proxy.
// For control plane cluster (no config), uses direct in-cluster service access.
func (b *tritonBackend) getServiceEndpoint(inferenceServerName, namespace string, targetCluster *v2pb.ClusterTarget) string {
	serviceName := backendCommon.GenerateInferenceServiceName(inferenceServerName)
	if targetCluster == nil {
		return fmt.Sprintf("http://%s.%s.svc.cluster.local:80", serviceName, namespace)
	}
	// otherwise, construct url for remote cluster
	return fmt.Sprintf("%s:%s/api/v1/namespaces/%s/services/%s:http/proxy", targetCluster.GetKubernetes().GetHost(), targetCluster.GetKubernetes().GetPort(), namespace, serviceName)
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
