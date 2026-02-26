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

var _ Backend = &dynamoEtcdBackend{}

const (
	defaultEtcdBackendImage = "nvcr.io/nvidia/ai-dynamo/vllm-runtime:0.9.0"
	defaultEtcdBackendModel = "Qwen/Qwen3-0.6B"

	etcdBackendManagedByValue = "michelangelo-dynamo-etcd"

	etcdImage = "quay.io/coreos/etcd:v3.5.9"
)

// dynamoEtcdBackend implements the Backend interface by directly provisioning
// Kubernetes resources (Deployments, Services) with etcd-based service discovery.
// It deploys an etcd instance, a Frontend, and Decode workers (no Prefill workers).
type dynamoEtcdBackend struct{}

// NewDynamoEtcdBackend creates a new etcd-based Dynamo backend.
func NewDynamoEtcdBackend() *dynamoEtcdBackend {
	return &dynamoEtcdBackend{}
}

// CreateServer creates the Kubernetes resources for a Dynamo inference server
// using etcd for service discovery. Deploys etcd, Frontend, and Decode workers.
func (b *dynamoEtcdBackend) CreateServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServer *v2pb.InferenceServer) (*ServerStatus, error) {
	logger.Info("Creating etcd-based Dynamo inference server",
		zap.String("name", inferenceServer.Name),
		zap.String("namespace", inferenceServer.Namespace))

	serverName := inferenceServer.Name
	namespace := inferenceServer.Namespace

	if err := b.createEtcd(ctx, logger, kubeClient, serverName, namespace); err != nil {
		return nil, fmt.Errorf("failed to create etcd: %w", err)
	}

	if err := b.createFrontend(ctx, logger, kubeClient, serverName, namespace); err != nil {
		return nil, fmt.Errorf("failed to create frontend: %w", err)
	}

	if err := b.createDecodeWorker(ctx, logger, kubeClient, serverName, namespace, inferenceServer); err != nil {
		return nil, fmt.Errorf("failed to create decode worker: %w", err)
	}

	logger.Info("Successfully created etcd-based Dynamo inference server",
		zap.String("name", serverName),
		zap.String("namespace", namespace))

	return &ServerStatus{
		State:     v2pb.INFERENCE_SERVER_STATE_CREATING,
		Endpoints: []string{b.generateEndpoint(serverName, namespace)},
	}, nil
}

// GetServerStatus queries the status of the etcd-based resources.
func (b *dynamoEtcdBackend) GetServerStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (*ServerStatus, error) {
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

	if frontendDeployment.DeletionTimestamp != nil {
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

// DeleteServer deletes all etcd-based resources for the inference server.
func (b *dynamoEtcdBackend) DeleteServer(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) error {
	logger.Info("Deleting etcd-based Dynamo inference server",
		zap.String("name", inferenceServerName),
		zap.String("namespace", namespace))

	var errs []error

	// Delete decode worker deployment
	decodeDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.decodeWorkerDeploymentName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := kubeClient.Delete(ctx, decodeDeployment); err != nil && !errors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to delete decode worker deployment: %w", err))
	}

	// Delete decode worker headless service
	decodeService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.decodeWorkerServiceName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := kubeClient.Delete(ctx, decodeService); err != nil && !errors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to delete decode worker service: %w", err))
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

	// Delete etcd deployment
	etcdDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.etcdDeploymentName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := kubeClient.Delete(ctx, etcdDeployment); err != nil && !errors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to delete etcd deployment: %w", err))
	}

	// Delete etcd service
	etcdService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.etcdServiceName(inferenceServerName),
			Namespace: namespace,
		},
	}
	if err := kubeClient.Delete(ctx, etcdService); err != nil && !errors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to delete etcd service: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during deletion: %v", errs)
	}

	logger.Info("Successfully deleted etcd-based Dynamo inference server",
		zap.String("name", inferenceServerName),
		zap.String("namespace", namespace))
	return nil
}

// IsHealthy checks if all deployments are ready (etcd, frontend, decode worker).
func (b *dynamoEtcdBackend) IsHealthy(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (bool, error) {
	// Check etcd deployment
	etcdDeployment := &appsv1.Deployment{}
	err := kubeClient.Get(ctx, client.ObjectKey{
		Name:      b.etcdDeploymentName(inferenceServerName),
		Namespace: namespace,
	}, etcdDeployment)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get etcd deployment: %w", err)
	}

	if etcdDeployment.Spec.Replicas != nil &&
		etcdDeployment.Status.ReadyReplicas < *etcdDeployment.Spec.Replicas {
		logger.Debug("Etcd deployment not ready",
			zap.Int32("ready", etcdDeployment.Status.ReadyReplicas),
			zap.Int32("desired", *etcdDeployment.Spec.Replicas))
		return false, nil
	}

	// Check frontend deployment
	frontendDeployment := &appsv1.Deployment{}
	err = kubeClient.Get(ctx, client.ObjectKey{
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

	// Check decode worker deployment
	decodeDeployment := &appsv1.Deployment{}
	err = kubeClient.Get(ctx, client.ObjectKey{
		Name:      b.decodeWorkerDeploymentName(inferenceServerName),
		Namespace: namespace,
	}, decodeDeployment)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get decode worker deployment: %w", err)
	}

	if decodeDeployment.Spec.Replicas != nil &&
		decodeDeployment.Status.ReadyReplicas < *decodeDeployment.Spec.Replicas {
		logger.Debug("Decode worker deployment not ready",
			zap.Int32("ready", decodeDeployment.Status.ReadyReplicas),
			zap.Int32("desired", *decodeDeployment.Spec.Replicas))
		return false, nil
	}

	return true, nil
}

// CheckModelStatus checks if the decode worker endpoints are ready.
func (b *dynamoEtcdBackend) CheckModelStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, httpClient *http.Client, inferenceServerName string, namespace string, modelName string) (bool, error) {
	decodeServiceName := b.decodeWorkerServiceName(inferenceServerName)
	decodeEndpoints := &corev1.Endpoints{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: decodeServiceName, Namespace: namespace}, decodeEndpoints)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debug("Decode worker endpoints not found", zap.String("service", decodeServiceName))
			return false, nil
		}
		return false, fmt.Errorf("failed to get decode worker endpoints: %w", err)
	}

	var decodeReady int
	for _, subset := range decodeEndpoints.Subsets {
		decodeReady += len(subset.Addresses)
	}

	if decodeReady > 0 {
		logger.Info("Model endpoints ready",
			zap.String("model", modelName),
			zap.Int("decodeReady", decodeReady))
		return true, nil
	}

	return false, nil
}

// LoadModel loads a LoRA adapter (not implemented for etcd backend yet).
func (b *dynamoEtcdBackend) LoadModel(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string, modelName string, sourcePath string) error {
	logger.Warn("LoadModel not yet implemented for etcd backend",
		zap.String("modelName", modelName),
		zap.String("sourcePath", sourcePath))
	return nil
}

// UnloadModel unloads a LoRA adapter (not implemented for etcd backend yet).
func (b *dynamoEtcdBackend) UnloadModel(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string, modelName string) error {
	logger.Warn("UnloadModel not yet implemented for etcd backend",
		zap.String("modelName", modelName))
	return nil
}

// GetFrontEndSvc returns the frontend service name.
func (b *dynamoEtcdBackend) GetFrontEndSvc(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) (string, error) {
	return b.frontendServiceName(inferenceServerName), nil
}

// createEtcd creates the etcd Deployment and Service for Dynamo service discovery.
func (b *dynamoEtcdBackend) createEtcd(ctx context.Context, logger *zap.Logger, kubeClient client.Client, serverName string, namespace string) error {
	deploymentName := b.etcdDeploymentName(serverName)
	serviceName := b.etcdServiceName(serverName)

	labels := map[string]string{
		selfProvisionManagedByLabel: etcdBackendManagedByValue,
		selfProvisionServerLabel:    serverName,
		selfProvisionComponentLabel: "etcd",
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
					TerminationGracePeriodSeconds: ptr.To(int64(30)),
					Containers: []corev1.Container{
						{
							Name:  "etcd",
							Image: etcdImage,
							Command: []string{
								"etcd",
								"--name=etcd0",
								"--data-dir=/etcd-data",
								fmt.Sprintf("--listen-client-urls=http://0.0.0.0:%d", etcdClientPort),
								fmt.Sprintf("--advertise-client-urls=http://%s.%s.svc.cluster.local:%d", serviceName, namespace, etcdClientPort),
								fmt.Sprintf("--listen-peer-urls=http://0.0.0.0:%d", etcdPeerPort),
								fmt.Sprintf("--initial-advertise-peer-urls=http://%s.%s.svc.cluster.local:%d", serviceName, namespace, etcdPeerPort),
								fmt.Sprintf("--initial-cluster=etcd0=http://%s.%s.svc.cluster.local:%d", serviceName, namespace, etcdPeerPort),
								"--initial-cluster-token=dynamo-etcd-cluster",
								"--initial-cluster-state=new",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "client",
									ContainerPort: int32(etcdClientPort),
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "peer",
									ContainerPort: int32(etcdPeerPort),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(int(etcdClientPort)),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(int(etcdClientPort)),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "etcd-data",
									MountPath: "/etcd-data",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "etcd-data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
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

	if err := b.createOrUpdateDeployment(ctx, kubeClient, deployment); err != nil {
		return fmt.Errorf("failed to create etcd deployment: %w", err)
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
					Name:       "client",
					Port:       int32(etcdClientPort),
					TargetPort: intstr.FromInt(int(etcdClientPort)),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "peer",
					Port:       int32(etcdPeerPort),
					TargetPort: intstr.FromInt(int(etcdPeerPort)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}

	if err := b.createOrUpdateService(ctx, kubeClient, service); err != nil {
		return fmt.Errorf("failed to create etcd service: %w", err)
	}

	logger.Info("Created etcd resources",
		zap.String("deployment", deploymentName),
		zap.String("service", serviceName))

	return nil
}

// createFrontend creates the Frontend Deployment and Service with etcd discovery.
func (b *dynamoEtcdBackend) createFrontend(ctx context.Context, logger *zap.Logger, kubeClient client.Client, serverName string, namespace string) error {
	deploymentName := b.frontendDeploymentName(serverName)
	serviceName := b.frontendServiceName(serverName)
	etcdEndpoint := b.etcdEndpoint(serverName, namespace)

	labels := map[string]string{
		selfProvisionManagedByLabel: etcdBackendManagedByValue,
		selfProvisionServerLabel:    serverName,
		selfProvisionComponentLabel: "frontend",
		dynamoComponentTypeLabel:    "frontend",
		dynamoNamespaceLabel:        "dynamo",
		dynamoComponentLabel:        "Frontend",
		dynamoDiscoveryEnabledLabel: "true",
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
							Name:  "main",
							Image: defaultEtcdBackendImage,
							Command: []string{
								"python3",
								"-m",
								"dynamo.frontend",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: int32(frontendPort),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{
								{Name: "DYN_NAMESPACE", Value: "dynamo"},
								{Name: "DYN_COMPONENT", Value: "Frontend"},
								{Name: "DYN_DISCOVERY_BACKEND", Value: "etcd"},
								{Name: "DYN_STORE_KV", Value: "etcd"},
								{Name: "ETCD_ENDPOINTS", Value: etcdEndpoint},
								{Name: "DYN_EVENT_PLANE", Value: "zmq"},
								{Name: "DYN_HTTP_PORT", Value: fmt.Sprintf("%d", frontendPort)},
								{Name: "DYNAMO_PORT", Value: fmt.Sprintf("%d", frontendPort)},
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
								{Name: "LD_LIBRARY_PATH", Value: "/usr/local/nvidia/lib64:/usr/local/cuda/lib64"},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/live",
										Port: intstr.FromInt(int(frontendPort)),
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
										Port: intstr.FromInt(int(frontendPort)),
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
					Port:       int32(frontendPort),
					TargetPort: intstr.FromInt(int(frontendPort)),
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

// createDecodeWorker creates the Decode Worker Deployment and Headless Service
// with etcd-based service discovery.
func (b *dynamoEtcdBackend) createDecodeWorker(ctx context.Context, logger *zap.Logger, kubeClient client.Client, serverName string, namespace string, inferenceServer *v2pb.InferenceServer) error {
	deploymentName := b.decodeWorkerDeploymentName(serverName)
	serviceName := b.decodeWorkerServiceName(serverName)
	etcdEndpoint := b.etcdEndpoint(serverName, namespace)

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

	modelName := defaultEtcdBackendModel

	baseModelHash := b.computeModelHash(modelName)
	sanitizedModelName := b.sanitizeLabelValue(modelName)

	labels := map[string]string{
		selfProvisionManagedByLabel:         etcdBackendManagedByValue,
		selfProvisionServerLabel:            serverName,
		selfProvisionComponentLabel:         "decode-worker",
		dynamoComponentTypeLabel:            "worker",
		dynamoSubComponentTypeLabel:         "decode",
		dynamoNamespaceLabel:                "dynamo",
		dynamoComponentLabel:                "VllmDecodeWorker",
		dynamoBaseModelLabel:                sanitizedModelName,
		dynamoDiscoveryEnabledLabel:         "true",
		"nvidia.com/dynamo-base-model-hash": baseModelHash,
	}

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
							Image: defaultEtcdBackendImage,
							Command: []string{
								"python3",
								"-m",
								"dynamo.vllm",
								fmt.Sprintf("--model=%s", modelName),
								"--is-decode-worker",
								"--connector=nixl",
								"--enable-lora",
								"--max-loras=4",
								"--max-lora-rank=64",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "system",
									ContainerPort: int32(workerSystemPort),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{
								{Name: "DYN_NAMESPACE", Value: "dynamo"},
								{Name: "DYN_COMPONENT", Value: "VllmDecodeWorker"},
								{Name: "DYN_DISCOVERY_BACKEND", Value: "etcd"},
								{Name: "DYN_STORE_KV", Value: "etcd"},
								{Name: "ETCD_ENDPOINTS", Value: etcdEndpoint},
								{Name: "DYN_EVENT_PLANE", Value: "zmq"},
								{Name: "DYN_SYSTEM_PORT", Value: fmt.Sprintf("%d", workerSystemPort)},
								{Name: "DYN_LORA_ENABLED", Value: "true"},
								{Name: "DYN_LORA_PATH", Value: "/tmp/dynamo_loras"},
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
								{Name: "LD_LIBRARY_PATH", Value: "/usr/local/nvidia/lib64:/usr/local/cuda/lib64"},
								{Name: "NVIDIA_VISIBLE_DEVICES", Value: "all"},
								{Name: "NVIDIA_DRIVER_CAPABILITIES", Value: "compute,utility"},
								{Name: "UCX_TLS", Value: "tcp,cuda_copy,cuda_ipc"},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/live",
										Port: intstr.FromInt(int(workerSystemPort)),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								FailureThreshold:    6,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(int(workerSystemPort)),
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
										Port: intstr.FromInt(int(workerSystemPort)),
									},
								},
								PeriodSeconds:    10,
								TimeoutSeconds:   5,
								FailureThreshold: 720,
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
		return fmt.Errorf("failed to create decode worker deployment: %w", err)
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: corev1.ClusterIPNone,
			Ports: []corev1.ServicePort{
				{
					Name:       "system",
					Port:       int32(workerSystemPort),
					TargetPort: intstr.FromInt(int(workerSystemPort)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}

	if err := b.createOrUpdateService(ctx, kubeClient, service); err != nil {
		return fmt.Errorf("failed to create decode worker service: %w", err)
	}

	logger.Info("Created decode worker resources",
		zap.String("deployment", deploymentName),
		zap.String("service", serviceName),
		zap.Int32("replicas", replicas))

	return nil
}

// createOrUpdateDeployment creates or updates a Deployment.
func (b *dynamoEtcdBackend) createOrUpdateDeployment(ctx context.Context, kubeClient client.Client, deployment *appsv1.Deployment) error {
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

	existing.Spec = deployment.Spec
	existing.Labels = deployment.Labels
	if updateErr := kubeClient.Update(ctx, existing); updateErr != nil {
		return fmt.Errorf("failed to update deployment %s: %w", deployment.Name, updateErr)
	}
	return nil
}

// createOrUpdateService creates or updates a Service.
func (b *dynamoEtcdBackend) createOrUpdateService(ctx context.Context, kubeClient client.Client, service *corev1.Service) error {
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

	service.Spec.ClusterIP = existing.Spec.ClusterIP
	service.ResourceVersion = existing.ResourceVersion
	if updateErr := kubeClient.Update(ctx, service); updateErr != nil {
		return fmt.Errorf("failed to update service %s: %w", service.Name, updateErr)
	}
	return nil
}

// computeModelHash computes a short hash of the model name for service naming.
func (b *dynamoEtcdBackend) computeModelHash(modelName string) string {
	hash := sha256.Sum256([]byte(modelName))
	return hex.EncodeToString(hash[:4])
}

// sanitizeLabelValue sanitizes a string to be a valid Kubernetes label value.
func (b *dynamoEtcdBackend) sanitizeLabelValue(value string) string {
	sanitized := strings.ReplaceAll(value, "/", "-")

	invalidChars := regexp.MustCompile(`[^a-zA-Z0-9\-_.]`)
	sanitized = invalidChars.ReplaceAllString(sanitized, "-")

	sanitized = strings.Trim(sanitized, "-_.")

	if len(sanitized) > 63 {
		sanitized = sanitized[:63]
		sanitized = strings.TrimRight(sanitized, "-_.")
	}

	return sanitized
}

func (b *dynamoEtcdBackend) frontendDeploymentName(serverName string) string {
	return fmt.Sprintf("dynamo-etcd-%s-frontend", serverName)
}

func (b *dynamoEtcdBackend) frontendServiceName(serverName string) string {
	return fmt.Sprintf("dynamo-etcd-%s-frontend", serverName)
}

func (b *dynamoEtcdBackend) decodeWorkerDeploymentName(serverName string) string {
	return fmt.Sprintf("dynamo-etcd-%s-decode", serverName)
}

func (b *dynamoEtcdBackend) decodeWorkerServiceName(serverName string) string {
	return fmt.Sprintf("dynamo-etcd-%s-decode", serverName)
}

func (b *dynamoEtcdBackend) etcdDeploymentName(serverName string) string {
	return fmt.Sprintf("dynamo-etcd-%s-etcd", serverName)
}

func (b *dynamoEtcdBackend) etcdServiceName(serverName string) string {
	return fmt.Sprintf("dynamo-etcd-%s-etcd", serverName)
}

// etcdEndpoint returns the etcd endpoint for Dynamo components to connect to.
func (b *dynamoEtcdBackend) etcdEndpoint(serverName string, namespace string) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		b.etcdServiceName(serverName), namespace, etcdClientPort)
}

func (b *dynamoEtcdBackend) generateEndpoint(serverName string, namespace string) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		b.frontendServiceName(serverName), namespace, frontendPort)
}
