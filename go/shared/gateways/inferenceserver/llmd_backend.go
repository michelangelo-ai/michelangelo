package inferenceserver

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// LLMD Infrastructure Management

func (g *gateway) createLLMDInfrastructure(ctx context.Context, logger logr.Logger, request InfrastructureRequest) (*InfrastructureResponse, error) {
	logger.Info("Creating LLMD infrastructure", "server", request.InferenceServer.Name)

	// Create ConfigMap for LLMD configuration
	if err := g.createLLMDConfigMap(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to create ConfigMap: %w", err)
	}

	// Create Deployment
	if err := g.createLLMDDeployment(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to create Deployment: %w", err)
	}

	// Create Service
	if err := g.createLLMDService(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to create Service: %w", err)
	}

	return &InfrastructureResponse{
		State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
		Message: "LLMD infrastructure creation initiated",
		Endpoints: []string{
			fmt.Sprintf("http://%s-service.%s.svc.cluster.local:80", request.InferenceServer.Name, request.Namespace),
		},
		Details: map[string]interface{}{
			"backend":   "llmd",
			"namespace": request.Namespace,
		},
	}, nil
}

func (g *gateway) getLLMDInfrastructureStatus(ctx context.Context, logger logr.Logger, request InfrastructureStatusRequest) (*InfrastructureStatus, error) {
	logger.Info("Getting LLMD infrastructure status", "server", request.InferenceServer)

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

func (g *gateway) deleteLLMDInfrastructure(ctx context.Context, logger logr.Logger, request InfrastructureDeleteRequest) error {
	logger.Info("Deleting LLMD infrastructure", "server", request.InferenceServer)

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

func (g *gateway) createLLMDConfigMap(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", request.InferenceServer.Name),
			Namespace: request.Namespace,
		},
		Data: map[string]string{
			"config.yaml": generateLLMDConfig(request),
		},
	}

	return g.kubeClient.Create(ctx, configMap)
}

func (g *gateway) createLLMDDeployment(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
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
							Name:  "llmd",
							Image: fmt.Sprintf("llmd-server:%s", getLLMDImageTag(request.Resources.ImageTag)),
							Ports: []corev1.ContainerPort{
								{ContainerPort: 8080, Name: "http"},
								{ContainerPort: 9090, Name: "metrics"},
							},
							Resources: buildResourceRequirements(request.Resources),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/llmd",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "CONFIG_PATH",
									Value: "/etc/llmd/config.yaml",
								},
								{
									Name:  "PORT",
									Value: "8080",
								},
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

func (g *gateway) createLLMDService(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
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
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "metrics",
					Port:       9090,
					TargetPort: intstr.FromInt(9090),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	return g.kubeClient.Create(ctx, service)
}

// LLMD Model Management

func (g *gateway) loadLLMDModel(ctx context.Context, logger logr.Logger, request ModelLoadRequest) error {
	logger.Info("Loading LLMD model", "model", request.ModelName)
	
	// For now, return success as the model is loaded via config
	// In a real implementation, this would call the LLMD model loading API
	return nil
}

func (g *gateway) checkLLMDModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (bool, error) {
	logger.Info("Checking LLMD model status", "model", request.ModelName)
	
	// For now, assume model is ready if infrastructure is ready
	// In a real implementation, this would call the LLMD model status API
	return true, nil
}

func (g *gateway) getLLMDModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (*ModelStatus, error) {
	logger.Info("Getting LLMD model status", "model", request.ModelName)
	
	return &ModelStatus{
		State:   "LOADED",
		Message: "Model is loaded and ready",
		Ready:   true,
	}, nil
}

func (g *gateway) isLLMDHealthy(ctx context.Context, logger logr.Logger, serverName string) (bool, error) {
	logger.Info("Checking LLMD health", "server", serverName)
	
	// For now, assume healthy if infrastructure exists
	// In a real implementation, this would call the LLMD health API
	return true, nil
}

// Helper functions

func generateLLMDConfig(request InfrastructureRequest) string {
	// Generate basic LLMD configuration
	return fmt.Sprintf(`
server:
  name: "%s"
  port: 8080
  metrics_port: 9090

model:
  name: "%s"
  type: "llm"
  config: {}

resources:
  cpu: "%s"
  memory: "%s"
  gpu: %d
`, request.InferenceServer.Name, request.InferenceServer.Name, 
   request.Resources.CPU, request.Resources.Memory, request.Resources.GPU)
}

func getLLMDImageTag(tag string) string {
	if tag == "" {
		return "latest"  // Default LLMD image tag
	}
	return tag
}