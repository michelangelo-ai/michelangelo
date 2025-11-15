package backends

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/shared/configmap"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Triton Infrastructure Management
type tritonBackend struct {
	kubeClient        client.Client
	dynamicClient     dynamic.Interface
	configMapProvider configmap.ConfigMapProvider
	logger            *zap.Logger
}

func NewTritonBackend(kubeClient client.Client, dynamicClient dynamic.Interface, configMapProvider configmap.ConfigMapProvider, logger *zap.Logger) *tritonBackend {
	return &tritonBackend{kubeClient: kubeClient, dynamicClient: dynamicClient, configMapProvider: configMapProvider, logger: logger}
}

func (b *tritonBackend) CreateInfrastructure(ctx context.Context, logger *zap.Logger, request gateways.CreateInfrastructureRequest) (*gateways.CreateInfrastructureResponse, error) {
	logger.Info("Creating Triton infrastructure", zap.String("server", request.InferenceServer.Name))

	// Create routing resource - prefer HTTPRoute (Gateway API) over VirtualService (Istio-specific)
	// Check if Gateway API is available by trying to create HTTPRoute first
	if err := b.createInferenceServerHTTPRoute(ctx, logger, request); err != nil {
		logger.Info("HTTPRoute creation failed, falling back to VirtualService", zap.Error(err))
		// Fallback to VirtualService if HTTPRoute fails
		if err := b.createInferenceServerVirtualService(ctx, logger, request); err != nil {
			return nil, fmt.Errorf("failed to create both HTTPRoute and VirtualService: %w", err)
		}
	} else {
		logger.Info("Successfully created HTTPRoute for inference server", zap.String("server", request.InferenceServer.Name))
	}

	// Create Deployment
	if err := b.createTritonDeployment(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to create Deployment: %w", err)
	}

	// Create Service
	if err := b.createTritonService(ctx, logger, request); err != nil {
		return nil, fmt.Errorf("failed to create Service: %w", err)
	}

	// Create empty ConfigMap for model configuration
	if err := b.configMapProvider.CreateModelConfigMap(ctx, configmap.ConfigMapRequest{
		InferenceServer: request.InferenceServer.Name,
		Namespace:       request.Namespace,
		BackendType:     request.BackendType,
		ModelConfigs:    []configmap.ModelConfigEntry{}, // Empty initially
	}); err != nil {
		return nil, fmt.Errorf("failed to create ConfigMap: %w", err)
	}

	return &gateways.CreateInfrastructureResponse{
		State:     v2pb.INFERENCE_SERVER_STATE_CREATING,
		Message:   "Triton infrastructure creation initiated with empty ConfigMap",
		Endpoints: []string{fmt.Sprintf("/%s-endpoint/%s", request.InferenceServer.Name, request.InferenceServer.Name)},
		Details:   map[string]interface{}{"backend": "triton", "namespace": request.Namespace},
	}, nil
}

func (b *tritonBackend) GetInfrastructureStatus(ctx context.Context, logger *zap.Logger, request gateways.GetInfrastructureStatusRequest) (*gateways.GetInfrastructureStatusResponse, error) {
	logger.Info("Getting Triton infrastructure status", zap.String("server", request.InferenceServer))

	// Check deployment status
	deployment := &appsv1.Deployment{}
	deploymentKey := client.ObjectKey{Name: fmt.Sprintf("triton-%s", request.InferenceServer), Namespace: request.Namespace}

	if err := b.kubeClient.Get(ctx, deploymentKey, deployment); err != nil {
		// When deployment doesn't exist, return CREATING state to trigger infrastructure creation
		return &gateways.GetInfrastructureStatusResponse{
			Status: gateways.InfrastructureStatus{
				State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
				Message: fmt.Sprintf("Deployment not found, needs creation: %v", err),
				Ready:   false,
			},
		}, nil
	}

	// Check if ConfigMap exists - if not, infrastructure is incomplete
	configMapName := fmt.Sprintf("%s-model-config", request.InferenceServer)
	configMap := &corev1.ConfigMap{}
	configMapKey := client.ObjectKey{Name: configMapName, Namespace: request.Namespace}

	if err := b.kubeClient.Get(ctx, configMapKey, configMap); err != nil {
		// ConfigMap doesn't exist, infrastructure is incomplete
		logger.Info("ConfigMap not found, infrastructure incomplete", zap.String("configMap", configMapName))
		return &gateways.GetInfrastructureStatusResponse{
			Status: gateways.InfrastructureStatus{
				State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
				Message: fmt.Sprintf("ConfigMap %s not found, infrastructure incomplete", configMapName),
				Ready:   false,
			},
		}, nil
	}

	// Check if deployment is ready
	ready := deployment.Status.ReadyReplicas == deployment.Status.Replicas && deployment.Status.Replicas > 0

	state := v2pb.INFERENCE_SERVER_STATE_CREATING
	if ready {
		state = v2pb.INFERENCE_SERVER_STATE_SERVING
	}

	return &gateways.GetInfrastructureStatusResponse{
		Status: gateways.InfrastructureStatus{
			State:   state,
			Message: fmt.Sprintf("Deployment status: %d/%d replicas ready", deployment.Status.ReadyReplicas, deployment.Status.Replicas),
			Ready:   ready,
			Endpoints: []string{
				fmt.Sprintf("http://%s-inference-service.%s.svc.cluster.local:80", request.InferenceServer, request.Namespace),
			},
		},
	}, nil
}

func (b *tritonBackend) DeleteInfrastructure(ctx context.Context, logger *zap.Logger, request gateways.DeleteInfrastructureRequest) error {
	logger.Info("Deleting Triton infrastructure", zap.String("server", request.InferenceServer), zap.String("backend", request.BackendType.String()))

	// Delete HTTPRoute (Gateway API)
	httpRouteGVR := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}
	httpRouteName := fmt.Sprintf("%s-httproute", request.InferenceServer)
	if err := b.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Delete(ctx, httpRouteName, metav1.DeleteOptions{}); err != nil {
		logger.Info("Failed to delete HTTPRoute (may not exist)", zap.String("name", httpRouteName), zap.Error(err))
	} else {
		logger.Info("HTTPRoute deleted successfully", zap.String("name", httpRouteName))
	}

	// Delete VirtualService (fallback/legacy)
	virtualServiceGVR := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}
	virtualServiceName := fmt.Sprintf("%s-virtualservice", request.InferenceServer)
	if err := b.dynamicClient.Resource(virtualServiceGVR).Namespace(request.Namespace).Delete(ctx, virtualServiceName, metav1.DeleteOptions{}); err != nil {
		logger.Info("Failed to delete VirtualService (may not exist)", zap.String("name", virtualServiceName), zap.Error(err))
	} else {
		logger.Info("VirtualService deleted successfully", zap.String("name", virtualServiceName))
	}

	// Delete Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("triton-%s", request.InferenceServer),
			Namespace: request.Namespace,
		},
	}
	if err := b.kubeClient.Delete(ctx, deployment); err != nil {
		logger.Error("Failed to delete deployment", zap.Error(err))
	}

	// Delete Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-inference-service", request.InferenceServer),
			Namespace: request.Namespace,
		},
	}
	if err := b.kubeClient.Delete(ctx, service); err != nil {
		logger.Error("Failed to delete service", zap.Error(err))
	}

	// Delete ConfigMap using the gateway's ConfigMap deletion method
	if err := b.configMapProvider.DeleteModelConfigMap(ctx, request.InferenceServer, request.Namespace); err != nil {
		logger.Error("Failed to delete ConfigMap", zap.Error(err))
	} else {
		logger.Info("ConfigMap deleted successfully", zap.String("name", fmt.Sprintf("%s-model-config", request.InferenceServer)))
	}

	return nil
}

// Triton Model Management

func (b *tritonBackend) LoadModel(ctx context.Context, logger *zap.Logger, request gateways.LoadModelRequest) error {
	logger.Info("Loading Triton model explicitly", zap.String("model", request.ModelName), zap.String("server", request.InferenceServer))

	// Call Triton /v2/repository/models/{model}/load endpoint
	// Use localhost when running outside cluster (via bazel run)
	// serviceURL := fmt.Sprintf("http://%s-inference-service.%s.svc.cluster.local:80", request.InferenceServer, request.Namespace)
	serviceURL := "http://localhost:8889"
	loadURL := fmt.Sprintf("%s/%s/v2/repository/models/%s/load", serviceURL, request.InferenceServer, request.ModelName)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create load request body
	loadRequestBody := map[string]interface{}{}
	bodyBytes, err := json.Marshal(loadRequestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal load request: %w", err)
	}

	// Make POST request to load model
	req, err := http.NewRequestWithContext(ctx, "POST", loadURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create load request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call Triton load endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Triton load failed with status %d: %s", resp.StatusCode, string(body))
	}

	logger.Info("Triton model loaded successfully", zap.String("model", request.ModelName))
	return nil
}

func (b *tritonBackend) UnloadModel(ctx context.Context, logger *zap.Logger, request gateways.UnloadModelRequest) error {
	// Construct Triton unload API endpoint
	unloadURL := fmt.Sprintf("http://localhost:8889/%s/v2/repository/models/%s/unload", request.InferenceServer, request.ModelName)

	logger.Info("Calling Triton unload API", zap.String("url", unloadURL), zap.String("model", request.ModelName))

	// Create HTTP request to unload model
	req, err := http.NewRequestWithContext(ctx, "POST", unloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create unload request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call Triton unload API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Triton unload API returned status %d", resp.StatusCode)
	}

	logger.Info("Successfully called Triton unload API", zap.String("model", request.ModelName), zap.Int("status", resp.StatusCode))
	return nil
}

func (b *tritonBackend) CheckModelStatus(ctx context.Context, logger *zap.Logger, request gateways.CheckModelStatusRequest) (bool, error) {
	logger.Info("Checking Triton model status", zap.String("model", request.ModelName), zap.String("server", request.InferenceServer))

	// Call Triton /v2/models/{model}/ready endpoint with deployment-specific routing
	// Use localhost when running outside cluster (via bazel run)
	// serviceURL := fmt.Sprintf("http://%s-inference-service.%s.svc.cluster.local:80", request.InferenceServer, request.Namespace)
	serviceURL := "http://localhost:8889"

	// Include deployment name in URL path for deployment-specific routing
	var readyURL string
	// if request.DeploymentName != "" {
	// 	readyURL = fmt.Sprintf("%s/%s/%s/v2/models/%s/ready", serviceURL, request.InferenceServer, request.DeploymentName, request.ModelName)
	// } else {
	// 	readyURL = fmt.Sprintf("%s/%s/v2/models/%s/ready", serviceURL, request.InferenceServer, request.ModelName)
	// }
	readyURL = fmt.Sprintf("%s/%s/v2/models/%s/ready", serviceURL, request.InferenceServer, request.ModelName)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", readyURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create ready request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to call Triton ready endpoint: %w", err)
	}
	defer resp.Body.Close()

	// Model is ready if status is 200
	ready := resp.StatusCode == http.StatusOK
	logger.Info("Triton model ready status", zap.String("model", request.ModelName), zap.Bool("ready", ready), zap.Int("statusCode", resp.StatusCode))
	return ready, nil
}

func (b *tritonBackend) IsHealthy(ctx context.Context, logger *zap.Logger, serverName string) (bool, error) {
	logger.Info("Checking Triton health via Kubernetes pod status", zap.String("server", serverName))

	// Following Uber's approach: Check Kubernetes resource status instead of HTTP endpoints
	// Get the Triton deployment status from Kubernetes
	deploymentName := fmt.Sprintf("triton-%s", serverName)
	namespace := "default" // TODO: Make configurable

	deployment := &appsv1.Deployment{}
	err := b.kubeClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: namespace}, deployment)
	if err != nil {
		logger.Info("Failed to get Triton deployment", zap.String("deployment", deploymentName), zap.Error(err))
		return false, fmt.Errorf("failed to get deployment %s: %w", deploymentName, err)
	}

	// Check deployment conditions following Uber's pattern
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable {
			if condition.Status == corev1.ConditionTrue {
				logger.Info("Triton deployment is available", zap.String("server", serverName))

				// Also check if pods are ready (additional safety check)
				if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas == deployment.Status.Replicas {
					logger.Info("Triton pods are ready", zap.String("server", serverName), zap.Int("readyReplicas", int(deployment.Status.ReadyReplicas)))
					return true, nil
				} else {
					logger.Info("Triton deployment available but pods not ready",
						zap.String("server", serverName),
						zap.Int("readyReplicas", int(deployment.Status.ReadyReplicas)),
						zap.Int("totalReplicas", int(deployment.Status.Replicas)),
					)
					return false, fmt.Errorf("pods not ready: %d/%d", deployment.Status.ReadyReplicas, deployment.Status.Replicas)
				}
			} else {
				logger.Info("Triton deployment not available",
					zap.String("server", serverName),
					zap.String("reason", condition.Reason),
					zap.String("message", condition.Message))
				return false, fmt.Errorf("deployment not available: %s", condition.Message)
			}
		}
	}

	logger.Info("Triton deployment status unclear", zap.String("server", serverName))
	return false, fmt.Errorf("deployment status unclear")
}

func (b *tritonBackend) createTritonDeployment(ctx context.Context, logger *zap.Logger, request gateways.CreateInfrastructureRequest) error {
	deploymentName := fmt.Sprintf("triton-%s", request.InferenceServer.Name)

	// Check if Deployment already exists
	existing := &appsv1.Deployment{}
	err := b.kubeClient.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: request.Namespace}, existing)
	if err == nil {
		// Deployment already exists, log and return success
		logger.Info("Deployment already exists, skipping creation", zap.String("name", deploymentName))
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
						{
							Name:    "model-sync",
							Image:   "amazon/aws-cli:2.15.50",
							Command: []string{"/bin/sh", "-c"},
							Args: []string{
								`yum install -y jq curl && \
CONFIG_FILE=/secret/localMinIO.json
ACCESS_KEY=$(jq -r '.access_key_id' $CONFIG_FILE)
SECRET_KEY=$(jq -r '.secret_access_key' $CONFIG_FILE)
ENDPOINT=$(jq -r '.endpoint_url' $CONFIG_FILE)
REGION=$(jq -r '.region' $CONFIG_FILE)
aws configure set aws_access_key_id $ACCESS_KEY
aws configure set aws_secret_access_key $SECRET_KEY
aws configure set default.region $REGION
aws configure set default.s3.endpoint_url $ENDPOINT

# Function to get currently loaded models from Triton
get_loaded_models() {
  curl -s http://localhost:8000/v2/models 2>/dev/null | jq -r '.models[]?.name // empty' 2>/dev/null || echo ""
}

# Function to load model in Triton
load_model() {
  local model_name=$1
  echo "Loading model $model_name in Triton"
  curl -s -X POST "http://localhost:8000/v2/repository/models/$model_name/load" -H "Content-Type: application/json" -d '{}'
  if [ $? -eq 0 ]; then
    echo "Model $model_name loaded successfully"
  else
    echo "Failed to load model $model_name"
  fi
}

# Function to unload model in Triton
unload_model() {
  local model_name=$1
  echo "Unloading model $model_name from Triton"
  curl -s -X POST "http://localhost:8000/v2/repository/models/$model_name/unload" -H "Content-Type: application/json" -d '{}'
  if [ $? -eq 0 ]; then
    echo "Model $model_name unloaded successfully"
  else
    echo "Failed to unload model $model_name"
  fi
}

# Wait for Triton to be ready
echo "Waiting for Triton server to be ready..."
while ! curl -s http://localhost:8000/v2/health/ready > /dev/null 2>&1; do
  echo "Triton not ready, waiting..."
  sleep 5
done
echo "Triton server is ready"

while true; do
  echo "Starting UCS-style model sync cycle"

  # SIMPLIFIED PATTERN: Only read from shared model-config ConfigMap
  # Read inference server model config (the only source of truth)
  cp /config/model-list.json /tmp/model-list.json 2>/dev/null || echo "[]" > /tmp/model-list.json

  # Get models from shared inference server config
  DESIRED_MODELS=$(jq -r '.[].name' /tmp/model-list.json 2>/dev/null | grep -v '^$' | sort -u || echo "")

  echo "Active models from shared ConfigMap: $DESIRED_MODELS"

  # Get currently loaded models from Triton
  LOADED_MODELS=$(get_loaded_models)
  
  # SYNC PATTERN: Sync models based on DESIRED_MODELS from shared ConfigMap
  for desired_model in $DESIRED_MODELS; do
    if [ ! -z "$desired_model" ]; then
      # Look up S3 path from inference server config for this model
      s3_path=$(jq -r --arg model "$desired_model" '.[] | select(.name == $model) | .s3_path' /tmp/model-list.json 2>/dev/null)
      if [ "$s3_path" = "null" ] || [ -z "$s3_path" ]; then
        s3_path="s3://deploy-models/$desired_model/"  # Default S3 path pattern
      fi

      if [ ! -d "/mnt/models/$desired_model" ] || [ -z "$(ls -A /mnt/models/$desired_model)" ]; then
        echo "SYNC: Syncing active model $desired_model from $s3_path to /mnt/models/$desired_model/"
        mkdir -p "/mnt/models/$desired_model"
        aws s3 sync "$s3_path" "/mnt/models/$desired_model/" --exact-timestamps --endpoint-url "$ENDPOINT"
      else
        echo "SYNC: Model $desired_model already synced locally, skipping download"
      fi
    fi
  done
  
  # Unload models that are no longer in config
  for loaded_model in $LOADED_MODELS; do
    if ! echo "$DESIRED_MODELS" | grep -q "^$loaded_model$"; then
      echo "Model $loaded_model no longer in config, unloading"
      unload_model "$loaded_model"
    fi
  done
  
  # Load models from config ONLY if they're not already loaded
  for desired_model in $DESIRED_MODELS; do
    if [ ! -z "$desired_model" ]; then
      # Get fresh list of loaded models before checking each model
      CURRENT_LOADED_MODELS=$(get_loaded_models)
      if ! echo "$CURRENT_LOADED_MODELS" | grep -q "^$desired_model$"; then
        echo "Model $desired_model not loaded, loading now"
        load_model "$desired_model"
      else
        echo "Model $desired_model already loaded, skipping"
      fi
    fi
  done
  
  echo "Model sync cycle completed, sleeping for 60 seconds"
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
										Name: fmt.Sprintf("%s-model-config", request.InferenceServer.Name),
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

	return b.kubeClient.Create(ctx, deployment)
}

func (b *tritonBackend) createTritonService(ctx context.Context, logger *zap.Logger, request gateways.CreateInfrastructureRequest) error {
	serviceName := fmt.Sprintf("%s-inference-service", request.InferenceServer.Name)

	// Check if Service already exists
	existing := &corev1.Service{}
	err := b.kubeClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: request.Namespace}, existing)
	if err == nil {
		// Service already exists, log and return success
		logger.Info("Service already exists, skipping creation", zap.String("name", serviceName))
		return nil
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: request.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": fmt.Sprintf("triton-%s", request.InferenceServer.Name),
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

	return b.kubeClient.Create(ctx, service)
}

func (b *tritonBackend) createInferenceServerVirtualService(ctx context.Context, logger *zap.Logger, request gateways.CreateInfrastructureRequest) error {
	gvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	virtualServiceName := fmt.Sprintf("%s-virtualservice", request.InferenceServer.Name)

	// Check if VirtualService already exists
	_, err := b.dynamicClient.Resource(gvr).Namespace(request.Namespace).Get(ctx, virtualServiceName, metav1.GetOptions{})
	if err == nil {
		logger.Info("VirtualService already exists, skipping creation", zap.String("name", virtualServiceName))
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

	_, err = b.dynamicClient.Resource(gvr).Namespace(request.Namespace).Create(ctx, vs, metav1.CreateOptions{})
	if err != nil {
		logger.Error("Failed to create VirtualService", zap.Error(err))
		return err
	}

	logger.Info("VirtualService created successfully", zap.String("name", virtualServiceName))
	return nil
}

// createInferenceServerHTTPRoute creates a HTTPRoute for the inference server using generic Gateway API
func (b *tritonBackend) createInferenceServerHTTPRoute(ctx context.Context, logger *zap.Logger, request gateways.CreateInfrastructureRequest) error {
	gvr := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}

	httpRouteName := fmt.Sprintf("%s-httproute", request.InferenceServer.Name)

	// Check if HTTPRoute already exists
	existingHTTPRoute, err := b.dynamicClient.Resource(gvr).Namespace(request.Namespace).Get(ctx, httpRouteName, metav1.GetOptions{})
	if err == nil {
		logger.Info("HTTPRoute already exists, checking if update needed", zap.String("name", httpRouteName))

		// Check if the existing HTTPRoute has the correct configuration
		if needsHTTPRouteUpdate(existingHTTPRoute, request) {
			logger.Info("HTTPRoute configuration outdated, updating", zap.String("name", httpRouteName))
			return b.updateHTTPRoute(ctx, logger, existingHTTPRoute, request, gvr)
		}

		logger.Info("HTTPRoute configuration is up-to-date, skipping", zap.String("name", httpRouteName))
		return nil
	}

	hr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      httpRouteName,
				"namespace": request.Namespace,
				"labels": map[string]string{
					"app":                        "inference-server",
					"inference-server":           request.InferenceServer.Name,
					"michelangelo.ai/managed-by": "controller",
				},
			},
			"spec": map[string]interface{}{
				"parentRefs": []map[string]interface{}{
					{
						"name":      "ma-gateway",
						"namespace": "default",
					},
				},
				"rules": []map[string]interface{}{
					{
						// Baseline inference server endpoint - routes to whatever model is loaded in Triton
						"matches": []map[string]interface{}{
							{
								"path": map[string]interface{}{
									"type":  "PathPrefix",
									"value": fmt.Sprintf("/%s", request.InferenceServer.Name),
								},
							},
						},
						"filters": []map[string]interface{}{
							{
								"type": "URLRewrite",
								"urlRewrite": map[string]interface{}{
									"path": map[string]interface{}{
										"type":               "ReplacePrefixMatch",
										"replacePrefixMatch": "/",
									},
								},
							},
						},
						"backendRefs": []map[string]interface{}{
							{
								"name":   fmt.Sprintf("%s-inference-service", request.InferenceServer.Name),
								"port":   80,
								"weight": 100,
							},
						},
					},
				},
			},
		},
	}

	_, err = b.dynamicClient.Resource(gvr).Namespace(request.Namespace).Create(ctx, hr, metav1.CreateOptions{})
	if err != nil {
		logger.Error("Failed to create HTTPRoute", zap.Error(err))
		return err
	}

	logger.Info("HTTPRoute created successfully", zap.String("name", httpRouteName))
	return nil
}

// Helper functions

// updateTritonModelConfig updates the model configuration for rolling out new models
// updateTritonModelConfig has been moved to the ConfigMap provider
// This backend no longer manages ConfigMaps directly

func getTritonImageTag(tag string) string {
	if tag == "" {
		return "23.04-py3" // Default Triton image tag
	}
	return tag
}

func buildResourceRequirements(resources gateways.ResourceSpec) corev1.ResourceRequirements {
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

// needsHTTPRouteUpdate checks if an existing HTTPRoute needs to be updated
func needsHTTPRouteUpdate(existingHTTPRoute *unstructured.Unstructured, request gateways.CreateInfrastructureRequest) bool {
	// Extract the existing rules
	rules, found, err := unstructured.NestedSlice(existingHTTPRoute.Object, "spec", "rules")
	if err != nil || !found || len(rules) == 0 {
		return true // Update if we can't parse rules
	}

	// Check health endpoint rule (first rule)
	if len(rules) > 0 {
		firstRule, ok := rules[0].(map[string]interface{})
		if !ok {
			return true
		}

		// Check if health endpoint has correct rewrite
		filters, found, _ := unstructured.NestedSlice(firstRule, "filters")
		if !found || len(filters) == 0 {
			return true
		}

		for _, filter := range filters {
			filterMap, ok := filter.(map[string]interface{})
			if !ok {
				continue
			}

			if filterType, ok := filterMap["type"]; ok && filterType == "URLRewrite" {
				urlRewrite, found, _ := unstructured.NestedMap(filterMap, "urlRewrite")
				if !found {
					return true
				}

				pathMap, found, _ := unstructured.NestedMap(urlRewrite, "path")
				if !found {
					return true
				}

				replacePrefixMatch, ok := pathMap["replacePrefixMatch"]
				if !ok {
					return true
				}

				// This is the key check - does it have the correct path?
				if replacePrefixMatch != "/v2/health/ready" {
					return true // Needs update
				}
			}
		}
	}

	return false // Configuration is up-to-date
}

// updateHTTPRoute updates an existing HTTPRoute with the correct configuration
func (b *tritonBackend) updateHTTPRoute(ctx context.Context, logger *zap.Logger, existingHTTPRoute *unstructured.Unstructured, request gateways.CreateInfrastructureRequest, gvr schema.GroupVersionResource) error {
	// Delete the existing HTTPRoute and create a new one to avoid deep copy issues
	logger.Info("Deleting existing HTTPRoute to recreate with correct configuration", zap.String("name", existingHTTPRoute.GetName()))

	if err := b.dynamicClient.Resource(gvr).Namespace(request.Namespace).Delete(ctx, existingHTTPRoute.GetName(), metav1.DeleteOptions{}); err != nil {
		logger.Error("Failed to delete existing HTTPRoute", zap.Error(err))
		return err
	}

	// Wait a moment for deletion to complete
	time.Sleep(100 * time.Millisecond)

	// Create new HTTPRoute with baseline configuration
	logger.Info("Creating new HTTPRoute with baseline configuration", zap.String("name", existingHTTPRoute.GetName()))

	hr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      existingHTTPRoute.GetName(),
				"namespace": request.Namespace,
				"labels": map[string]string{
					"app":                        "inference-server",
					"inference-server":           request.InferenceServer.Name,
					"michelangelo.ai/managed-by": "controller",
				},
			},
			"spec": map[string]interface{}{
				"parentRefs": []map[string]interface{}{
					{
						"name":      "ma-gateway",
						"namespace": "default",
					},
				},
				"rules": []map[string]interface{}{
					{
						// Baseline inference server endpoint - routes to whatever model is loaded in Triton
						"matches": []map[string]interface{}{
							{
								"path": map[string]interface{}{
									"type":  "PathPrefix",
									"value": fmt.Sprintf("/%s", request.InferenceServer.Name),
								},
							},
						},
						"filters": []map[string]interface{}{
							{
								"type": "URLRewrite",
								"urlRewrite": map[string]interface{}{
									"path": map[string]interface{}{
										"type":               "ReplacePrefixMatch",
										"replacePrefixMatch": "/",
									},
								},
							},
						},
						"backendRefs": []map[string]interface{}{
							{
								"name":   fmt.Sprintf("%s-inference-service", request.InferenceServer.Name),
								"port":   80,
								"weight": 100,
							},
						},
					},
				},
			},
		},
	}

	_, err := b.dynamicClient.Resource(gvr).Namespace(request.Namespace).Create(ctx, hr, metav1.CreateOptions{})
	if err != nil {
		logger.Error("Failed to create new HTTPRoute", zap.Error(err))
		return err
	}

	logger.Info("HTTPRoute recreated successfully", zap.String("name", existingHTTPRoute.GetName()))
	return nil
}
