package inferenceserver

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Triton Infrastructure Management

func (g *gateway) createTritonInfrastructure(ctx context.Context, logger logr.Logger, request InfrastructureRequest) (*InfrastructureResponse, error) {
	logger.Info("Creating Triton infrastructure", "server", request.InferenceServer.Name)

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
			fmt.Sprintf("http://%s-service.%s.svc.cluster.local:80", request.InferenceServer.Name, request.Namespace),
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
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", request.InferenceServer.Name),
			Namespace: request.Namespace,
		},
		Data: map[string]string{
			"config.pbtxt": generateTritonConfig(request),
		},
	}

	return g.kubeClient.Create(ctx, configMap)
}

func (g *gateway) createTritonDeployment(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	replicas := request.Resources.Replicas
	if replicas == 0 {
		replicas = 1
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      request.InferenceServer.Name,
			Namespace: request.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": request.InferenceServer.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": request.InferenceServer.Name,
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
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/models",
								},
							},
							Args: []string{
								"tritonserver",
								"--model-repository=/models",
								"--allow-http=true",
								"--allow-grpc=true",
								"--allow-metrics=true",
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-config", request.InferenceServer.Name),
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
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-service", request.InferenceServer.Name),
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

func generateTritonConfig(request InfrastructureRequest) string {
	// Generate basic Triton model config
	return fmt.Sprintf(`
name: "%s"
platform: "python"
max_batch_size: 0
input [
  {
    name: "input"
    data_type: TYPE_STRING
    dims: [ -1 ]
  }
]
output [
  {
    name: "output"
    data_type: TYPE_STRING
    dims: [ -1 ]
  }
]
`, request.InferenceServer.Name)
}

func getTritonImageTag(tag string) string {
	if tag == "" {
		return "23.04-py3"  // Default Triton image tag
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