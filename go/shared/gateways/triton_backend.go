package gateways

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

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

	// Create routing resource - prefer HTTPRoute (Gateway API) over VirtualService (Istio-specific)
	// Check if Gateway API is available by trying to create HTTPRoute first
	if err := g.createInferenceServerHTTPRoute(ctx, logger, request); err != nil {
		logger.Info("HTTPRoute creation failed, falling back to VirtualService", "error", err)
		// Fallback to VirtualService if HTTPRoute fails
		if err := g.createInferenceServerVirtualService(ctx, logger, request); err != nil {
			return nil, fmt.Errorf("failed to create both HTTPRoute and VirtualService: %w", err)
		}
	} else {
		logger.Info("Successfully created HTTPRoute for inference server", "server", request.InferenceServer.Name)
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
		Message: "Triton infrastructure creation initiated (ConfigMap handled separately)",
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
	deploymentKey := client.ObjectKey{Name: fmt.Sprintf("triton-%s", request.InferenceServer), Namespace: request.Namespace}

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
			fmt.Sprintf("http://%s-inference-service.%s.svc.cluster.local:80", request.InferenceServer, request.Namespace),
		},
	}, nil
}

func (g *gateway) deleteTritonInfrastructure(ctx context.Context, logger logr.Logger, request InfrastructureDeleteRequest) error {
	logger.Info("Deleting Triton infrastructure", "server", request.InferenceServer)

	// Delete HTTPRoute (Gateway API)
	httpRouteGVR := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}
	httpRouteName := fmt.Sprintf("%s-http-route", request.InferenceServer)
	if err := g.dynamicClient.Resource(httpRouteGVR).Namespace(request.Namespace).Delete(ctx, httpRouteName, metav1.DeleteOptions{}); err != nil {
		logger.Info("Failed to delete HTTPRoute (may not exist)", "name", httpRouteName, "error", err)
	} else {
		logger.Info("HTTPRoute deleted successfully", "name", httpRouteName)
	}

	// Delete VirtualService (fallback/legacy)
	virtualServiceGVR := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}
	virtualServiceName := fmt.Sprintf("%s-virtualservice", request.InferenceServer)
	if err := g.dynamicClient.Resource(virtualServiceGVR).Namespace(request.Namespace).Delete(ctx, virtualServiceName, metav1.DeleteOptions{}); err != nil {
		logger.Info("Failed to delete VirtualService (may not exist)", "name", virtualServiceName, "error", err)
	} else {
		logger.Info("VirtualService deleted successfully", "name", virtualServiceName)
	}

	// Delete Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("triton-%s", request.InferenceServer),
			Namespace: request.Namespace,
		},
	}
	if err := g.kubeClient.Delete(ctx, deployment); err != nil {
		logger.Error(err, "Failed to delete deployment")
	}

	// Delete Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-inference-service", request.InferenceServer),
			Namespace: request.Namespace,
		},
	}
	if err := g.kubeClient.Delete(ctx, service); err != nil {
		logger.Error(err, "Failed to delete service")
	}

	// Note: ConfigMap deletion is handled by the ConfigMap provider, not the backend

	return nil
}

// createTritonConfigMap has been moved to the ConfigMap provider
// This backend no longer creates ConfigMaps directly

func (g *gateway) createTritonDeployment(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	deploymentName := fmt.Sprintf("triton-%s", request.InferenceServer.Name)

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
							Name:  "model-sync",
							Image: "amazon/aws-cli:2.15.50",
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
  echo "Starting model sync cycle"
  cp /config/model-list.json /tmp/model-list.json
  
  # Get current models from config
  DESIRED_MODELS=$(jq -r '.[].name' /tmp/model-list.json 2>/dev/null || echo "")
  
  # Get currently loaded models from Triton
  LOADED_MODELS=$(get_loaded_models)
  
  # Sync models from S3
  jq -c '.[]' /tmp/model-list.json | while read model; do
    name=$(echo "$model" | jq -r '.name')
    s3_path=$(echo "$model" | jq -r '.s3_path')
    echo "Syncing model $name from $s3_path to /mnt/models/$name/"
    mkdir -p "/mnt/models/$name"
    aws s3 sync "$s3_path" "/mnt/models/$name/" --delete --exact-timestamps --endpoint-url "$ENDPOINT"
  done
  
  # Unload models that are no longer in config
  for loaded_model in $LOADED_MODELS; do
    if ! echo "$DESIRED_MODELS" | grep -q "^$loaded_model$"; then
      echo "Model $loaded_model no longer in config, unloading"
      unload_model "$loaded_model"
    fi
  done
  
  # Load models from config
  for desired_model in $DESIRED_MODELS; do
    if [ ! -z "$desired_model" ]; then
      echo "Loading model $desired_model"
      load_model "$desired_model"
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

	return g.kubeClient.Create(ctx, deployment)
}

func (g *gateway) createTritonService(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	serviceName := fmt.Sprintf("%s-inference-service", request.InferenceServer.Name)

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

// createInferenceServerHTTPRoute creates a HTTPRoute for the inference server using generic Gateway API
func (g *gateway) createInferenceServerHTTPRoute(ctx context.Context, logger logr.Logger, request InfrastructureRequest) error {
	gvr := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}

	httpRouteName := fmt.Sprintf("%s-http-route", request.InferenceServer.Name)

	// Check if HTTPRoute already exists
	_, err := g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Get(ctx, httpRouteName, metav1.GetOptions{})
	if err == nil {
		logger.Info("HTTPRoute already exists, skipping creation", "name", httpRouteName)
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
						// Health check endpoint
						"matches": []map[string]interface{}{
							{
								"path": map[string]interface{}{
									"type":  "PathPrefix",
									"value": fmt.Sprintf("/%s-endpoint/%s/v2/health", request.InferenceServer.Name, request.InferenceServer.Name),
								},
							},
						},
						"filters": []map[string]interface{}{
							{
								"type": "URLRewrite",
								"urlRewrite": map[string]interface{}{
									"path": map[string]interface{}{
										"type":               "ReplacePrefixMatch",
										"replacePrefixMatch": "/v2/health",
									},
								},
							},
						},
						"backendRefs": []map[string]interface{}{
							{
								"name": fmt.Sprintf("%s-inference-service", request.InferenceServer.Name),
								"port": 80,
								"weight": 100,
							},
						},
					},
					{
						// Model endpoints
						"matches": []map[string]interface{}{
							{
								"path": map[string]interface{}{
									"type":  "PathPrefix",
									"value": fmt.Sprintf("/%s-endpoint/%s/v2/models", request.InferenceServer.Name, request.InferenceServer.Name),
								},
							},
						},
						"filters": []map[string]interface{}{
							{
								"type": "URLRewrite",
								"urlRewrite": map[string]interface{}{
									"path": map[string]interface{}{
										"type":               "ReplacePrefixMatch",
										"replacePrefixMatch": "/v2/models",
									},
								},
							},
						},
						"backendRefs": []map[string]interface{}{
							{
								"name": fmt.Sprintf("%s-inference-service", request.InferenceServer.Name),
								"port": 80,
								"weight": 100,
							},
						},
					},
					{
						// General endpoint with URL rewrite
						"matches": []map[string]interface{}{
							{
								"path": map[string]interface{}{
									"type": "PathPrefix",
									"value": fmt.Sprintf("/%s-endpoint/%s/",
										request.InferenceServer.Name,
										request.InferenceServer.Name),
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
								"name": fmt.Sprintf("%s-inference-service", request.InferenceServer.Name),
								"port": 80,
								"weight": 100,
							},
							// Example: Traffic splitting capability (commented for now)
							// {
							//     "name": fmt.Sprintf("%s-canary-service", request.InferenceServer.Name),
							//     "port": 80,
							//     "weight": 10,
							// },
						},
					},
				},
			},
		},
	}

	_, err = g.dynamicClient.Resource(gvr).Namespace(request.Namespace).Create(ctx, hr, metav1.CreateOptions{})
	if err != nil {
		logger.Error(err, "Failed to create HTTPRoute")
		return err
	}

	logger.Info("HTTPRoute created successfully", "name", httpRouteName)
	return nil
}

// Triton Model Management

func (g *gateway) loadTritonModel(ctx context.Context, logger logr.Logger, request ModelLoadRequest) error {
	logger.Info("Loading Triton model explicitly", "model", request.ModelName, "server", request.InferenceServer)

	// Call Triton /v2/repository/models/{model}/load endpoint
	serviceURL := fmt.Sprintf("http://%s-inference-service.%s.svc.cluster.local:8000", request.InferenceServer, request.Namespace)
	loadURL := fmt.Sprintf("%s/v2/repository/models/%s/load", serviceURL, request.ModelName)

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

	logger.Info("Triton model loaded successfully", "model", request.ModelName)
	return nil
}

func (g *gateway) checkTritonModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (bool, error) {
	logger.Info("Checking Triton model status", "model", request.ModelName, "server", request.InferenceServer)

	// Call Triton /v2/models/{model}/ready endpoint
	serviceURL := fmt.Sprintf("http://%s-inference-service.%s.svc.cluster.local:8000", request.InferenceServer, request.Namespace)
	readyURL := fmt.Sprintf("%s/v2/models/%s/ready", serviceURL, request.ModelName)

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
	logger.Info("Triton model ready status", "model", request.ModelName, "ready", ready, "statusCode", resp.StatusCode)
	return ready, nil
}

func (g *gateway) getTritonModelStatus(ctx context.Context, logger logr.Logger, request ModelStatusRequest) (*ModelStatus, error) {
	logger.Info("Getting Triton model status", "model", request.ModelName, "server", request.InferenceServer)

	// First check if model is ready
	ready, err := g.checkTritonModelStatus(ctx, logger, request)
	if err != nil {
		return &ModelStatus{
			State:   v2pb.INFERENCE_SERVER_STATE_FAILED,
			Message: fmt.Sprintf("Failed to check model status: %v", err),
			Ready:   false,
		}, nil
	}

	if ready {
		return &ModelStatus{
			State:   v2pb.INFERENCE_SERVER_STATE_SERVING,
			Message: "Model is loaded and ready for inference",
			Ready:   true,
		}, nil
	}

	// Check if model exists in repository
	serviceURL := fmt.Sprintf("http://%s-inference-service.%s.svc.cluster.local:8000", request.InferenceServer, request.Namespace)
	modelURL := fmt.Sprintf("%s/v2/models/%s", serviceURL, request.ModelName)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", modelURL, nil)
	if err != nil {
		return &ModelStatus{
			State:   v2pb.INFERENCE_SERVER_STATE_FAILED,
			Message: fmt.Sprintf("Failed to create model status request: %v", err),
			Ready:   false,
		}, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return &ModelStatus{
			State:   v2pb.INFERENCE_SERVER_STATE_FAILED,
			Message: fmt.Sprintf("Failed to get model info: %v", err),
			Ready:   false,
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &ModelStatus{
			State:   v2pb.INFERENCE_SERVER_STATE_FAILED,
			Message: "Model not found in repository",
			Ready:   false,
		}, nil
	}

	return &ModelStatus{
		State:   v2pb.INFERENCE_SERVER_STATE_CREATING,
		Message: "Model exists but not ready for inference",
		Ready:   false,
	}, nil
}

func (g *gateway) isTritonHealthy(ctx context.Context, logger logr.Logger, serverName string) (bool, error) {
	logger.Info("Checking Triton health", "server", serverName)

	// Call Triton /v2/health/ready endpoint
	serviceURL := fmt.Sprintf("http://%s-inference-service.default.svc.cluster.local:8000", serverName)
	healthURL := fmt.Sprintf("%s/v2/health/ready", serviceURL)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create health request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to call Triton health endpoint: %w", err)
	}
	defer resp.Body.Close()

	// Triton is healthy if status is 200
	healthy := resp.StatusCode == http.StatusOK
	logger.Info("Triton health status", "server", serverName, "healthy", healthy, "statusCode", resp.StatusCode)
	return healthy, nil
}

// Helper functions

// updateTritonModelConfig updates the model configuration for rolling out new models
// updateTritonModelConfig has been moved to the ConfigMap provider
// This backend no longer manages ConfigMaps directly

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
