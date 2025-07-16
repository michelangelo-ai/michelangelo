package gateways

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TorchServe Infrastructure Management

func (g *gateway) createTorchServeInfrastructure(ctx context.Context, logger logr.Logger, request InfrastructureRequest) (*InfrastructureResponse, error) {
	logger.Info("Creating TorchServe infrastructure", "server", request.InferenceServer.Name)

	// Create ConfigMap for model configuration
	if err := g.createTorchServeConfigMap(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to create ConfigMap: %w", err)
	}

	// Create Deployment
	if err := g.createTorchServeDeployment(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to create Deployment: %w", err)
	}

	// Create Service
	if err := g.createTorchServeService(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to create Service: %w", err)
	}

	return &InfrastructureResponse{
		State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
		Message: "TorchServe infrastructure creation initiated",
		Endpoints: []string{
			fmt.Sprintf("http://%s-service.%s.svc.cluster.local:8080", request.InferenceServer.Name, request.Namespace),
		},
		Details: map[string]interface{}{
			"backend":   "torchserve",
			"namespace": request.Namespace,
		},
	}, nil
}

func (g *gateway) getTorchServeInfrastructureStatus(ctx context.Context, logger logr.Logger, request InfrastructureStatusRequest) (*InfrastructureStatus, error) {
	logger.Info("Getting TorchServe infrastructure status", "server", request.InferenceServer)

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
			fmt.Sprintf("http://%s-service.%s.svc.cluster.local:8080", request.InferenceServer, request.Namespace),
		},
	}, nil
}

func (g *gateway) deleteTorchServeInfrastructure(ctx context.Context, logger logr.Logger, request InfrastructureDeleteRequest) error {
	logger.Info("Deleting TorchServe infrastructure", "server", request.InferenceServer)

	// Delete Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      request.InferenceServer,
			Namespace: request.Namespace,
		},
	}
	if err := g.kubeClient.Delete(ctx, deployment); err != nil {
		logger.Error(err, "Failed to delete TorchServe deployment")
	}

	// Delete Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-service", request.InferenceServer),
			Namespace: request.Namespace,
		},
	}
	if err := g.kubeClient.Delete(ctx, service); err != nil {
		logger.Error(err, "Failed to delete TorchServe service")
	}

	// Delete ConfigMap
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-models", request.InferenceServer),
			Namespace: request.Namespace,
		},
	}
	if err := g.kubeClient.Delete(ctx, configMap); err != nil {
		logger.Error(err, "Failed to delete TorchServe ConfigMap")
	}

	return nil
}

func (g *gateway) createTorchServeDeployment(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	logger.Info("Creating TorchServe deployment", "name", request.InferenceServer.Name)

	// TorchServe container configuration
	container := corev1.Container{
		Name:  "torchserve",
		Image: "pytorch/torchserve:0.8.2-cpu", // Use CPU version by default
		Ports: []corev1.ContainerPort{
			{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
			{Name: "grpc", ContainerPort: 7070, Protocol: corev1.ProtocolTCP},
			{Name: "metrics", ContainerPort: 8082, Protocol: corev1.ProtocolTCP},
		},
		Command: []string{
			"torchserve",
			"--start",
			"--ts-config", "/config/config.properties",
			"--model-store", "/mnt/models",
			"--models", "all",
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(request.Resources.CPU),
				corev1.ResourceMemory: resource.MustParse(request.Resources.Memory),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(request.Resources.CPU),
				corev1.ResourceMemory: resource.MustParse(request.Resources.Memory),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "model-config", MountPath: "/config"},
			{Name: "workdir", MountPath: "/mnt/models"},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/ping",
					Port: intstr.FromInt(8080),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/ping",
					Port: intstr.FromInt(8080),
				},
			},
			InitialDelaySeconds: 60,
			PeriodSeconds:       30,
		},
	}

	// Model sync sidecar for downloading models
	modelSyncContainer := corev1.Container{
		Name:  "model-sync",
		Image: "amazon/aws-cli:2.15.50",
		Command: []string{"/bin/sh", "-c"},
		Args: []string{`
			yum install -y jq && \
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
			    echo "Syncing TorchServe model $name from $s3_path to /mnt/models/"
			    aws s3 sync "$s3_path" /mnt/models/ --delete --exact-timestamps --endpoint-url "$ENDPOINT"
			  done
			  
			  sleep 60
			done
		`},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "model-config", MountPath: "/config"},
			{Name: "workdir", MountPath: "/mnt/models"},
			{Name: "storage-secret", MountPath: "/secret"},
		},
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      request.InferenceServer.Name,
			Namespace: request.Namespace,
			Labels: map[string]string{
				"app": request.InferenceServer.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &request.Resources.Replicas,
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
					Containers: []corev1.Container{container, modelSyncContainer},
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

func (g *gateway) createTorchServeService(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	logger.Info("Creating TorchServe service", "name", request.InferenceServer.Name)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-service", request.InferenceServer.Name),
			Namespace: request.Namespace,
			Labels: map[string]string{
				"app": request.InferenceServer.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": request.InferenceServer.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8080,
					TargetPort: intstr.FromString("http"),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "grpc",
					Port:       7070,
					TargetPort: intstr.FromString("grpc"),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "metrics",
					Port:       8082,
					TargetPort: intstr.FromString("metrics"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	return g.kubeClient.Create(ctx, service)
}

func (g *gateway) createTorchServeConfigMap(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	logger.Info("Creating TorchServe ConfigMap", "name", request.InferenceServer.Name)

	// TorchServe configuration
	configProperties := `
inference_address=http://0.0.0.0:8080
management_address=http://0.0.0.0:8081
metrics_address=http://0.0.0.0:8082
grpc_inference_port=7070
grpc_management_port=7071
enable_envvars_config=true
install_py_dep_per_model=true
enable_metrics_api=true
metrics_format=prometheus
number_of_netty_threads=4
job_queue_size=10
number_of_gpu=0
model_store=/mnt/models
model_snapshot={"name":"startup.cfg","modelCount":0,"models":{}}
`

	// Model list configuration (will be updated by deployment actors)
	modelList := fmt.Sprintf(`[
  {
    "name": "%s",
    "s3_path": "s3://deploy-models/default-model"
  }
]`, request.InferenceServer.Name)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-models", request.InferenceServer.Name),
			Namespace: request.Namespace,
			Labels: map[string]string{
				"app": request.InferenceServer.Name,
			},
		},
		Data: map[string]string{
			"config.properties": configProperties,
			"model-list.json":   modelList,
		},
	}

	return g.kubeClient.Create(ctx, configMap)
}

// TorchServe Model Management

func (g *gateway) loadTorchServeModel(ctx context.Context, logger logr.Logger, request ModelLoadRequest) error {
	logger.Info("Loading TorchServe model", "model", request.ModelName, "server", request.InferenceServer)

	// Update ConfigMap with new model configuration
	updateRequest := ModelConfigUpdateRequest{
		InferenceServer: request.InferenceServer,
		BackendType:     v2pb.BACKEND_TYPE_TORCHSERVE,
		ModelConfigs: []ModelConfigEntry{{
			Name:   request.ModelName,
			S3Path: fmt.Sprintf("s3://deploy-models/%s/", request.ModelName),
		}},
	}

	return g.UpdateModelConfig(ctx, logger, updateRequest)
}

func (g *gateway) updateTorchServeModelConfig(ctx context.Context, logger logr.Logger, request ModelConfigUpdateRequest) error {
	logger.Info("Updating TorchServe model configuration", "server", request.InferenceServer)

	// Get existing ConfigMap
	configMap := &corev1.ConfigMap{}
	configMapKey := client.ObjectKey{
		Name:      fmt.Sprintf("%s-models", request.InferenceServer),
		Namespace: request.Namespace,
	}

	if err := g.kubeClient.Get(ctx, configMapKey, configMap); err != nil {
		return fmt.Errorf("failed to get TorchServe ConfigMap: %w", err)
	}

	// Update model list
	modelList := "["
	for i, model := range request.ModelConfigs {
		if i > 0 {
			modelList += ","
		}
		modelList += fmt.Sprintf(`
  {
    "name": "%s",
    "s3_path": "%s"
  }`, model.Name, model.S3Path)
	}
	modelList += "\n]"

	// Update ConfigMap
	configMap.Data["model-list.json"] = modelList

	return g.kubeClient.Update(ctx, configMap)
}

func (g *gateway) getTorchServeModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (*ModelStatus, error) {
	logger.Info("Getting TorchServe model status", "model", request.ModelName, "server", request.InferenceServer)

	// For TorchServe, we can check model status via management API
	// For now, return a simple status based on deployment readiness
	infraStatus, err := g.getTorchServeInfrastructureStatus(ctx, logger, InfrastructureStatusRequest{
		InferenceServer: request.InferenceServer,
		Namespace:       "default", // Use default namespace
	})
	if err != nil {
		return &ModelStatus{
			State:   v2pb.INFERENCE_SERVER_STATE_FAILED,
			Message: fmt.Sprintf("Failed to get infrastructure status: %v", err),
		}, nil
	}

	if infraStatus.Ready {
		return &ModelStatus{
			State:   v2pb.INFERENCE_SERVER_STATE_SERVING,
			Message: "Model loaded and ready",
			Ready:   true,
		}, nil
	}

	return &ModelStatus{
		State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
		Message: "Model is loading",
		Ready:   false,
	}, nil
}

func (g *gateway) isTorchServeHealthy(ctx context.Context, logger logr.Logger, request HealthCheckRequest) (*HealthStatus, error) {
	logger.Info("Checking TorchServe health", "server", request.InferenceServer)

	// Check if deployment is ready
	infraStatus, err := g.getTorchServeInfrastructureStatus(ctx, logger, InfrastructureStatusRequest{
		InferenceServer: request.InferenceServer,
		Namespace:       "default", // Use default namespace
	})
	if err != nil {
		return &HealthStatus{
			Healthy: false,
			Message: fmt.Sprintf("Infrastructure check failed: %v", err),
		}, nil
	}

	return &HealthStatus{
		Healthy: infraStatus.Ready,
		Message: infraStatus.Message,
	}, nil
}