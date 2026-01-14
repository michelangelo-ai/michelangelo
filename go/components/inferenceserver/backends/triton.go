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

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpointregistry"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/secrets"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ Backend = &tritonBackend{}

const (
	defaultTritonImageTag = "23.04-py3"
)

// Triton Server Management
type tritonBackend struct {
	kubeClient             client.Client
	clientFactory          clientfactory.ClientFactory
	modelConfigMapProvider configmap.ModelConfigMapProvider
	endpointRegistry       endpointregistry.EndpointRegistry
	logger                 *zap.Logger
}

func NewTritonBackend(kubeClient client.Client, modelConfigMapProvider configmap.ModelConfigMapProvider, endpointRegistry endpointregistry.EndpointRegistry, logger *zap.Logger) *tritonBackend {
	sp := secrets.NewProvider(kubeClient)
	return &tritonBackend{
		kubeClient:             kubeClient,
		clientFactory:          clientfactory.NewClientFactory(kubeClient, sp, kubeClient.Scheme(), logger),
		modelConfigMapProvider: modelConfigMapProvider,
		endpointRegistry:       endpointRegistry,
		logger:                 logger,
	}
}

func (b *tritonBackend) CreateServer(ctx context.Context, logger *zap.Logger, inferenceServer *v2pb.InferenceServer) (*ServerStatus, error) {
	logger.Info("Creating Triton server",
		zap.String("server", inferenceServer.Name),
		zap.String("namespace", inferenceServer.Namespace),
	)
	endpoints := []string{}
	for _, clusterTarget := range inferenceServer.Spec.ClusterTargets {
		connectionSpec := clusterTarget.GetKubernetes()
		clusterClient, err := b.clientFactory.GetClient(ctx, connectionSpec)
		if err != nil {
			logger.Error("failed to get client for cluster",
				zap.Error(err),
				zap.String("operation", "create_server"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("inferenceServer", inferenceServer.Name),
				zap.String("cluster", clusterTarget.ClusterId))
			return nil, fmt.Errorf("failed to get client for cluster %s: %w", clusterTarget.ClusterId, err)
		}

		// Create Deployment in the target cluster
		if err = b.createTritonDeployment(ctx, logger, inferenceServer, clusterClient); err != nil {
			logger.Error("failed to create Deployment",
				zap.Error(err),
				zap.String("operation", "create_server"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("inferenceServer", inferenceServer.Name))
			return nil, fmt.Errorf("failed to create Deployment for %s/%s: %w",
				inferenceServer.Namespace, inferenceServer.Name, err)
		}

		// Create Service in the target cluster (returns the service with allocated NodePort)
		tritonService, err := b.createTritonService(ctx, logger, inferenceServer, clusterClient)
		if err != nil {
			logger.Error("failed to create Service",
				zap.Error(err),
				zap.String("operation", "create_server"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("inferenceServer", inferenceServer.Name))
			return nil, fmt.Errorf("failed to create Service for %s/%s: %w",
				inferenceServer.Namespace, inferenceServer.Name, err)
		}

		// Create empty ConfigMap for model configuration in the target cluster
		if err = b.modelConfigMapProvider.CreateModelConfigMap(ctx, inferenceServer.Name, inferenceServer.Namespace, connectionSpec, nil, nil, nil); err != nil {
			logger.Error("failed to create ConfigMap",
				zap.Error(err),
				zap.String("operation", "create_server"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("inferenceServer", inferenceServer.Name))
			return nil, fmt.Errorf("failed to create ConfigMap for %s/%s: %w",
				inferenceServer.Namespace, inferenceServer.Name, err)
		}

		// Find the HTTP NodePort from the created service
		var httpNodePort int32
		for _, port := range tritonService.Spec.Ports {
			if port.Name == "http" {
				httpNodePort = port.NodePort
				break
			}
		}
		if httpNodePort == 0 {
			return nil, fmt.Errorf("Triton service %s has no HTTP NodePort allocated", tritonService.Name)
		}

		// Get a node address from the target cluster for routing
		nodeHost, err := b.getNodeAddress(ctx, clusterClient)
		if err != nil {
			logger.Error("failed to get node address from target cluster",
				zap.Error(err),
				zap.String("cluster", clusterTarget.ClusterId))
			return nil, fmt.Errorf("failed to get node address for cluster %s: %w", clusterTarget.ClusterId, err)
		}

		// Register the cluster endpoint in the control plane (ServiceEntry + ExternalName Service)
		// Use the actual node address and NodePort for traffic routing
		clusterEndpoint := endpointregistry.ClusterEndpoint{
			ClusterID:           clusterTarget.ClusterId,
			InferenceServerName: inferenceServer.Name,
			Namespace:           inferenceServer.Namespace,
			Host:                nodeHost,
			Port:                fmt.Sprintf("%d", httpNodePort),
			ServiceHost:         nodeHost, // ExternalName will point to the actual node
			ServicePort:         uint32(httpNodePort),
			TokenSecretRef:      connectionSpec.TokenTag,
			CASecretRef:         connectionSpec.CaDataTag,
		}
		if err := b.endpointRegistry.RegisterEndpoint(ctx, logger, clusterEndpoint); err != nil {
			logger.Error("failed to register cluster endpoint",
				zap.Error(err),
				zap.String("operation", "create_server"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("inferenceServer", inferenceServer.Name),
				zap.String("cluster", clusterTarget.ClusterId))
			return nil, fmt.Errorf("failed to register endpoint for cluster %s: %w",
				clusterTarget.ClusterId, err)
		}

		// Build endpoint URL based on cluster context
		endpoint := b.buildServiceEndpoint(inferenceServer.Name, inferenceServer.Namespace, connectionSpec)
		endpoints = append(endpoints, endpoint)
	}

	return &ServerStatus{
		State:     v2pb.INFERENCE_SERVER_STATE_CREATING,
		Message:   "Triton Server creation initiated with empty ConfigMap",
		Endpoints: endpoints,
	}, nil
}

func (b *tritonBackend) GetServerStatus(ctx context.Context, logger *zap.Logger, inferenceServer *v2pb.InferenceServer) (*ServerStatus, error) {
	logger.Info("Getting Triton server status", zap.String("server", inferenceServer.Name))

	// Check deployment status for each cluster target
	endpoints := []string{}
	allReady := true
	clustersChecked := 0
	totalClusters := len(inferenceServer.Spec.ClusterTargets)

	for _, clusterTarget := range inferenceServer.Spec.ClusterTargets {
		connectionSpec := clusterTarget.GetKubernetes()
		clusterClient, err := b.clientFactory.GetClient(ctx, connectionSpec)
		if err != nil {
			logger.Error("failed to get client for cluster",
				zap.Error(err),
				zap.String("operation", "get_server_status"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("inferenceServer", inferenceServer.Name),
				zap.String("cluster", clusterTarget.ClusterId))
			// Mark as not ready but continue checking other clusters
			allReady = false
			continue
		}

		deployment := &appsv1.Deployment{}
		deploymentKey := client.ObjectKey{Name: fmt.Sprintf("triton-%s", inferenceServer.Name), Namespace: inferenceServer.Namespace}

		if err := clusterClient.Get(ctx, deploymentKey, deployment); err != nil {
			logger.Info("Deployment not found in cluster",
				zap.String("cluster", clusterTarget.ClusterId),
				zap.Error(err))
			allReady = false
			continue
		}

		// Check if ConfigMap exists
		configMapName := fmt.Sprintf("%s-model-config", inferenceServer.Name)
		configMap := &corev1.ConfigMap{}
		configMapKey := client.ObjectKey{Name: configMapName, Namespace: inferenceServer.Namespace}

		if err := clusterClient.Get(ctx, configMapKey, configMap); err != nil {
			logger.Info("ConfigMap not found in cluster",
				zap.String("configMap", configMapName),
				zap.String("cluster", clusterTarget.ClusterId))
			allReady = false
			continue
		}

		// Check if deployment is ready by comparing against desired replicas (Spec.Replicas)
		desiredReplicas := int32(1)
		if deployment.Spec.Replicas != nil {
			desiredReplicas = *deployment.Spec.Replicas
		}

		clusterReady := deployment.Status.ReadyReplicas == desiredReplicas && desiredReplicas > 0
		if !clusterReady {
			logger.Info("Deployment not ready in cluster",
				zap.String("cluster", clusterTarget.ClusterId),
				zap.Int32("readyReplicas", deployment.Status.ReadyReplicas),
				zap.Int32("desiredReplicas", desiredReplicas),
				zap.Int32("statusReplicas", deployment.Status.Replicas))
			allReady = false
		} else {
			logger.Info("Deployment ready in cluster",
				zap.String("cluster", clusterTarget.ClusterId),
				zap.Int32("readyReplicas", deployment.Status.ReadyReplicas),
				zap.Int32("desiredReplicas", desiredReplicas))
		}

		clustersChecked++

		// Build endpoint URL based on cluster context
		endpoint := b.buildServiceEndpoint(inferenceServer.Name, inferenceServer.Namespace, connectionSpec)
		endpoints = append(endpoints, endpoint)
	}

	// Determine overall state: only SERVING if ALL clusters are checked and ALL are ready
	state := v2pb.INFERENCE_SERVER_STATE_CREATING
	message := fmt.Sprintf("Inference server %s creation is pending", inferenceServer.Name)

	if clustersChecked == totalClusters && totalClusters > 0 && allReady {
		state = v2pb.INFERENCE_SERVER_STATE_SERVING
		message = fmt.Sprintf("Inference server %s is ready on all %d cluster(s)", inferenceServer.Name, clustersChecked)
	} else if clustersChecked > 0 {
		message = fmt.Sprintf("Inference server %s is creating (%d/%d cluster(s) checked, allReady=%v)",
			inferenceServer.Name, clustersChecked, totalClusters, allReady)
	}

	logger.Info("Server status determined",
		zap.String("state", state.String()),
		zap.Int("clustersChecked", clustersChecked),
		zap.Int("totalClusters", totalClusters),
		zap.Bool("allReady", allReady))

	return &ServerStatus{
		State:     state,
		Message:   message,
		Ready:     allReady && clustersChecked == totalClusters && totalClusters > 0,
		Endpoints: endpoints,
	}, nil
}

func (b *tritonBackend) DeleteServer(ctx context.Context, logger *zap.Logger, inferenceServer *v2pb.InferenceServer) error {
	logger.Info("Deleting Triton server", zap.String("server", inferenceServer.Name))

	var errs []error

	for _, clusterTarget := range inferenceServer.Spec.ClusterTargets {
		connectionSpec := clusterTarget.GetKubernetes()
		clusterClient, err := b.clientFactory.GetClient(ctx, connectionSpec)
		if err != nil {
			logger.Error("failed to get client for cluster",
				zap.Error(err),
				zap.String("operation", "delete_server"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("inferenceServer", inferenceServer.Name),
				zap.String("cluster", clusterTarget.ClusterId))
			errs = append(errs, fmt.Errorf("failed to get client for cluster %s: %w", clusterTarget.ClusterId, err))
			continue
		}

		// Delete Deployment
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("triton-%s", inferenceServer.Name),
				Namespace: inferenceServer.Namespace,
			},
		}

		if err := clusterClient.Delete(ctx, deployment); client.IgnoreNotFound(err) != nil {
			logger.Error("failed to delete deployment",
				zap.Error(err),
				zap.String("operation", "delete_server"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("inferenceServer", inferenceServer.Name),
				zap.String("cluster", clusterTarget.ClusterId))
			errs = append(errs, err)
		}

		// Delete Service
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-inference-service", inferenceServer.Name),
				Namespace: inferenceServer.Namespace,
			},
		}
		if err := clusterClient.Delete(ctx, service); client.IgnoreNotFound(err) != nil {
			logger.Error("failed to delete service",
				zap.Error(err),
				zap.String("operation", "delete_server"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("inferenceServer", inferenceServer.Name),
				zap.String("cluster", clusterTarget.ClusterId))
			errs = append(errs, err)
		}

		// Delete ConfigMap using the modelConfigMapProvider
		if err := b.modelConfigMapProvider.DeleteModelConfigMap(ctx, inferenceServer.Name, inferenceServer.Namespace, connectionSpec); err != nil {
			logger.Error("failed to delete ConfigMap",
				zap.Error(err),
				zap.String("operation", "delete_server"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("inferenceServer", inferenceServer.Name),
				zap.String("cluster", clusterTarget.ClusterId))
			errs = append(errs, err)
		} else {
			logger.Info("ConfigMap deleted successfully",
				zap.String("name", fmt.Sprintf("%s-model-config", inferenceServer.Name)),
				zap.String("cluster", clusterTarget.ClusterId))
		}

		// Unregister the cluster endpoint from the control plane
		if err := b.endpointRegistry.UnregisterEndpoint(ctx, logger, inferenceServer.Name, inferenceServer.Namespace, clusterTarget.ClusterId); err != nil {
			logger.Error("failed to unregister cluster endpoint",
				zap.Error(err),
				zap.String("operation", "delete_server"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("inferenceServer", inferenceServer.Name),
				zap.String("cluster", clusterTarget.ClusterId))
			errs = append(errs, err)
		} else {
			logger.Info("Cluster endpoint unregistered successfully",
				zap.String("inferenceServer", inferenceServer.Name),
				zap.String("cluster", clusterTarget.ClusterId))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("encountered %d error(s) during deletion", len(errs))
	}

	return nil
}

func (b *tritonBackend) IsHealthy(ctx context.Context, logger *zap.Logger, inferenceServer *v2pb.InferenceServer) (bool, error) {
	logger.Info("Checking Triton health via Kubernetes pod status", zap.String("server", inferenceServer.Name))

	for _, clusterTarget := range inferenceServer.Spec.ClusterTargets {
		connectionSpec := clusterTarget.GetKubernetes()
		clusterClient, err := b.clientFactory.GetClient(ctx, connectionSpec)
		if err != nil {
			logger.Error("failed to get client for cluster",
				zap.Error(err),
				zap.String("operation", "health_check"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("inferenceServer", inferenceServer.Name),
				zap.String("cluster", clusterTarget.ClusterId))
			return false, fmt.Errorf("failed to get client for cluster %s: %w", clusterTarget.ClusterId, err)
		}

		deploymentName := fmt.Sprintf("triton-%s", inferenceServer.Name)
		deployment := &appsv1.Deployment{}

		err = clusterClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: inferenceServer.Namespace}, deployment)
		if err != nil {
			logger.Error("failed to get Triton deployment",
				zap.Error(err),
				zap.String("operation", "health_check"),
				zap.String("namespace", inferenceServer.Namespace),
				zap.String("deployment", deploymentName),
				zap.String("cluster", clusterTarget.ClusterId))
			return false, fmt.Errorf("failed to get deployment %s/%s in cluster %s: %w",
				inferenceServer.Namespace, deploymentName, clusterTarget.ClusterId, err)
		}

		// Check deployment conditions
		for _, condition := range deployment.Status.Conditions {
			if condition.Type == appsv1.DeploymentAvailable {
				if condition.Status == corev1.ConditionTrue {
					// Also check if pods are ready
					if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas == deployment.Status.Replicas {
						logger.Info("Triton pods are ready in cluster",
							zap.String("server", inferenceServer.Name),
							zap.Int32("readyReplicas", deployment.Status.ReadyReplicas),
							zap.String("cluster", clusterTarget.ClusterId))
					} else {
						logger.Error("Triton deployment available but pods not ready",
							zap.String("operation", "health_check"),
							zap.String("namespace", inferenceServer.Namespace),
							zap.String("server", inferenceServer.Name),
							zap.Int32("readyReplicas", deployment.Status.ReadyReplicas),
							zap.Int32("totalReplicas", deployment.Status.Replicas),
							zap.String("cluster", clusterTarget.ClusterId))
						return false, nil
					}
				} else {
					logger.Error("Triton deployment not available",
						zap.String("operation", "health_check"),
						zap.String("namespace", inferenceServer.Namespace),
						zap.String("server", inferenceServer.Name),
						zap.String("reason", condition.Reason),
						zap.String("message", condition.Message),
						zap.String("cluster", clusterTarget.ClusterId))
					return false, nil
				}
			}
		}
	}
	return true, nil
}

// Triton Model Management

func (b *tritonBackend) CheckModelStatus(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, connectionSpec *v2pb.ConnectionSpec) (bool, error) {
	logger.Info("Checking Triton model status", zap.String("model", modelName), zap.String("server", inferenceServerName))

	modelReadyPath := fmt.Sprintf("/v2/models/%s/ready", modelName)

	// Use Kubernetes API proxy to access the service
	serviceURL := b.buildServiceEndpoint(inferenceServerName, namespace, connectionSpec) + modelReadyPath

	logger.Info("Checking Triton model ready endpoint",
		zap.String("url", serviceURL),
		zap.String("model", modelName),
		zap.String("namespace", namespace),
		zap.String("server", inferenceServerName))

	httpClient := &http.Client{
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

	resp, err := httpClient.Do(req)
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

func (b *tritonBackend) createTritonDeployment(ctx context.Context, logger *zap.Logger, inferenceServer *v2pb.InferenceServer, clusterClient client.Client) error {
	deploymentName := fmt.Sprintf("triton-%s", inferenceServer.Name)

	// Check if Deployment already exists in the target cluster
	existing := &appsv1.Deployment{}
	err := clusterClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: inferenceServer.Namespace}, existing)
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

	if err := clusterClient.Create(ctx, deployment); err != nil {
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

func (b *tritonBackend) createTritonService(ctx context.Context, logger *zap.Logger, inferenceServer *v2pb.InferenceServer, clusterClient client.Client) (*corev1.Service, error) {
	serviceName := fmt.Sprintf("%s-inference-service", inferenceServer.Name)

	// Check if Service already exists in the target cluster
	existing := &corev1.Service{}
	err := clusterClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: inferenceServer.Namespace}, existing)
	if err == nil {
		// Service already exists, return it
		logger.Info("Service already exists, skipping creation", zap.String("name", serviceName))
		return existing, nil
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
			Type: corev1.ServiceTypeNodePort,
		},
	}

	if err := clusterClient.Create(ctx, service); err != nil {
		logger.Error("failed to create Triton Service",
			zap.Error(err),
			zap.String("operation", "create_triton_service"),
			zap.String("namespace", inferenceServer.Namespace),
			zap.String("service", serviceName))
		return nil, fmt.Errorf("failed to create Triton Service %s/%s: %w",
			inferenceServer.Namespace, serviceName, err)
	}
	// Create mutates the object in place with the server response (including allocated NodePort)
	return service, nil
}

// getNodeAddress gets an accessible address for a node in the target cluster.
// For k3d clusters, this returns the node's hostname which is resolvable from
// other k3d clusters on the same Docker network.
func (b *tritonBackend) getNodeAddress(ctx context.Context, clusterClient client.Client) (string, error) {
	nodeList := &corev1.NodeList{}
	if err := clusterClient.List(ctx, nodeList); err != nil {
		return "", fmt.Errorf("failed to list nodes: %w", err)
	}

	if len(nodeList.Items) == 0 {
		return "", fmt.Errorf("no nodes found in cluster")
	}

	// Get the first node and find a usable address
	node := nodeList.Items[0]

	// Prefer InternalIP, then Hostname, then ExternalIP
	var internalIP, hostname, externalIP string
	for _, addr := range node.Status.Addresses {
		switch addr.Type {
		case corev1.NodeInternalIP:
			internalIP = addr.Address
		case corev1.NodeHostName:
			hostname = addr.Address
		case corev1.NodeExternalIP:
			externalIP = addr.Address
		}
	}

	// For k3d/Docker environments, hostname is often the most reliable
	// as it's resolvable across containers on the same network
	if hostname != "" {
		return hostname, nil
	}
	if internalIP != "" {
		return internalIP, nil
	}
	if externalIP != "" {
		return externalIP, nil
	}

	return "", fmt.Errorf("no usable address found for node %s", node.Name)
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

// buildServiceEndpoint constructs the Kubernetes API proxy URL for the service.
// Format: https://{host}:{port}/api/v1/namespaces/{namespace}/services/{service}:http/proxy
func (b *tritonBackend) buildServiceEndpoint(inferenceServerName string, namespace string, connectionSpec *v2pb.ConnectionSpec) string {
	serviceName := fmt.Sprintf("%s-inference-service", inferenceServerName)
	return fmt.Sprintf("https://%s:%s/api/v1/namespaces/%s/services/%s:http/proxy",
		connectionSpec.Host, connectionSpec.Port, namespace, serviceName)
}
