package backends

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ Backend = &dynamoSingleContainerBackend{}

const (
	singleContainerImage = "nvcr.io/nvidia/ai-dynamo/vllm-runtime:0.9.0"
	singleContainerModel = "Qwen/Qwen3-0.6B"

	singleContainerManagedByLabel = "app.kubernetes.io/managed-by"
	singleContainerManagedByValue = "michelangelo-single-container"
	singleContainerServerLabel    = "michelangelo.ai/inference-server"
	singleContainerComponentLabel = "michelangelo.ai/component"

	singleContainerFrontendPort = 8000
	singleContainerWorkerPort   = 9090
)

// dynamoSingleContainerBackend implements the Backend interface by deploying
// both the Dynamo frontend and decode worker in a single container/pod.
// This is useful for POC/testing scenarios where simplified deployment is preferred.
type dynamoSingleContainerBackend struct{}

func NewDynamoSingleContainerBackend() *dynamoSingleContainerBackend {
	return &dynamoSingleContainerBackend{}
}

// CreateServer creates a single Deployment with both frontend and decode worker.
func (b *dynamoSingleContainerBackend) CreateServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServer *v2pb.InferenceServer) (*ServerStatus, error) {
	logger.Info("Creating single-container Dynamo inference server",
		zap.String("name", inferenceServer.Name),
		zap.String("namespace", inferenceServer.Namespace))

	serverName := inferenceServer.Name
	namespace := inferenceServer.Namespace

	// Create ConfigMap with entrypoint script
	if err := b.createEntrypointConfigMap(ctx, logger, kubeClient, serverName, namespace); err != nil {
		return nil, fmt.Errorf("failed to create entrypoint configmap: %w", err)
	}

	// Create the combined Deployment
	if err := b.createDeployment(ctx, logger, kubeClient, serverName, namespace, inferenceServer); err != nil {
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	// Create the Service
	if err := b.createService(ctx, logger, kubeClient, serverName, namespace); err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}

	logger.Info("Successfully created single-container Dynamo inference server",
		zap.String("name", serverName),
		zap.String("namespace", namespace))

	return &ServerStatus{
		State:     v2pb.INFERENCE_SERVER_STATE_CREATING,
		Endpoints: []string{b.generateEndpoint(serverName, namespace)},
	}, nil
}

// GetServerStatus returns the current state of the inference server.
func (b *dynamoSingleContainerBackend) GetServerStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (*ServerStatus, error) {
	deployment := &appsv1.Deployment{}
	deploymentName := b.deploymentName(inferenceServerName)
	err := kubeClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: namespace}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			return &ServerStatus{
				State: v2pb.INFERENCE_SERVER_STATE_CREATE_PENDING,
			}, nil
		}
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	if deployment.DeletionTimestamp != nil {
		return &ServerStatus{
			State:     v2pb.INFERENCE_SERVER_STATE_DELETING,
			Endpoints: []string{b.generateEndpoint(inferenceServerName, namespace)},
		}, nil
	}

	healthy, err := b.IsHealthy(ctx, logger, kubeClient, inferenceServerName, namespace)
	if err != nil {
		return nil, err
	}

	state := v2pb.INFERENCE_SERVER_STATE_CREATING
	if healthy {
		state = v2pb.INFERENCE_SERVER_STATE_SERVING
	}

	return &ServerStatus{
		State:     state,
		Endpoints: []string{b.generateEndpoint(inferenceServerName, namespace)},
	}, nil
}

// DeleteServer removes all resources for the inference server.
func (b *dynamoSingleContainerBackend) DeleteServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) error {
	logger.Info("Deleting single-container Dynamo inference server",
		zap.String("name", inferenceServerName),
		zap.String("namespace", namespace))

	var errs []error

	// Delete Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.deploymentName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := kubeClient.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to delete deployment: %w", err))
	}

	// Delete Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.serviceName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := kubeClient.Delete(ctx, service); err != nil && !errors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to delete service: %w", err))
	}

	// Delete ConfigMap
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.configMapName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := kubeClient.Delete(ctx, configMap); err != nil && !errors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to delete configmap: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during deletion: %v", errs)
	}

	logger.Info("Successfully deleted single-container Dynamo inference server",
		zap.String("name", inferenceServerName),
		zap.String("namespace", namespace))
	return nil
}

// IsHealthy checks if the deployment is ready.
func (b *dynamoSingleContainerBackend) IsHealthy(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (bool, error) {
	deployment := &appsv1.Deployment{}
	err := kubeClient.Get(ctx, client.ObjectKey{
		Name:      b.deploymentName(inferenceServerName),
		Namespace: namespace,
	}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get deployment: %w", err)
	}

	if deployment.Spec.Replicas != nil &&
		deployment.Status.ReadyReplicas < *deployment.Spec.Replicas {
		logger.Debug("Deployment not ready",
			zap.Int32("ready", deployment.Status.ReadyReplicas),
			zap.Int32("desired", *deployment.Spec.Replicas))
		return false, nil
	}

	return true, nil
}

// CheckModelStatus checks if the model is ready for inference.
func (b *dynamoSingleContainerBackend) CheckModelStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, httpClient *http.Client, inferenceServerName string, namespace string, modelName string) (bool, error) {
	serviceName := b.serviceName(inferenceServerName)
	endpoints := &corev1.Endpoints{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: namespace}, endpoints)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debug("Endpoints not found", zap.String("service", serviceName))
			return false, nil
		}
		return false, fmt.Errorf("failed to get endpoints: %w", err)
	}

	var readyCount int
	for _, subset := range endpoints.Subsets {
		readyCount += len(subset.Addresses)
	}

	if readyCount > 0 {
		logger.Info("Model endpoint ready",
			zap.String("model", modelName),
			zap.Int("readyEndpoints", readyCount))
		return true, nil
	}

	return false, nil
}

// createEntrypointConfigMap creates a ConfigMap with the entrypoint script.
func (b *dynamoSingleContainerBackend) createEntrypointConfigMap(ctx context.Context, logger *zap.Logger, kubeClient client.Client, serverName string, namespace string) error {
	configMapName := b.configMapName(serverName)

	labels := map[string]string{
		singleContainerManagedByLabel: singleContainerManagedByValue,
		singleContainerServerLabel:    serverName,
		singleContainerComponentLabel: "entrypoint",
	}

	entrypointScript := b.generateEntrypointScript()

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: map[string]string{
			"entrypoint.sh": entrypointScript,
		},
	}

	existing := &corev1.ConfigMap{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			if createErr := kubeClient.Create(ctx, configMap); createErr != nil {
				return fmt.Errorf("failed to create configmap: %w", createErr)
			}
			logger.Info("Created entrypoint configmap", zap.String("name", configMapName))
			return nil
		}
		return fmt.Errorf("failed to get configmap: %w", err)
	}

	existing.Data = configMap.Data
	existing.Labels = configMap.Labels
	if updateErr := kubeClient.Update(ctx, existing); updateErr != nil {
		return fmt.Errorf("failed to update configmap: %w", updateErr)
	}

	return nil
}

// generateEntrypointScript creates the shell script that runs both frontend and decode worker
// in disaggregated mode, using file-based KV store for service discovery between them.
// Both processes share the same filesystem, so they can discover each other via /tmp/dynamo_store_kv.
func (b *dynamoSingleContainerBackend) generateEntrypointScript() string {
	script := strings.TrimSpace(`
#!/bin/bash
set -e

echo "Starting Dynamo single-container deployment (disaggregated mode)..."
echo "Model: ` + singleContainerModel + `"
echo "Using file-based KV store for service discovery"

# Create the shared KV store directory
mkdir -p /tmp/dynamo_store_kv

# Function to handle shutdown
cleanup() {
    echo "Shutting down..."
    kill $WORKER_PID 2>/dev/null || true
    exit 0
}
trap cleanup SIGTERM SIGINT

# Start decode worker in background
echo "Starting decode worker on port ` + fmt.Sprintf("%d", singleContainerWorkerPort) + `..."
python3 -m dynamo.vllm \
    --model=` + singleContainerModel + ` \
    --is-decode-worker \
    --connector=nixl &
WORKER_PID=$!

# Wait for worker to initialize and register with file-based KV store
echo "Waiting for decode worker to initialize..."
sleep 30

# Start frontend in foreground
echo "Starting frontend on port ` + fmt.Sprintf("%d", singleContainerFrontendPort) + `..."
python3 -m dynamo.frontend
`)
	return script
}

// createDeployment creates the combined Deployment.
func (b *dynamoSingleContainerBackend) createDeployment(ctx context.Context, logger *zap.Logger, kubeClient client.Client, serverName string, namespace string, inferenceServer *v2pb.InferenceServer) error {
	deploymentName := b.deploymentName(serverName)
	configMapName := b.configMapName(serverName)

	// Get configuration from InferenceServer spec
	gpuCount := int64(1)
	if inferenceServer.Spec.InitSpec.ResourceSpec.Gpu > 0 {
		gpuCount = int64(inferenceServer.Spec.InitSpec.ResourceSpec.Gpu)
	}

	cpuRequest := "4"
	if inferenceServer.Spec.InitSpec.ResourceSpec.Cpu > 0 {
		cpuRequest = fmt.Sprintf("%d", inferenceServer.Spec.InitSpec.ResourceSpec.Cpu)
	}

	memoryRequest := "16Gi"
	if inferenceServer.Spec.InitSpec.ResourceSpec.Memory != "" {
		memoryRequest = inferenceServer.Spec.InitSpec.ResourceSpec.Memory
	}

	labels := map[string]string{
		singleContainerManagedByLabel: singleContainerManagedByValue,
		singleContainerServerLabel:    serverName,
		singleContainerComponentLabel: "dynamo-combined",
		// Dynamo discovery labels
		"nvidia.com/dynamo-component-type":    "combined",
		"nvidia.com/dynamo-namespace":         "dynamo",
		"nvidia.com/dynamo-discovery-enabled": "true",
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: ptr.To(int64(60)),
					Containers: []corev1.Container{
						{
							Name:    "dynamo",
							Image:   singleContainerImage,
							Command: []string{"/bin/bash", "/scripts/entrypoint.sh"},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: singleContainerFrontendPort,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "worker",
									ContainerPort: singleContainerWorkerPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{
								// Dynamo configuration - use file-based KV store for discovery
								// Both processes share the same filesystem, so they can discover each other
								{Name: "DYN_NAMESPACE", Value: "dynamo"},
								{Name: "DYN_DISCOVERY_BACKEND", Value: "kv_store"},
								{Name: "DYN_STORE_KV", Value: "file"},
								{Name: "DYN_FILE_KV", Value: "/tmp/dynamo_store_kv"},
								{Name: "DYN_EVENT_PLANE", Value: "zmq"},
								// Pod identity (Downward API)
								{
									Name: "POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
									},
								},
								{
									Name: "POD_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
									},
								},
								{
									Name: "POD_UID",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"},
									},
								},
								// GPU configuration
								{Name: "LD_LIBRARY_PATH", Value: "/usr/local/nvidia/lib64:/usr/local/cuda/lib64"},
								{Name: "NVIDIA_VISIBLE_DEVICES", Value: "all"},
								{Name: "NVIDIA_DRIVER_CAPABILITIES", Value: "compute,utility"},
								{Name: "UCX_TLS", Value: "tcp,cuda_copy,cuda_ipc"},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/live",
										Port: intstr.FromInt(singleContainerFrontendPort),
									},
								},
								InitialDelaySeconds: 60,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								FailureThreshold:    6,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(singleContainerFrontendPort),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
							StartupProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/live",
										Port: intstr.FromInt(singleContainerFrontendPort),
									},
								},
								PeriodSeconds:    10,
								TimeoutSeconds:   5,
								FailureThreshold: 720, // 2 hours for model loading
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(cpuRequest),
									corev1.ResourceMemory: resource.MustParse(memoryRequest),
									"nvidia.com/gpu":      resource.MustParse(fmt.Sprintf("%d", gpuCount)),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(cpuRequest),
									corev1.ResourceMemory: resource.MustParse(memoryRequest),
									"nvidia.com/gpu":      resource.MustParse(fmt.Sprintf("%d", gpuCount)),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "shared-memory",
									MountPath: "/dev/shm",
								},
								{
									Name:      "entrypoint",
									MountPath: "/scripts",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "shared-memory",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									Medium:    corev1.StorageMediumMemory,
									SizeLimit: ptr.To(resource.MustParse("16Gi")),
								},
							},
						},
						{
							Name: "entrypoint",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: configMapName,
									},
									DefaultMode: ptr.To(int32(0755)),
								},
							},
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "nvidia.com/gpu",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
		},
	}

	existing := &appsv1.Deployment{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			if createErr := kubeClient.Create(ctx, deployment); createErr != nil {
				return fmt.Errorf("failed to create deployment: %w", createErr)
			}
			logger.Info("Created deployment", zap.String("name", deploymentName))
			return nil
		}
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	existing.Spec = deployment.Spec
	existing.Labels = deployment.Labels
	if updateErr := kubeClient.Update(ctx, existing); updateErr != nil {
		return fmt.Errorf("failed to update deployment: %w", updateErr)
	}

	return nil
}

// createService creates the Service for the frontend.
func (b *dynamoSingleContainerBackend) createService(ctx context.Context, logger *zap.Logger, kubeClient client.Client, serverName string, namespace string) error {
	serviceName := b.serviceName(serverName)

	labels := map[string]string{
		singleContainerManagedByLabel: singleContainerManagedByValue,
		singleContainerServerLabel:    serverName,
		singleContainerComponentLabel: "dynamo-combined",
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       singleContainerFrontendPort,
					TargetPort: intstr.FromInt(singleContainerFrontendPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}

	existing := &corev1.Service{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			if createErr := kubeClient.Create(ctx, service); createErr != nil {
				return fmt.Errorf("failed to create service: %w", createErr)
			}
			logger.Info("Created service", zap.String("name", serviceName))
			return nil
		}
		return fmt.Errorf("failed to get service: %w", err)
	}

	service.Spec.ClusterIP = existing.Spec.ClusterIP
	service.ResourceVersion = existing.ResourceVersion
	if updateErr := kubeClient.Update(ctx, existing); updateErr != nil {
		return fmt.Errorf("failed to update service: %w", updateErr)
	}

	return nil
}

// Naming helpers
func (b *dynamoSingleContainerBackend) deploymentName(serverName string) string {
	return fmt.Sprintf("dynamo-sc-%s", serverName)
}

func (b *dynamoSingleContainerBackend) serviceName(serverName string) string {
	return fmt.Sprintf("dynamo-sc-%s", serverName)
}

func (b *dynamoSingleContainerBackend) configMapName(serverName string) string {
	return fmt.Sprintf("dynamo-sc-%s-entrypoint", serverName)
}

func (b *dynamoSingleContainerBackend) generateEndpoint(serverName string, namespace string) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		b.serviceName(serverName), namespace, singleContainerFrontendPort)
}
