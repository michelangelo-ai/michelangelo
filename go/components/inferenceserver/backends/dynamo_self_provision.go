package backends

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
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

var _ Backend = &dynamoSelfProvisionBackend{}

const (
	// Default Dynamo container image
	defaultSelfProvisionImage = "nvcr.io/nvidia/ai-dynamo/vllm-runtime:0.8.1"

	// Default model for aggregated deployment
	defaultSelfProvisionModel = "Qwen/Qwen3-0.6B"

	// Labels for self-provisioned resources
	selfProvisionManagedByLabel = "app.kubernetes.io/managed-by"
	selfProvisionManagedByValue = "michelangelo-self-provision"
	selfProvisionServerLabel    = "michelangelo.ai/inference-server"
	selfProvisionComponentLabel = "michelangelo.ai/component"

	// Dynamo discovery labels (required for Dynamo runtime)
	dynamoComponentTypeLabel    = "nvidia.com/dynamo-component-type"
	dynamoSubComponentTypeLabel = "nvidia.com/dynamo-sub-component-type"
	dynamoNamespaceLabel        = "nvidia.com/dynamo-namespace"
	dynamoComponentLabel        = "nvidia.com/dynamo-component"
	dynamoBaseModelLabel        = "nvidia.com/dynamo-base-model"
	dynamoDiscoveryEnabledLabel = "nvidia.com/dynamo-discovery-enabled"

	// Ports
	frontendPort     = 8000
	workerSystemPort = 9090
)

// dynamoSelfProvisionBackend implements the Backend interface by directly
// provisioning Kubernetes resources (Deployments, Services) instead of
// relying on the Dynamo operator.
type dynamoSelfProvisionBackend struct{}

// NewDynamoSelfProvisionBackend creates a new self-provisioning Dynamo backend.
func NewDynamoSelfProvisionBackend() *dynamoSelfProvisionBackend {
	return &dynamoSelfProvisionBackend{}
}

// CreateServer creates the Kubernetes resources (Deployments, Services) for a Dynamo
// inference server directly, without using the Dynamo operator.
func (b *dynamoSelfProvisionBackend) CreateServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServer *v2pb.InferenceServer) (*ServerStatus, error) {
	logger.Info("Creating self-provisioned Dynamo inference server",
		zap.String("name", inferenceServer.Name),
		zap.String("namespace", inferenceServer.Namespace))

	serverName := inferenceServer.Name
	namespace := inferenceServer.Namespace

	// Create Frontend Deployment and Service
	if err := b.createFrontend(ctx, logger, kubeClient, serverName, namespace); err != nil {
		return nil, fmt.Errorf("failed to create frontend: %w", err)
	}

	// Create Worker Deployment and Service
	if err := b.createWorker(ctx, logger, kubeClient, serverName, namespace, inferenceServer); err != nil {
		return nil, fmt.Errorf("failed to create worker: %w", err)
	}

	logger.Info("Successfully created self-provisioned Dynamo inference server",
		zap.String("name", serverName),
		zap.String("namespace", namespace))

	return &ServerStatus{
		State:     v2pb.INFERENCE_SERVER_STATE_CREATING,
		Endpoints: []string{b.generateEndpoint(serverName, namespace)},
	}, nil
}

// GetServerStatus queries the status of the self-provisioned resources.
func (b *dynamoSelfProvisionBackend) GetServerStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (*ServerStatus, error) {
	// Check if frontend deployment exists
	frontendDeployment := &appsv1.Deployment{}
	frontendName := b.frontendDeploymentName(inferenceServerName)
	err := kubeClient.Get(ctx, client.ObjectKey{Name: frontendName, Namespace: namespace}, frontendDeployment)
	if err != nil {
		if errors.IsNotFound(err) {
			return &ServerStatus{
				State: v2pb.INFERENCE_SERVER_STATE_CREATE_PENDING,
			}, nil
		}
		return nil, fmt.Errorf("failed to get frontend deployment: %w", err)
	}

	// Check deletion
	if frontendDeployment.DeletionTimestamp != nil {
		return &ServerStatus{
			State:     v2pb.INFERENCE_SERVER_STATE_DELETING,
			Endpoints: []string{b.generateEndpoint(inferenceServerName, namespace)},
		}, nil
	}

	// Check if all deployments are ready
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

// DeleteServer deletes all self-provisioned resources for the inference server.
func (b *dynamoSelfProvisionBackend) DeleteServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) error {
	logger.Info("Deleting self-provisioned Dynamo inference server",
		zap.String("name", inferenceServerName),
		zap.String("namespace", namespace))

	var errs []error

	// Delete worker deployment
	workerDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.workerDeploymentName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := kubeClient.Delete(ctx, workerDeployment); err != nil && !errors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to delete worker deployment: %w", err))
	}

	// Delete worker headless service
	workerService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.workerServiceName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := kubeClient.Delete(ctx, workerService); err != nil && !errors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to delete worker service: %w", err))
	}

	// Delete frontend deployment
	frontendDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.frontendDeploymentName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := kubeClient.Delete(ctx, frontendDeployment); err != nil && !errors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to delete frontend deployment: %w", err))
	}

	// Delete frontend service
	frontendService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.frontendServiceName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := kubeClient.Delete(ctx, frontendService); err != nil && !errors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to delete frontend service: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during deletion: %v", errs)
	}

	logger.Info("Successfully deleted self-provisioned Dynamo inference server",
		zap.String("name", inferenceServerName),
		zap.String("namespace", namespace))
	return nil
}

// IsHealthy checks if all deployments are ready.
func (b *dynamoSelfProvisionBackend) IsHealthy(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (bool, error) {
	// Check frontend deployment
	frontendDeployment := &appsv1.Deployment{}
	err := kubeClient.Get(ctx, client.ObjectKey{
		Name:      b.frontendDeploymentName(inferenceServerName),
		Namespace: namespace,
	}, frontendDeployment)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get frontend deployment: %w", err)
	}

	if frontendDeployment.Spec.Replicas != nil &&
		frontendDeployment.Status.ReadyReplicas < *frontendDeployment.Spec.Replicas {
		logger.Debug("Frontend deployment not ready",
			zap.Int32("ready", frontendDeployment.Status.ReadyReplicas),
			zap.Int32("desired", *frontendDeployment.Spec.Replicas))
		return false, nil
	}

	// Check worker deployment
	workerDeployment := &appsv1.Deployment{}
	err = kubeClient.Get(ctx, client.ObjectKey{
		Name:      b.workerDeploymentName(inferenceServerName),
		Namespace: namespace,
	}, workerDeployment)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get worker deployment: %w", err)
	}

	if workerDeployment.Spec.Replicas != nil &&
		workerDeployment.Status.ReadyReplicas < *workerDeployment.Spec.Replicas {
		logger.Debug("Worker deployment not ready",
			zap.Int32("ready", workerDeployment.Status.ReadyReplicas),
			zap.Int32("desired", *workerDeployment.Spec.Replicas))
		return false, nil
	}

	return true, nil
}

// CheckModelStatus checks if a model is available on the inference server.
func (b *dynamoSelfProvisionBackend) CheckModelStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, httpClient *http.Client, inferenceServerName string, namespace string, modelName string) (bool, error) {
	// Check worker endpoints
	workerServiceName := b.workerServiceName(inferenceServerName)

	endpoints := &corev1.Endpoints{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: workerServiceName, Namespace: namespace}, endpoints)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debug("Worker endpoints not found", zap.String("service", workerServiceName))
			return false, nil
		}
		return false, fmt.Errorf("failed to get worker endpoints: %w", err)
	}

	var totalEndpoints, readyEndpoints int
	for _, subset := range endpoints.Subsets {
		readyEndpoints += len(subset.Addresses)
		totalEndpoints += len(subset.Addresses) + len(subset.NotReadyAddresses)
	}

	if totalEndpoints > 0 && readyEndpoints == totalEndpoints {
		logger.Info("Model endpoints ready",
			zap.String("model", modelName),
			zap.Int("readyEndpoints", readyEndpoints))
		return true, nil
	}

	return false, nil
}

// LoadModel loads a LoRA adapter (not implemented for self-provision yet).
func (b *dynamoSelfProvisionBackend) LoadModel(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string, modelName string, sourcePath string) error {
	logger.Warn("LoadModel not yet implemented for self-provision backend",
		zap.String("modelName", modelName),
		zap.String("sourcePath", sourcePath))
	// TODO: Implement direct HTTP call to worker's /v1/loras endpoint
	return nil
}

// UnloadModel unloads a LoRA adapter (not implemented for self-provision yet).
func (b *dynamoSelfProvisionBackend) UnloadModel(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string, modelName string) error {
	logger.Warn("UnloadModel not yet implemented for self-provision backend",
		zap.String("modelName", modelName))
	// TODO: Implement direct HTTP call to worker's /v1/loras endpoint
	return nil
}

// GetFrontEndSvc returns the frontend service name.
func (b *dynamoSelfProvisionBackend) GetFrontEndSvc(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) (string, error) {
	return b.frontendServiceName(inferenceServerName), nil
}

// createFrontend creates the Frontend Deployment and Service.
func (b *dynamoSelfProvisionBackend) createFrontend(ctx context.Context, logger *zap.Logger, kubeClient client.Client, serverName string, namespace string) error {
	deploymentName := b.frontendDeploymentName(serverName)
	serviceName := b.frontendServiceName(serverName)

	// Common labels
	labels := map[string]string{
		selfProvisionManagedByLabel: selfProvisionManagedByValue,
		selfProvisionServerLabel:    serverName,
		selfProvisionComponentLabel: "frontend",
		// Dynamo discovery labels
		dynamoComponentTypeLabel:    "frontend",
		dynamoNamespaceLabel:        "dynamo",
		dynamoComponentLabel:        "Frontend",
		dynamoDiscoveryEnabledLabel: "true",
	}

	// Create Frontend Deployment
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
							Name:  "main",
							Image: defaultSelfProvisionImage,
							Command: []string{
								"python3",
								"-m",
								"dynamo.frontend",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: frontendPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{
								// Dynamo configuration
								{Name: "DYN_NAMESPACE", Value: "dynamo"},
								{Name: "DYN_COMPONENT", Value: "Frontend"},
								{Name: "DYN_DISCOVERY_BACKEND", Value: "kubernetes"},
								// Use in-memory KV store (no external etcd needed for aggregated mode)
								{Name: "DYN_STORE_KV", Value: "mem"},
								{Name: "DYN_HTTP_PORT", Value: fmt.Sprintf("%d", frontendPort)},
								{Name: "DYNAMO_PORT", Value: fmt.Sprintf("%d", frontendPort)},
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
								// GKE GPU node CUDA paths
								{Name: "LD_LIBRARY_PATH", Value: "/usr/local/nvidia/lib64:/usr/local/cuda/lib64"},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/live",
										Port: intstr.FromInt(frontendPort),
									},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       10,
								TimeoutSeconds:      1,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(frontendPort),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								TimeoutSeconds:      3,
								FailureThreshold:    3,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
							},
						},
					},
					// Tolerate GPU nodes (for scheduling flexibility)
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

	if err := b.createOrUpdateDeployment(ctx, kubeClient, deployment); err != nil {
		return fmt.Errorf("failed to create frontend deployment: %w", err)
	}

	// Create Frontend Service
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
					Port:       frontendPort,
					TargetPort: intstr.FromInt(frontendPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}

	if err := b.createOrUpdateService(ctx, kubeClient, service); err != nil {
		return fmt.Errorf("failed to create frontend service: %w", err)
	}

	logger.Info("Created frontend resources",
		zap.String("deployment", deploymentName),
		zap.String("service", serviceName))

	return nil
}

// createWorker creates the vLLM Worker Deployment and Headless Service.
func (b *dynamoSelfProvisionBackend) createWorker(ctx context.Context, logger *zap.Logger, kubeClient client.Client, serverName string, namespace string, inferenceServer *v2pb.InferenceServer) error {
	deploymentName := b.workerDeploymentName(serverName)
	serviceName := b.workerServiceName(serverName)

	// Get configuration from InferenceServer spec
	replicas := int32(1)
	if inferenceServer.Spec.InitSpec.NumInstances > 0 {
		replicas = int32(inferenceServer.Spec.InitSpec.NumInstances)
	}

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

	modelName := defaultSelfProvisionModel

	// Compute base model hash for service discovery
	baseModelHash := b.computeModelHash(modelName)

	// Sanitize model name for label value (K8s labels don't allow '/')
	sanitizedModelName := b.sanitizeLabelValue(modelName)

	// Common labels
	labels := map[string]string{
		selfProvisionManagedByLabel: selfProvisionManagedByValue,
		selfProvisionServerLabel:    serverName,
		selfProvisionComponentLabel: "worker",
		// Dynamo discovery labels
		dynamoComponentTypeLabel:    "worker",
		dynamoSubComponentTypeLabel: "decode",
		dynamoNamespaceLabel:        "dynamo",
		dynamoComponentLabel:        "VllmDecodeWorker",
		dynamoBaseModelLabel:        sanitizedModelName, // Sanitized for K8s label compliance
		dynamoDiscoveryEnabledLabel: "true",
		// Base model hash for service discovery
		"nvidia.com/dynamo-base-model-hash": baseModelHash,
	}

	// Create Worker Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(replicas),
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
							Name:  "main",
							Image: defaultSelfProvisionImage,
							Command: []string{
								"python3",
								"-m",
								"dynamo.vllm",
								fmt.Sprintf("--model=%s", modelName),
								"--connector=none", // Aggregated mode, no KV transfer
								"--kv-events-config={\"enable_kv_cache_events\": false}",
								"--enable-lora",
								"--max-loras=4",
								"--max-lora-rank=64",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "system",
									ContainerPort: workerSystemPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{
								// Dynamo configuration
								{Name: "DYN_NAMESPACE", Value: "dynamo"},
								{Name: "DYN_COMPONENT", Value: "VllmDecodeWorker"},
								{Name: "DYN_DISCOVERY_BACKEND", Value: "kubernetes"},
								// Use in-memory KV store (no external etcd needed for aggregated mode)
								{Name: "DYN_STORE_KV", Value: "mem"},
								// System status server
								{Name: "DYN_SYSTEM_ENABLED", Value: "true"},
								{Name: "DYN_SYSTEM_PORT", Value: fmt.Sprintf("%d", workerSystemPort)},
								// LoRA support
								{Name: "DYN_LORA_ENABLED", Value: "true"},
								{Name: "DYN_LORA_PATH", Value: "/tmp/dynamo_loras"},
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
								// GKE GPU node configuration
								{Name: "LD_LIBRARY_PATH", Value: "/usr/local/nvidia/lib64:/usr/local/cuda/lib64"},
								{Name: "NVIDIA_VISIBLE_DEVICES", Value: "all"},
								{Name: "NVIDIA_DRIVER_CAPABILITIES", Value: "compute,utility"},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/live",
										Port: intstr.FromInt(workerSystemPort),
									},
								},
								PeriodSeconds:    5,
								TimeoutSeconds:   4,
								FailureThreshold: 1,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(workerSystemPort),
									},
								},
								PeriodSeconds:    10,
								TimeoutSeconds:   4,
								FailureThreshold: 3,
							},
							StartupProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/live",
										Port: intstr.FromInt(workerSystemPort),
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
					},
					// Tolerate GPU nodes
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

	if err := b.createOrUpdateDeployment(ctx, kubeClient, deployment); err != nil {
		return fmt.Errorf("failed to create worker deployment: %w", err)
	}

	// Create Worker Headless Service (for Kubernetes discovery)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: corev1.ClusterIPNone, // Headless service
			Ports: []corev1.ServicePort{
				{
					Name:       "system",
					Port:       workerSystemPort,
					TargetPort: intstr.FromInt(workerSystemPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}

	if err := b.createOrUpdateService(ctx, kubeClient, service); err != nil {
		return fmt.Errorf("failed to create worker service: %w", err)
	}

	logger.Info("Created worker resources",
		zap.String("deployment", deploymentName),
		zap.String("service", serviceName),
		zap.Int32("replicas", replicas))

	return nil
}

// createOrUpdateDeployment creates or updates a Deployment.
func (b *dynamoSelfProvisionBackend) createOrUpdateDeployment(ctx context.Context, kubeClient client.Client, deployment *appsv1.Deployment) error {
	existing := &appsv1.Deployment{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: deployment.Name, Namespace: deployment.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			if createErr := kubeClient.Create(ctx, deployment); createErr != nil {
				return fmt.Errorf("failed to create deployment %s: %w", deployment.Name, createErr)
			}
			return nil
		}
		return fmt.Errorf("failed to get deployment %s: %w", deployment.Name, err)
	}

	// Update existing deployment
	existing.Spec = deployment.Spec
	existing.Labels = deployment.Labels
	if updateErr := kubeClient.Update(ctx, existing); updateErr != nil {
		return fmt.Errorf("failed to update deployment %s: %w", deployment.Name, updateErr)
	}
	return nil
}

// createOrUpdateService creates or updates a Service.
func (b *dynamoSelfProvisionBackend) createOrUpdateService(ctx context.Context, kubeClient client.Client, service *corev1.Service) error {
	existing := &corev1.Service{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: service.Name, Namespace: service.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			if createErr := kubeClient.Create(ctx, service); createErr != nil {
				return fmt.Errorf("failed to create service %s: %w", service.Name, createErr)
			}
			return nil
		}
		return fmt.Errorf("failed to get service %s: %w", service.Name, err)
	}

	// Update existing service (preserve ClusterIP)
	service.Spec.ClusterIP = existing.Spec.ClusterIP
	service.ResourceVersion = existing.ResourceVersion
	if updateErr := kubeClient.Update(ctx, existing); updateErr != nil {
		return fmt.Errorf("failed to update service %s: %w", service.Name, updateErr)
	}
	return nil
}

// computeModelHash computes a short hash of the model name for service naming.
func (b *dynamoSelfProvisionBackend) computeModelHash(modelName string) string {
	hash := sha256.Sum256([]byte(modelName))
	return hex.EncodeToString(hash[:4]) // First 4 bytes = 8 hex chars
}

// sanitizeLabelValue sanitizes a string to be a valid Kubernetes label value.
// Valid label values: must be 63 characters or less, begin/end with alphanumeric,
// and only contain alphanumerics, '-', '_', and '.'.
func (b *dynamoSelfProvisionBackend) sanitizeLabelValue(value string) string {
	// Replace '/' with '-'
	sanitized := strings.ReplaceAll(value, "/", "-")

	// Replace any remaining invalid characters with '-'
	invalidChars := regexp.MustCompile(`[^a-zA-Z0-9\-_.]`)
	sanitized = invalidChars.ReplaceAllString(sanitized, "-")

	// Ensure it starts and ends with alphanumeric
	sanitized = strings.Trim(sanitized, "-_.")

	// Truncate to 63 characters max
	if len(sanitized) > 63 {
		sanitized = sanitized[:63]
		// Ensure it ends with alphanumeric after truncation
		sanitized = strings.TrimRight(sanitized, "-_.")
	}

	return sanitized
}

// Naming helper functions
func (b *dynamoSelfProvisionBackend) frontendDeploymentName(serverName string) string {
	return fmt.Sprintf("dynamo-sp-%s-frontend", serverName)
}

func (b *dynamoSelfProvisionBackend) frontendServiceName(serverName string) string {
	return fmt.Sprintf("dynamo-sp-%s-frontend", serverName)
}

func (b *dynamoSelfProvisionBackend) workerDeploymentName(serverName string) string {
	return fmt.Sprintf("dynamo-sp-%s-worker", serverName)
}

func (b *dynamoSelfProvisionBackend) workerServiceName(serverName string) string {
	return fmt.Sprintf("dynamo-sp-%s-worker", serverName)
}

func (b *dynamoSelfProvisionBackend) generateEndpoint(serverName string, namespace string) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		b.frontendServiceName(serverName), namespace, frontendPort)
}
