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
			fmt.Sprintf("/%s-endpoint/%s", request.InferenceServer.Name, request.InferenceServer.Name),
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
			Name:      fmt.Sprintf("%s-models", request.InferenceServer),
			Namespace: request.Namespace,
		},
	}
	if err := g.kubeClient.Delete(ctx, configMap); err != nil {
		logger.Error(err, "Failed to delete configmap")
	}

	return nil
}

func (g *gateway) createTritonConfigMap(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	configMapName := fmt.Sprintf("%s-models", request.InferenceServer.Name)
	
	// Check if ConfigMap already exists
	existing := &corev1.ConfigMap{}
	err := g.kubeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: request.Namespace}, existing)
	if err == nil {
		logger.Info("ConfigMap already exists, skipping creation", "name", configMapName)
		return nil
	}
	
	// Create model list configuration for the sidecar
	// Extract model path from resource configuration, fallback to default
	modelPath := "s3://deploy-models/bert-cola-23/"
	if modelConfig, ok := request.Resources.ModelConfig["model"]; ok {
		modelPath = modelConfig
	}
	
	modelList := fmt.Sprintf(`[
  {
    "name": "%s",
    "s3_path": "%s"
  }
]`, request.InferenceServer.Name, modelPath)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: request.Namespace,
		},
		Data: map[string]string{
			"model-list.json": modelList,
		},
	}

	return g.kubeClient.Create(ctx, configMap)
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
								"--model-store=/mnt/models",
								"--grpc-port=8001",
								"--http-port=8000",
								"--allow-grpc=true",
								"--allow-http=true",
								"--allow-metrics=true",
								"--metrics-port=8002",
								"--model-control-mode=poll",
								"--repository-poll-secs=60",
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
						{
							Name:  "model-sync",
							Image: "amazon/aws-cli:2.15.50",
							Command: []string{"/bin/sh", "-c"},
							Args: []string{
								`yum install -y jq && \
CONFIG_FILE=/secret/localMinIO.json
ACCESS_KEY=$(jq -r '.access_key_id' $CONFIG_FILE)
SECRET_KEY=$(jq -r '.secret_access_key' $CONFIG_FILE)
ENDPOINT=$(jq -r '.endpoint_url' $CONFIG_FILE)
REGION=$(jq -r '.region' $CONFIG_FILE)
aws configure set aws_access_key_id $ACCESS_KEY
aws configure set aws_secret_access_key $SECRET_KEY
aws configure set default.region $REGION
aws configure set default.s3.endpoint_url $ENDPOINT

while true; do
  cp /config/model-list.json /tmp/model-list.json
  jq -c '.[]' /tmp/model-list.json | while read model; do
    name=$(echo "$model" | jq -r '.name')
    s3_path=$(echo "$model" | jq -r '.s3_path')
    echo "Syncing model $name from $s3_path to /mnt/models/"
    aws s3 sync "$s3_path" /mnt/models/ --delete --exact-timestamps --endpoint-url "$ENDPOINT"
  done
  
  sleep 60
done`,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    parseQuantity("100m"),
									corev1.ResourceMemory: parseQuantity("100Mi"),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "workdir",
									MountPath: "/mnt/models",
								},
								{
									Name:      "model-config",
									MountPath: "/config",
								},
								{
									Name:      "storage-secret",
									MountPath: "/secret",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "workdir",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "model-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-models", request.InferenceServer.Name),
									},
								},
							},
						},
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

// updateTritonModelConfig updates the model configuration for rolling out new models
func (g *gateway) updateTritonModelConfig(ctx context.Context, logger logr.Logger, inferenceServerName, namespace string, modelConfigs []ModelConfig) error {
	configMapName := fmt.Sprintf("%s-models", inferenceServerName)
	
	// Build model list JSON
	modelList := "["
	for i, config := range modelConfigs {
		if i > 0 {
			modelList += ","
		}
		modelList += fmt.Sprintf(`
  {
    "name": "%s",
    "s3_path": "%s"
  }`, config.Name, config.S3Path)
	}
	modelList += "\n]"
	
	// Get existing ConfigMap
	configMap := &corev1.ConfigMap{}
	err := g.kubeClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, configMap)
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap: %w", err)
	}
	
	// Update the model list
	configMap.Data["model-list.json"] = modelList
	
	// Apply the update
	err = g.kubeClient.Update(ctx, configMap)
	if err != nil {
		return fmt.Errorf("failed to update ConfigMap: %w", err)
	}
	
	logger.Info("Updated model configuration", "configMap", configMapName, "models", len(modelConfigs))
	return nil
}

// ModelConfig represents a model configuration for syncing
type ModelConfig struct {
	Name   string
	S3Path string
}

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
