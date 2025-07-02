package inferenceserver

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Triton Infrastructure Management

func (g *gateway) createTritonInfrastructure(ctx context.Context, logger logr.Logger, request InfrastructureRequest) (*InfrastructureResponse, error) {
	logger.Info("Creating Triton infrastructure", "server", request.InferenceServer.Name)

	// Create VirtualService first for fixed endpoint routing
	if err := g.createInferenceServerVirtualService(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to create VirtualService: %w", err)
	}

	// Create ConfigMap for model configuration
	if err := g.createTritonConfigMap(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to create ConfigMap: %w", err)
	}

	// Create Deployment
	if err := g.createTritonDeployment(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to create Deployment: %w", err)
	}

	// Create Service
	if err := g.createTritonService(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to create Service: %w", err)
	}

	return &InfrastructureResponse{
		State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
		Message: "Triton infrastructure creation initiated",
		Endpoints: []string{
			fmt.Sprintf("/%s-endpoint/%s/production", request.InferenceServer.Name, request.InferenceServer.Name),
		},
		Details: map[string]interface{}{
			"backend":   "triton",
			"namespace": request.Namespace,
		},
	}, nil
}

func (g *gateway) getTritonInfrastructureStatus(ctx context.Context, logger logr.Logger, request InfrastructureStatusRequest) (*InfrastructureStatus, error) {
	logger.Info("Getting Triton infrastructure status", "server", request.InferenceServer)

	// Check deployment status
	deployment := &appsv1.Deployment{}
	deploymentKey := client.ObjectKey{Name: request.InferenceServer, Namespace: request.Namespace}

	if err := g.kubeClient.Get(ctx, deploymentKey, deployment); err != nil {
		return &InfrastructureStatus{
			State:   v2pb.INFERENCE_SERVER_STATE_FAILED,
			Message: fmt.Sprintf("Deployment not found: %v", err),
			Ready:   false,
		}, nil
	}

	// Check if deployment is ready
	ready := deployment.Status.ReadyReplicas == deployment.Status.Replicas && deployment.Status.Replicas > 0

	state := v2pb.INFERENCE_SERVER_STATE_CREATING
	if ready {
		state = v2pb.INFERENCE_SERVER_STATE_SERVING
	}

	return &InfrastructureStatus{
		State:   state,
		Message: fmt.Sprintf("Deployment status: %d/%d replicas ready", deployment.Status.ReadyReplicas, deployment.Status.Replicas),
		Ready:   ready,
		Endpoints: []string{
			fmt.Sprintf("http://%s-service.%s.svc.cluster.local:80", request.InferenceServer, request.Namespace),
		},
	}, nil
}

func (g *gateway) deleteTritonInfrastructure(ctx context.Context, logger logr.Logger, request InfrastructureDeleteRequest) error {
	logger.Info("Deleting Triton infrastructure", "server", request.InferenceServer)

	// Delete VirtualService
	gvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}
	virtualServiceName := fmt.Sprintf("%s-virtualservice", request.InferenceServer)
	if err := g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Delete(ctx, virtualServiceName, metav1.DeleteOptions{}); err != nil {
		logger.Error(err, "Failed to delete VirtualService")
	}

	// Delete Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      request.InferenceServer,
			Namespace: request.Namespace,
		},
	}
	if err := g.kubeClient.Delete(ctx, deployment); err != nil {
		logger.Error(err, "Failed to delete deployment")
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

	// Delete ConfigMap
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", request.InferenceServer),
			Namespace: request.Namespace,
		},
	}
	if err := g.kubeClient.Delete(ctx, configMap); err != nil {
		logger.Error(err, "Failed to delete configmap")
	}

	return nil
}

func (g *gateway) createTritonConfigMap(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	// Skip ConfigMap creation as we'll use config.pbtxt from the model package
	logger.Info("Using config.pbtxt from model package, skipping ConfigMap creation")
	return nil
}

func (g *gateway) createTritonDeployment(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	deploymentName := request.InferenceServer.Name

	// Check if Deployment already exists
	existing := &appsv1.Deployment{}
	err := g.kubeClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: request.Namespace}, existing)
	if err == nil {
		// Deployment already exists, log and return success
		logger.Info("Deployment already exists, skipping creation", "name", deploymentName)
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
								"--model-store=s3://deploy-models/bert-cola-23",
								"--allow-http=true",
								"--allow-grpc=true",
								"--allow-metrics=true",
								"--strict-model-config=false",
								"--exit-on-error=false",
								"--model-control-mode=poll",
								"--repository-poll-secs=60",
								"--log-verbose=1",
							},
							Env: []corev1.EnvVar{
								{
									Name:  "AWS_ACCESS_KEY_ID",
									Value: "minioadmin",
								},
								{
									Name:  "AWS_SECRET_ACCESS_KEY",
									Value: "minioadmin",
								},
								{
									Name:  "AWS_DEFAULT_REGION",
									Value: "us-south",
								},
								{
									Name:  "AWS_ENDPOINT_URL",
									Value: "http://minio:9091",
								},
								{
									Name:  "AWS_S3_FORCE_PATH_STYLE",
									Value: "true",
								},
								{
									Name:  "AWS_S3_USE_PATH_STYLE_ENDPOINT",
									Value: "true",
								},
								{
									Name:  "S3_VERIFY_SSL",
									Value: "false",
								},
								{
									Name:  "TRITON_AWS_MOUNT_DIRECTORY",
									Value: "/etc/storage",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "storage-secret",
									MountPath: "/etc/storage",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "storage-secret",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "storage-config",
									Items: []corev1.KeyToPath{
										{
											Key:  "localMinIO",
											Path: "localMinIO.json",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return g.kubeClient.Create(ctx, deployment)
}

func (g *gateway) createTritonService(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	serviceName := fmt.Sprintf("%s-service", request.InferenceServer.Name)

	// Check if Service already exists
	existing := &corev1.Service{}
	err := g.kubeClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: request.Namespace}, existing)
	if err == nil {
		// Service already exists, log and return success
		logger.Info("Service already exists, skipping creation", "name", serviceName)
		return nil
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: request.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": request.InferenceServer.Name,
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

	return g.kubeClient.Create(ctx, service)
}


func (g *gateway) createInferenceServerVirtualService(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	gvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	virtualServiceName := fmt.Sprintf("%s-virtualservice", request.InferenceServer.Name)
	
	// Check if VirtualService already exists
	_, err := g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Get(ctx, virtualServiceName, metav1.GetOptions{})
	if err == nil {
		logger.Info("VirtualService already exists, skipping creation", "name", virtualServiceName)
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
									"prefix": fmt.Sprintf("/%s-endpoint/%s/production", 
										request.InferenceServer.Name, 
										request.InferenceServer.Name),
								},
							},
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
					{
						"match": []map[string]interface{}{
							{
								"uri": map[string]string{
									"prefix": fmt.Sprintf("/%s-endpoint/%s/canary", 
										request.InferenceServer.Name, 
										request.InferenceServer.Name),
								},
							},
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

	_, err = g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Create(ctx, vs, metav1.CreateOptions{})
	if err != nil {
		logger.Error(err, "Failed to create VirtualService")
		return err
	}

	logger.Info("VirtualService created successfully", "name", virtualServiceName)
	return nil
}

// Triton Model Management

func (g *gateway) loadTritonModel(ctx context.Context, logger logr.Logger, request ModelLoadRequest) error {
	logger.Info("Loading Triton model", "model", request.ModelName)

	// For now, return success as the model is loaded via config
	// In a real implementation, this would call the Triton model management API
	return nil
}

func (g *gateway) checkTritonModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (bool, error) {
	logger.Info("Checking Triton model status", "model", request.ModelName)

	// For now, assume model is ready if infrastructure is ready
	// In a real implementation, this would call the Triton model status API
	return true, nil
}

func (g *gateway) getTritonModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (*ModelStatus, error) {
	logger.Info("Getting Triton model status", "model", request.ModelName)

	return &ModelStatus{
		State:   "LOADED",
		Message: "Model is loaded and ready",
		Ready:   true,
	}, nil
}

func (g *gateway) isTritonHealthy(ctx context.Context, logger logr.Logger, serverName string) (bool, error) {
	logger.Info("Checking Triton health", "server", serverName)

	// For now, assume healthy if infrastructure exists
	// In a real implementation, this would call the Triton health API
	return true, nil
}

// Helper functions


func getTritonImageTag(tag string) string {
	if tag == "" {
		return "23.04-py3" // Default Triton image tag
	}
	return tag
}

func buildResourceRequirements(resources ResourceSpec) corev1.ResourceRequirements {
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
