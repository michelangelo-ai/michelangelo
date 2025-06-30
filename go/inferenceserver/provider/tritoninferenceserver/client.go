package tritoninferenceserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/provider/serving"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type TritonInferenceServerProvider struct {
	DynamicClient dynamic.Interface
}

var _ serving.Provider = &TritonInferenceServerProvider{}

// Triton gRPC service definitions for model management
type TritonModelRepositoryService interface {
	ModelRepositoryLoad(ctx context.Context, req *ModelRepositoryLoadRequest) (*ModelRepositoryLoadResponse, error)
	ModelRepositoryUnload(ctx context.Context, req *ModelRepositoryUnloadRequest) (*ModelRepositoryUnloadResponse, error)
	ModelRepositoryIndex(ctx context.Context, req *ModelRepositoryIndexRequest) (*ModelRepositoryIndexResponse, error)
}

type ModelRepositoryLoadRequest struct {
	ModelName string `json:"model_name"`
}

type ModelRepositoryLoadResponse struct {
}

type ModelRepositoryUnloadRequest struct {
	ModelName string `json:"model_name"`
}

type ModelRepositoryUnloadResponse struct {
}

type ModelRepositoryIndexRequest struct {
	Ready *bool `json:"ready,omitempty"`
}

type ModelRepositoryIndexResponse struct {
	Models []*ModelRepositoryModel `json:"models"`
}

type ModelRepositoryModel struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	State   string `json:"state"`
	Reason  string `json:"reason,omitempty"`
}

// Model deployment request structure
type ModelDeploymentRequest struct {
	ModelName string `json:"model_name"`
	ModelPath string `json:"model_path"` // S3 path
	Priority  int    `json:"priority,omitempty"`
}

// Rollout state for tracking model deployment progress
type RolloutState struct {
	Stage        string    `json:"stage"`        // "", "downloading", "loading", "ready", "failed"
	TargetModel  string    `json:"targetModel"`  // Model being deployed
	CurrentModel string    `json:"currentModel"` // Currently serving model (if any)
	ModelPath    string    `json:"modelPath"`    // S3 path for the target model
	JobName      string    `json:"jobName"`      // Download job name (if any)
	StartTime    time.Time `json:"startTime"`    // When deployment started
	LastUpdate   time.Time `json:"lastUpdate"`   // Last status update
	ErrorMessage string    `json:"errorMessage"` // Error details if failed
	Timeout      time.Time `json:"timeout"`      // When this stage times out
}

func (r TritonInferenceServerProvider) CreateInferenceServer(ctx context.Context, log logr.Logger, name, namespace string, configMapName string) error {
	log.Info("Creating empty Triton InferenceServer for lazy loading", "name", name, "namespace", namespace)

	// Create empty Triton Deployment (no models loaded initially)
	err := r.createEmptyTritonDeployment(ctx, log, name, namespace, configMapName)
	if err != nil {
		return err
	}

	// Create Service for Triton Server
	err = r.createTritonService(ctx, log, name, namespace)
	if err != nil {
		return err
	}

	// Create generic VirtualService for routing
	err = r.createGenericVirtualService(ctx, log, name, namespace)
	if err != nil {
		return err
	}

	log.Info("Empty Triton InferenceServer created successfully - ready for on-demand model loading")
	return nil
}

func (r TritonInferenceServerProvider) createEmptyTritonDeployment(ctx context.Context, log logr.Logger, name, namespace string, configMapName string) error {
	gvr := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app":                       name,
					"michelangelo.ai/inference": name,
					"michelangelo.ai/provider":  "triton",
					"michelangelo.ai/type":      "lazy-loading",
				},
				"annotations": map[string]interface{}{
					"michelangelo.ai/model-config": configMapName,
					"michelangelo.ai/created":      fmt.Sprintf("%d", time.Now().Unix()),
				},
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app": name,
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app":                       name,
							"michelangelo.ai/inference": name,
							"michelangelo.ai/provider":  "triton",
							"component":                 "predictor",
						},
					},
					"spec": map[string]interface{}{
						"containers": []map[string]interface{}{
							{
								"name":  "triton",
								"image": "nvcr.io/nvidia/tritonserver:23.12-py3",
								"args": []string{
									"tritonserver",
									"--model-store=/mnt/models",
									"--grpc-port=8001",
									"--http-port=8000",
									"--allow-grpc=true",
									"--allow-http=true",
									"--allow-metrics=true",
									"--metrics-port=8002",
									"--model-control-mode=explicit", // Changed to explicit for on-demand loading
									"--exit-on-error=false",         // Don't exit if no models initially
									"--log-error=true",
									"--log-warning=true",
									"--log-verbose=0",
								},
								"resources": map[string]interface{}{
									"limits":   map[string]interface{}{"cpu": "2", "memory": "4Gi"},
									"requests": map[string]interface{}{"cpu": "1", "memory": "4Gi"},
								},
								"ports": []map[string]interface{}{
									{"containerPort": 8000},
									{"containerPort": 8001},
									{"containerPort": 8002},
								},
								"volumeMounts": []map[string]interface{}{
									{
										"name":      "workdir",
										"mountPath": "/mnt/models",
									},
								},
								"readinessProbe": map[string]interface{}{
									"httpGet": map[string]interface{}{
										"path": "/v2/health/ready",
										"port": 8000,
									},
									"initialDelaySeconds": 30,
									"periodSeconds":       10,
								},
								"livenessProbe": map[string]interface{}{
									"httpGet": map[string]interface{}{
										"path": "/v2/health/live",
										"port": 8000,
									},
									"initialDelaySeconds": 60,
									"periodSeconds":       30,
								},
							},
						},
						"volumes": []map[string]interface{}{
							{
								"name": "workdir",
								"persistentVolumeClaim": map[string]interface{}{
									"claimName": "triton-model-storage",
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := r.DynamicClient.Resource(gvr).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create empty Triton Deployment: %w", err)
	}

	log.Info("Empty Triton Deployment created", "deployment", name)
	return nil
}

func (r TritonInferenceServerProvider) createTritonDeployment(ctx context.Context, log logr.Logger, name, namespace string, configMapName string) error {
	gvr := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      name, // Use the inference server name directly
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app":                       name,
					"michelangelo.ai/inference": name,
					"michelangelo.ai/provider":  "triton",
				},
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app": name, // Matches service selector
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app":                       name, // Must match selector
							"michelangelo.ai/inference": name,
							"michelangelo.ai/provider":  "triton",
							"component":                 "predictor",
						},
					},
					"spec": map[string]interface{}{
						"containers": []map[string]interface{}{
							{
								"name":  "triton",
								"image": "nvcr.io/nvidia/tritonserver:23.12-py3",
								"args": []string{
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
								"resources": map[string]interface{}{
									"limits":   map[string]interface{}{"cpu": "2", "memory": "4Gi"},
									"requests": map[string]interface{}{"cpu": "1", "memory": "4Gi"},
								},
								"ports": []map[string]interface{}{
									{"containerPort": 8000},
									{"containerPort": 8001},
									{"containerPort": 8002},
								},
								"volumeMounts": []map[string]interface{}{
									{
										"name":      "workdir",
										"mountPath": "/mnt/models",
									},
								},
							},
							{
								"name":    "model-sync",
								"image":   "amazon/aws-cli:2.15.50",
								"command": []string{"/bin/sh", "-c"},
								"args": []string{
									`yum install -y jq && \
CONFIG_FILE=/secret/localMinIO.json
echo "[CONFIG] Reading AWS configuration from $CONFIG_FILE"
ACCESS_KEY=$(jq -r '.access_key_id' $CONFIG_FILE)
SECRET_KEY=$(jq -r '.secret_access_key' $CONFIG_FILE)  
ENDPOINT=$(jq -r '.endpoint_url' $CONFIG_FILE)
REGION=$(jq -r '.region' $CONFIG_FILE)
echo "[CONFIG] Endpoint: $ENDPOINT, Region: $REGION"
echo "[AWS CONFIG] Setting aws_access_key_id"
aws configure set aws_access_key_id $ACCESS_KEY
echo "[AWS CONFIG] Setting aws_secret_access_key"
aws configure set aws_secret_access_key $SECRET_KEY
echo "[AWS CONFIG] Setting default.region to $REGION"
aws configure set default.region $REGION
echo "[AWS CONFIG] Setting default.s3.endpoint_url to $ENDPOINT"
aws configure set default.s3.endpoint_url $ENDPOINT
echo "[CONFIG] AWS configuration completed"

# Initialize state tracking files
mkdir -p /tmp/sync-state
LAST_CONFIG_HASH=""
LAST_SYNC_TIME=0

while true; do
  # Check if config has changed
  CURRENT_CONFIG_HASH=$(md5sum /config/model-list.json | cut -d' ' -f1)
  CONFIG_CHANGED=false
  
  if [ "$CURRENT_CONFIG_HASH" != "$LAST_CONFIG_HASH" ]; then
    echo "Model configuration changed (hash: $CURRENT_CONFIG_HASH)"
    CONFIG_CHANGED=true
    LAST_CONFIG_HASH="$CURRENT_CONFIG_HASH"
  fi
  
  # Only proceed if config changed or it's been more than 5 minutes since last sync
  CURRENT_TIME=$(date +%%s)
  TIME_SINCE_SYNC=$((CURRENT_TIME - LAST_SYNC_TIME))
  
  if [ "$CONFIG_CHANGED" = "true" ] || [ $TIME_SINCE_SYNC -gt 300 ]; then
    echo "Performing model sync (config_changed=$CONFIG_CHANGED, time_since_sync=${TIME_SINCE_SYNC}s)"
    
    cp /config/model-list.json /tmp/model-list.json
    current_models=$(jq -r '.[].name' /tmp/model-list.json | tr '\n' ' ')
    echo "Target models: $current_models"
    
    # Track which models need syncing
    MODELS_CHANGED=false
    
    # Sync each model and check for changes
    jq -c '.[]' /tmp/model-list.json | while read model; do
      name=$(echo "$model" | jq -r '.name')
      s3_path=$(echo "$model" | jq -r '.s3_path')
      
      # Check if model directory exists and get current state
      MODEL_STATE_FILE="/tmp/sync-state/$name.state"
      MODEL_DIR="/mnt/models/$name"
      
      echo "Checking model $name from $s3_path/$name/"
      
      # Check if this is a new model (directory doesn't exist)
      if [ ! -d "$MODEL_DIR" ]; then
        echo "New model detected: $name, performing initial sync"
        echo "[AWS SYNC] Command: aws s3 sync $s3_path/$name/ $MODEL_DIR/ --delete --exact-timestamps --endpoint-url $ENDPOINT"
        SYNC_START=$(date +%%s)
        aws s3 sync "$s3_path/$name/" "$MODEL_DIR/" --delete --exact-timestamps --endpoint-url "$ENDPOINT" 2>&1 | while IFS= read -r line; do
          echo "[AWS SYNC OUTPUT] $line"
        done
        SYNC_EXIT_CODE=${PIPESTATUS[0]}
        SYNC_END=$(date +%%s)
        SYNC_DURATION=$((SYNC_END - SYNC_START))
        echo "[AWS SYNC] Completed in ${SYNC_DURATION}s with exit code: $SYNC_EXIT_CODE"
        if [ $SYNC_EXIT_CODE -eq 0 ]; then
          echo "true" > "$MODEL_STATE_FILE"
          MODELS_CHANGED=true
        else
          echo "[AWS SYNC ERROR] Failed to sync new model $name"
        fi
      else
        # Existing model - check for changes with dry-run
        echo "[AWS DRYRUN] Command: aws s3 sync $s3_path/$name/ $MODEL_DIR/ --delete --exact-timestamps --endpoint-url $ENDPOINT --dryrun"
        DRYRUN_START=$(date +%%s)
        SYNC_OUTPUT=$(aws s3 sync "$s3_path/$name/" "$MODEL_DIR/" --delete --exact-timestamps --endpoint-url "$ENDPOINT" --dryrun 2>&1)
        DRYRUN_EXIT_CODE=$?
        DRYRUN_END=$(date +%%s)
        DRYRUN_DURATION=$((DRYRUN_END - DRYRUN_START))
        echo "[AWS DRYRUN] Completed in ${DRYRUN_DURATION}s with exit code: $DRYRUN_EXIT_CODE"
        echo "[AWS DRYRUN OUTPUT] $SYNC_OUTPUT"
        
        if [ $DRYRUN_EXIT_CODE -ne 0 ]; then
          echo "[AWS DRYRUN ERROR] Failed to check model $name for changes"
        elif echo "$SYNC_OUTPUT" | grep -q "(dryrun)"; then
          echo "Model $name has changes, performing update sync"
          echo "[AWS SYNC] Command: aws s3 sync $s3_path/$name/ $MODEL_DIR/ --delete --exact-timestamps --endpoint-url $ENDPOINT"
          SYNC_START=$(date +%%s)
          aws s3 sync "$s3_path/$name/" "$MODEL_DIR/" --delete --exact-timestamps --endpoint-url "$ENDPOINT" 2>&1 | while IFS= read -r line; do
            echo "[AWS SYNC OUTPUT] $line"
          done
          SYNC_EXIT_CODE=${PIPESTATUS[0]}
          SYNC_END=$(date +%%s)
          SYNC_DURATION=$((SYNC_END - SYNC_START))
          echo "[AWS SYNC] Completed in ${SYNC_DURATION}s with exit code: $SYNC_EXIT_CODE"
          if [ $SYNC_EXIT_CODE -eq 0 ]; then
            echo "true" > "$MODEL_STATE_FILE"
            MODELS_CHANGED=true
          else
            echo "[AWS SYNC ERROR] Failed to update model $name"
          fi
        else
          echo "Model $name is up to date, no changes needed"
          echo "false" > "$MODEL_STATE_FILE"
        fi
      fi
    done
    
    # Only cleanup old models if configuration changed
    if [ "$CONFIG_CHANGED" = "true" ]; then
      echo "Configuration changed, cleaning up old models"
      for dir in /mnt/models/*/; do
        if [ -d "$dir" ]; then
          dirname=$(basename "$dir")
          if ! echo " $current_models " | grep -q " $dirname "; then
            echo "Removing old model directory: $dirname"
            rm -rf "$dir"
            rm -f "/tmp/sync-state/$dirname.state"
          fi
        fi
      done
    fi
    
    LAST_SYNC_TIME=$CURRENT_TIME
    echo "Sync cycle completed"
  else
    echo "No changes detected, skipping sync (next check in $((300 - TIME_SINCE_SYNC))s)"
  fi

  # Sleep for 2 seconds between checks (much less frequent than before)
  sleep 2
done`,
								},
								"resources": map[string]interface{}{
									"requests": map[string]interface{}{"cpu": "100m", "memory": "100Mi"},
								},
								"volumeMounts": []map[string]interface{}{
									{
										"name":      "workdir",
										"mountPath": "/mnt/models",
									},
									{
										"name":      "model-config",
										"mountPath": "/config",
									},
									{
										"name":      "storage-secret",
										"mountPath": "/secret",
										"readOnly":  true,
									},
								},
							},
						},
						"volumes": []map[string]interface{}{
							{
								"name":     "workdir",
								"emptyDir": map[string]interface{}{},
							},
							{
								"name": "model-config",
								"configMap": map[string]interface{}{
									"name": configMapName,
								},
							},
							{
								"name": "storage-secret",
								"secret": map[string]interface{}{
									"secretName": "storage-config",
									"items": []map[string]interface{}{
										{
											"key":  "localMinIO",
											"path": "localMinIO.json",
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

	_, err := r.DynamicClient.Resource(gvr).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		log.Error(err, "Failed to create Triton Deployment")
		return err
	}

	log.Info("Triton Deployment created successfully")
	return nil
}

func (r TritonInferenceServerProvider) createTritonService(ctx context.Context, log logr.Logger, name, namespace string) error {
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "services",
	}

	// Create service following the tis-service.yaml pattern
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("%s-service", name), // Matches tis-service pattern: {name}-service
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app":                       name,
					"michelangelo.ai/inference": name,
					"michelangelo.ai/provider":  "triton",
				},
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"app": name, // Matches the deployment selector
				},
				"ports": []map[string]interface{}{
					{"port": 80, "targetPort": 8000, "name": "http"},      // HTTP on port 80 -> 8000
					{"port": 8001, "targetPort": 8001, "name": "grpc"},    // GRPC direct
					{"port": 8002, "targetPort": 8002, "name": "metrics"}, // Metrics direct
				},
				"type": "ClusterIP",
			},
		},
	}

	_, err := r.DynamicClient.Resource(gvr).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		log.Error(err, "Failed to create Triton Service")
		return err
	}

	log.Info("Triton Service created successfully")
	return nil
}

func (r TritonInferenceServerProvider) createGenericVirtualService(ctx context.Context, log logr.Logger, name, namespace string) error {
	gvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	// Create VirtualService with generic routing (non-production)
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1beta1",
			"kind":       "VirtualService",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("%s-virtualservice", name),
				"namespace": namespace,
				"labels": map[string]interface{}{
					"michelangelo.ai/inference": name,
					"michelangelo.ai/provider":  "istio",
				},
			},
			"spec": map[string]interface{}{
				"hosts":    []interface{}{"*"},
				"gateways": []interface{}{"ma-gateway"},
				"http": []interface{}{
					map[string]interface{}{
						"match": []interface{}{
							map[string]interface{}{
								"uri": map[string]interface{}{
									"prefix": fmt.Sprintf("/%s-endpoint/", name),
								},
							},
						},
						"rewrite": map[string]interface{}{
							"uri": "/",
						},
						"route": []interface{}{
							map[string]interface{}{
								"destination": map[string]interface{}{
									"host": fmt.Sprintf("%s-service.%s.svc.cluster.local", name, namespace),
									"port": map[string]interface{}{
										"number": int64(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := r.DynamicClient.Resource(gvr).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		log.Error(err, "Failed to create VirtualService")
		return err
	}

	log.Info("VirtualService created successfully")
	return nil
}

// UpdateInferenceServer implements state-driven reconciliation for lazy loading
func (r TritonInferenceServerProvider) UpdateInferenceServer(ctx context.Context, log logr.Logger, name, namespace string) error {
	log.Info("Starting lazy loading reconciliation for Triton InferenceServer", "name", name, "namespace", namespace)

	// Check current rollout state
	state, err := r.getRolloutState(ctx, log, name, namespace)
	if err != nil {
		log.Error(err, "Failed to get rollout state")
		return err
	}

	// If no active rollout, check if we need to start one
	if state == nil {
		return r.checkForNewDeploymentRequests(ctx, log, name, namespace)
	}

	// Handle current rollout state
	switch state.Stage {
	case "downloading":
		return r.checkDownloadProgress(ctx, log, name, namespace, state)
	case "loading":
		return r.checkModelLoadProgress(ctx, log, name, namespace, state)
	case "ready":
		log.Info("Model rollout completed successfully", "model", state.TargetModel)
		return r.clearRolloutState(ctx, log, name, namespace)
	case "failed":
		return r.handleFailedRollout(ctx, log, name, namespace, state)
	default:
		log.Info("Unknown rollout stage", "stage", state.Stage)
		return nil
	}
}

// DeployModel allows deployments to trigger model loading on inference servers
func (r TritonInferenceServerProvider) DeployModel(ctx context.Context, log logr.Logger, name, namespace string, request *ModelDeploymentRequest) error {
	log.Info("Deployment triggering model loading", "inferenceServer", name, "model", request.ModelName, "path", request.ModelPath)

	// Check if server exists and is ready
	ready, err := r.isServerReady(ctx, log, name, namespace)
	if err != nil {
		return fmt.Errorf("failed to check server readiness: %w", err)
	}
	if !ready {
		return fmt.Errorf("inference server %s is not ready for model deployment", name)
	}

	// Check for existing rollout
	existingState, err := r.getRolloutState(ctx, log, name, namespace)
	if err != nil {
		return fmt.Errorf("failed to check existing rollout state: %w", err)
	}
	if existingState != nil && existingState.Stage != "failed" {
		return fmt.Errorf("model rollout already in progress for %s: stage=%s, model=%s", name, existingState.Stage, existingState.TargetModel)
	}

	// Get current model to check if we need to switch
	currentModel, err := r.getCurrentlyLoadedModel(ctx, log, name, namespace)
	if err != nil {
		log.Error(err, "Failed to get currently loaded model")
		// Continue anyway - assume no model is loaded
	}

	if currentModel == request.ModelName {
		log.Info("Model already loaded", "model", request.ModelName)
		return nil
	}

	// Start model rollout
	return r.startModelRollout(ctx, log, name, namespace, currentModel, request.ModelName, request.ModelPath)
}

func (r TritonInferenceServerProvider) GetStatus(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer) error {
	name := inferenceServer.GetMetadata().GetName()
	namespace := inferenceServer.GetMetadata().GetNamespace()

	logger.Info("Getting Triton InferenceServer status", "name", name, "namespace", namespace)

	// Check if Triton deployment exists
	deploymentGVR := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}

	deployment, err := r.DynamicClient.Resource(deploymentGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			logger.Info("Triton deployment not found", "name", name)
			inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_INITIALIZED
			inferenceServer.Status.UpdateTime = fmt.Sprintf("%d", time.Now().Unix())
			return nil
		}
		logger.Error(err, "Failed to get Triton deployment")
		inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_FAILED
		inferenceServer.Status.UpdateTime = fmt.Sprintf("%d", time.Now().Unix())
		return err
	}

	// Check if replicas are ready
	readyReplicas, found, err := unstructured.NestedInt64(deployment.Object, "status", "readyReplicas")
	if err != nil || !found {
		readyReplicas = 0
	}

	replicas, found, err := unstructured.NestedInt64(deployment.Object, "status", "replicas")
	if err != nil || !found {
		replicas = 1 // Default expected replicas
	}

	// Update status based on deployment state
	if readyReplicas >= replicas && replicas > 0 {
		logger.Info("Triton deployment is running", "name", name, "readyReplicas", readyReplicas)
		inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_SERVING
	} else {
		logger.Info("Triton deployment is creating", "name", name, "readyReplicas", readyReplicas, "replicas", replicas)
		inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_CREATING
	}

	inferenceServer.Status.UpdateTime = fmt.Sprintf("%d", time.Now().Unix())
	return nil
}

func (r TritonInferenceServerProvider) DeleteInferenceServer(ctx context.Context, log logr.Logger, name, namespace string) error {
	// Delete VirtualService
	err := r.deleteVirtualService(ctx, log, name, namespace)
	if err != nil {
		return err
	}

	// Delete Triton Service
	err = r.deleteTritonService(ctx, log, name, namespace)
	if err != nil {
		return err
	}

	// Delete Triton Deployment
	err = r.deleteTritonDeployment(ctx, log, name, namespace)
	if err != nil {
		return err
	}

	log.Info("Triton InferenceServer deleted successfully")
	return nil
}

func (r TritonInferenceServerProvider) deleteTritonDeployment(ctx context.Context, log logr.Logger, name, namespace string) error {
	gvr := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}

	err := r.DynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		log.Error(err, "Failed to delete Triton Deployment")
		return err
	}

	log.Info("Triton Deployment deleted successfully")
	return nil
}

func (r TritonInferenceServerProvider) deleteTritonService(ctx context.Context, log logr.Logger, name, namespace string) error {
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "services",
	}

	serviceName := fmt.Sprintf("%s-service", name)
	err := r.DynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
	if err != nil {
		log.Error(err, "Failed to delete Triton Service")
		return err
	}

	log.Info("Triton Service deleted successfully")
	return nil
}

func (r TritonInferenceServerProvider) deleteVirtualService(ctx context.Context, log logr.Logger, name, namespace string) error {
	gvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	virtualServiceName := fmt.Sprintf("%s-virtualservice", name)
	err := r.DynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, virtualServiceName, metav1.DeleteOptions{})
	if err != nil {
		log.Error(err, "Failed to delete VirtualService")
		return err
	}

	log.Info("VirtualService deleted successfully")
	return nil
}

func (r TritonInferenceServerProvider) getCurrentModel(ctx context.Context, log logr.Logger, name, namespace string) (string, error) {
	// Check VirtualService to see which model is currently being served
	vsGVR := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	vsName := fmt.Sprintf("%s-virtualservice", name)
	vs, err := r.DynamicClient.Resource(vsGVR).Namespace(namespace).Get(ctx, vsName, metav1.GetOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			log.Info("VirtualService not found, no current model")
			return "", nil
		}
		return "", err
	}

	// Extract current destination from VirtualService
	routes, found, err := unstructured.NestedSlice(vs.Object, "spec", "http", "0", "route")
	if err != nil || !found || len(routes) == 0 {
		return "", nil
	}

	route := routes[0].(map[string]interface{})
	destination, found, err := unstructured.NestedMap(route, "destination")
	if err != nil || !found {
		return "", nil
	}

	host, found, err := unstructured.NestedString(destination, "host")
	if err != nil || !found {
		return "", nil
	}

	// Extract model name from host (format: {name}-{model}-service.{namespace}.svc.cluster.local)
	if strings.Contains(host, fmt.Sprintf(".%s.svc.cluster.local", namespace)) {
		serviceName := strings.Split(host, ".")[0]
		// Remove -service suffix
		if strings.HasSuffix(serviceName, "-service") {
			serviceName = strings.TrimSuffix(serviceName, "-service")
		}
		if strings.HasPrefix(serviceName, name+"-") {
			modelName := strings.TrimPrefix(serviceName, name+"-")
			return modelName, nil
		}
	}

	return "", nil
}

func (r TritonInferenceServerProvider) deployNewTritonInstance(ctx context.Context, log logr.Logger, name, namespace, modelName, modelURI string) error {
	log.Info("Deploying new Triton instance", "model", modelName)

	instanceName := fmt.Sprintf("%s-%s", name, modelName)

	// Create Deployment for new Triton instance without model-sync sidecar
	err := r.createTritonDeploymentForModel(ctx, log, instanceName, namespace, modelName, modelURI)
	if err != nil {
		return err
	}

	// Create Service for new Triton instance
	err = r.createTritonServiceForModel(ctx, log, instanceName, namespace)
	if err != nil {
		return err
	}

	log.Info("New Triton instance created", "instance", instanceName)
	return nil
}

func (r TritonInferenceServerProvider) createTritonDeploymentForModel(ctx context.Context, log logr.Logger, instanceName, namespace, modelName, modelURI string) error {
	gvr := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      instanceName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app":                       instanceName,
					"michelangelo.ai/inference": strings.Split(instanceName, "-")[0], // original inference server name
					"michelangelo.ai/provider":  "triton",
					"michelangelo.ai/model":     modelName,
				},
				"annotations": map[string]interface{}{
					"michelangelo.ai/model-name": modelName,
					"michelangelo.ai/model-uri":  modelURI,
					"michelangelo.ai/created":    fmt.Sprintf("%d", time.Now().Unix()),
				},
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app": instanceName,
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app":                       instanceName,
							"michelangelo.ai/inference": strings.Split(instanceName, "-")[0],
							"michelangelo.ai/provider":  "triton",
							"michelangelo.ai/model":     modelName,
							"component":                 "predictor",
						},
					},
					"spec": map[string]interface{}{
						"containers": []map[string]interface{}{
							{
								"name":  "triton",
								"image": "nvcr.io/nvidia/tritonserver:23.12-py3",
								"args": []string{
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
								"resources": map[string]interface{}{
									"limits":   map[string]interface{}{"cpu": "2", "memory": "4Gi"},
									"requests": map[string]interface{}{"cpu": "1", "memory": "4Gi"},
								},
								"ports": []map[string]interface{}{
									{"containerPort": 8000},
									{"containerPort": 8001},
									{"containerPort": 8002},
								},
								"volumeMounts": []map[string]interface{}{
									{
										"name":      "workdir",
										"mountPath": "/mnt/models",
									},
								},
							},
							{
								"name":    "model-sync",
								"image":   "amazon/aws-cli:2.15.50",
								"command": []string{"/bin/sh", "-c"},
								"args": []string{
									fmt.Sprintf(`yum install -y jq && \
CONFIG_FILE=/secret/localMinIO.json
echo "[CONFIG] Reading AWS configuration from $CONFIG_FILE"
ACCESS_KEY=$(jq -r '.access_key_id' $CONFIG_FILE)
SECRET_KEY=$(jq -r '.secret_access_key' $CONFIG_FILE)  
ENDPOINT=$(jq -r '.endpoint_url' $CONFIG_FILE)
REGION=$(jq -r '.region' $CONFIG_FILE)
echo "[CONFIG] Endpoint: $ENDPOINT, Region: $REGION"
echo "[AWS CONFIG] Setting aws_access_key_id"
aws configure set aws_access_key_id $ACCESS_KEY
echo "[AWS CONFIG] Setting aws_secret_access_key"
aws configure set aws_secret_access_key $SECRET_KEY
echo "[AWS CONFIG] Setting default.region to $REGION"
aws configure set default.region $REGION
echo "[AWS CONFIG] Setting default.s3.endpoint_url to $ENDPOINT"
aws configure set default.s3.endpoint_url $ENDPOINT
echo "[CONFIG] AWS configuration completed"

# This instance is dedicated to model: %s
TARGET_MODEL="%s"
TARGET_MODEL_URI=""

# Initialize state tracking files
mkdir -p /tmp/sync-state
LAST_CONFIG_HASH=""
LAST_SYNC_TIME=0

while true; do
  # Check if config has changed
  CURRENT_CONFIG_HASH=$(md5sum /config/model-list.json | cut -d' ' -f1)
  CONFIG_CHANGED=false
  
  if [ "$CURRENT_CONFIG_HASH" != "$LAST_CONFIG_HASH" ]; then
    echo "Model configuration changed (hash: $CURRENT_CONFIG_HASH)"
    CONFIG_CHANGED=true
    LAST_CONFIG_HASH="$CURRENT_CONFIG_HASH"
  fi
  
  # Only proceed if config changed or it's been more than 5 minutes since last sync
  CURRENT_TIME=$(date +%%s)
  TIME_SINCE_SYNC=$((CURRENT_TIME - LAST_SYNC_TIME))
  
  if [ "$CONFIG_CHANGED" = "true" ] || [ $TIME_SINCE_SYNC -gt 300 ]; then
    echo "Performing model sync for dedicated model: $TARGET_MODEL"
    
    # Read the model list and find our target model
    cp /config/model-list.json /tmp/model-list.json
    TARGET_MODEL_FOUND=false
    
    # Parse JSON to find our specific model
    jq -c '.[]' /tmp/model-list.json | while read model; do
      name=$(echo "$model" | jq -r '.name')
      s3_path=$(echo "$model" | jq -r '.s3_path')
      
      if [ "$name" = "$TARGET_MODEL" ]; then
        echo "Found target model in config: $TARGET_MODEL from $s3_path"
        TARGET_MODEL_FOUND=true
        
        # Check if model directory exists and get current state
        MODEL_STATE_FILE="/tmp/sync-state/$name.state"
        MODEL_DIR="/mnt/models/$name"
        
        echo "Syncing model $name from $s3_path/$name/"
        
        # Check if this is a new model (directory doesn't exist)
        if [ ! -d "$MODEL_DIR" ]; then
          echo "New model detected: $name, performing initial sync"
          echo "[AWS SYNC] Command: aws s3 sync $s3_path/$name/ $MODEL_DIR/ --delete --exact-timestamps --endpoint-url $ENDPOINT"
          SYNC_START=$(date +%%s)
          aws s3 sync "$s3_path/$name/" "$MODEL_DIR/" --delete --exact-timestamps --endpoint-url "$ENDPOINT" 2>&1 | while IFS= read -r line; do
            echo "[AWS SYNC OUTPUT] $line"
          done
          SYNC_EXIT_CODE=${PIPESTATUS[0]}
          SYNC_END=$(date +%%s)
          SYNC_DURATION=$((SYNC_END - SYNC_START))
          echo "[AWS SYNC] Completed in ${SYNC_DURATION}s with exit code: $SYNC_EXIT_CODE"
          if [ $SYNC_EXIT_CODE -eq 0 ]; then
            echo "true" > "$MODEL_STATE_FILE"
          else
            echo "[AWS SYNC ERROR] Failed to sync new model $name"
          fi
        else
          # Existing model - check for changes with dry-run
          echo "[AWS DRYRUN] Command: aws s3 sync $s3_path/$name/ $MODEL_DIR/ --delete --exact-timestamps --endpoint-url $ENDPOINT --dryrun"
          DRYRUN_START=$(date +%%s)
          SYNC_OUTPUT=$(aws s3 sync "$s3_path/$name/" "$MODEL_DIR/" --delete --exact-timestamps --endpoint-url "$ENDPOINT" --dryrun 2>&1)
          DRYRUN_EXIT_CODE=$?
          DRYRUN_END=$(date +%%s)
          DRYRUN_DURATION=$((DRYRUN_END - DRYRUN_START))
          echo "[AWS DRYRUN] Completed in ${DRYRUN_DURATION}s with exit code: $DRYRUN_EXIT_CODE"
          echo "[AWS DRYRUN OUTPUT] $SYNC_OUTPUT"
          
          if [ $DRYRUN_EXIT_CODE -ne 0 ]; then
            echo "[AWS DRYRUN ERROR] Failed to check model $name for changes"
          elif echo "$SYNC_OUTPUT" | grep -q "(dryrun)"; then
            echo "Model $name has changes, performing update sync"
            echo "[AWS SYNC] Command: aws s3 sync $s3_path/$name/ $MODEL_DIR/ --delete --exact-timestamps --endpoint-url $ENDPOINT"
            SYNC_START=$(date +%%s)
            aws s3 sync "$s3_path/$name/" "$MODEL_DIR/" --delete --exact-timestamps --endpoint-url "$ENDPOINT" 2>&1 | while IFS= read -r line; do
              echo "[AWS SYNC OUTPUT] $line"
            done
            SYNC_EXIT_CODE=${PIPESTATUS[0]}
            SYNC_END=$(date +%%s)
            SYNC_DURATION=$((SYNC_END - SYNC_START))
            echo "[AWS SYNC] Completed in ${SYNC_DURATION}s with exit code: $SYNC_EXIT_CODE"
            if [ $SYNC_EXIT_CODE -eq 0 ]; then
              echo "true" > "$MODEL_STATE_FILE"
            else
              echo "[AWS SYNC ERROR] Failed to update model $name"
            fi
          else
            echo "Model $name is up to date, no changes needed"
            echo "false" > "$MODEL_STATE_FILE"
          fi
        fi
        break
      fi
    done
    
    # Clean up any models that are not our target model
    for dir in /mnt/models/*/; do
      if [ -d "$dir" ]; then
        dirname=$(basename "$dir")
        if [ "$dirname" != "$TARGET_MODEL" ]; then
          echo "Removing non-target model directory: $dirname (this instance serves: $TARGET_MODEL)"
          rm -rf "$dir"
          rm -f "/tmp/sync-state/$dirname.state"
        fi
      fi
    done
    
    LAST_SYNC_TIME=$CURRENT_TIME
    echo "Sync cycle completed for model: $TARGET_MODEL"
  else
    echo "No changes detected for $TARGET_MODEL, skipping sync (next check in $((300 - TIME_SINCE_SYNC))s)"
  fi

  # Sleep for 10 seconds between checks
  sleep 10
done`, modelName, modelName),
								},
								"resources": map[string]interface{}{
									"requests": map[string]interface{}{"cpu": "100m", "memory": "100Mi"},
								},
								"volumeMounts": []map[string]interface{}{
									{
										"name":      "workdir",
										"mountPath": "/mnt/models",
									},
									{
										"name":      "model-config",
										"mountPath": "/config",
									},
									{
										"name":      "storage-secret",
										"mountPath": "/secret",
										"readOnly":  true,
									},
								},
							},
						},
						"volumes": []map[string]interface{}{
							{
								"name":     "workdir",
								"emptyDir": map[string]interface{}{},
							},
							{
								"name": "model-config",
								"configMap": map[string]interface{}{
									"name": fmt.Sprintf("%s-model-config", strings.Split(instanceName, "-")[0]),
								},
							},
							{
								"name": "storage-secret",
								"secret": map[string]interface{}{
									"secretName": "storage-config",
									"items": []map[string]interface{}{
										{
											"key":  "localMinIO",
											"path": "localMinIO.json",
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

	_, err := r.DynamicClient.Resource(gvr).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Triton Deployment for new model: %w", err)
	}

	log.Info("Triton Deployment created for model", "deployment", instanceName, "model", modelName)
	return nil
}

func (r TritonInferenceServerProvider) createTritonServiceForModel(ctx context.Context, log logr.Logger, instanceName, namespace string) error {
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "services",
	}

	serviceName := fmt.Sprintf("%s-service", instanceName)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      serviceName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app":                       instanceName,
					"michelangelo.ai/inference": strings.Split(instanceName, "-")[0],
					"michelangelo.ai/provider":  "triton",
				},
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"app": instanceName,
				},
				"ports": []map[string]interface{}{
					{"port": 80, "targetPort": 8000, "name": "http"},
					{"port": 8001, "targetPort": 8001, "name": "grpc"},
					{"port": 8002, "targetPort": 8002, "name": "metrics"},
				},
				"type": "ClusterIP",
			},
		},
	}

	_, err := r.DynamicClient.Resource(gvr).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Triton Service for new model: %w", err)
	}

	log.Info("Triton Service created for model", "service", serviceName)
	return nil
}

func (r TritonInferenceServerProvider) waitForTritonReady(ctx context.Context, log logr.Logger, name, namespace, modelName string) error {
	log.Info("Waiting for new Triton instance to be ready", "model", modelName)

	instanceName := fmt.Sprintf("%s-%s", name, modelName)
	serviceName := fmt.Sprintf("%s-service", instanceName)

	// Wait for Deployment to be ready and model endpoint to respond
	timeout := time.After(10 * time.Minute) // 10 minute timeout
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for Triton instance to be ready: %s", modelName)
		case <-ticker.C:
			// Check deployment readiness
			ready, err := r.checkTritonDeploymentReady(ctx, log, instanceName, namespace)
			if err != nil {
				log.Error(err, "Error checking Triton deployment readiness")
				continue
			}
			if ready {
				// Additional check: ping the model endpoint
				modelReady, err := r.pingTritonModelEndpoint(ctx, log, serviceName, namespace, modelName)
				if err != nil {
					log.Error(err, "Error pinging Triton model endpoint")
					continue
				}
				if modelReady {
					log.Info("Triton instance is ready and serving model", "model", modelName)
					return nil
				}
			}
			log.Info("Still waiting for Triton instance to be ready", "model", modelName)
		}
	}
}

func (r TritonInferenceServerProvider) checkTritonDeploymentReady(ctx context.Context, log logr.Logger, instanceName, namespace string) (bool, error) {
	deploymentGVR := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}

	deployment, err := r.DynamicClient.Resource(deploymentGVR).Namespace(namespace).Get(ctx, instanceName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	readyReplicas, found, err := unstructured.NestedInt64(deployment.Object, "status", "readyReplicas")
	if err != nil || !found {
		readyReplicas = 0
	}

	replicas, found, err := unstructured.NestedInt64(deployment.Object, "status", "replicas")
	if err != nil || !found {
		replicas = 1
	}

	return readyReplicas >= replicas && replicas > 0, nil
}

func (r TritonInferenceServerProvider) pingTritonModelEndpoint(ctx context.Context, log logr.Logger, serviceName, namespace, modelName string) (bool, error) {
	// Construct the Triton model endpoint URL
	// Format: http://{serviceName}.{namespace}.svc.cluster.local/v2/models/{modelName}
	url := fmt.Sprintf("http://%s.%s.svc.cluster.local/v2/models/%s", serviceName, namespace, modelName)

	log.Info("Pinging Triton model endpoint", "url", url)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return false, fmt.Errorf("failed to ping Triton model endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Info("Triton model endpoint is healthy", "model", modelName, "status", resp.StatusCode)
		return true, nil
	}

	log.Info("Triton model endpoint not ready yet", "model", modelName, "status", resp.StatusCode)
	return false, nil
}

func (r TritonInferenceServerProvider) switchTrafficToNewTriton(ctx context.Context, log logr.Logger, name, namespace, modelName string) error {
	log.Info("Switching traffic to new Triton instance", "model", modelName)

	instanceName := fmt.Sprintf("%s-%s", name, modelName)
	serviceName := fmt.Sprintf("%s-service", instanceName)

	// Update VirtualService to route to new Triton service
	vsGVR := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	vsName := fmt.Sprintf("%s-virtualservice", name)
	vs, err := r.DynamicClient.Resource(vsGVR).Namespace(namespace).Get(ctx, vsName, metav1.GetOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			// Create new VirtualService
			return r.createVirtualServiceForTriton(ctx, log, name, namespace, serviceName)
		}
		return fmt.Errorf("failed to get VirtualService: %w", err)
	}

	// Update existing VirtualService to point to new Triton service
	newDestination := map[string]interface{}{
		"host": fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, namespace),
		"port": map[string]interface{}{
			"number": int64(80),
		},
	}

	err = unstructured.SetNestedField(vs.Object, newDestination, "spec", "http", "0", "route", "0", "destination")
	if err != nil {
		return fmt.Errorf("failed to update VirtualService destination: %w", err)
	}

	// Add metadata about the switch
	metadata := vs.Object["metadata"].(map[string]interface{})
	if metadata["annotations"] == nil {
		metadata["annotations"] = make(map[string]interface{})
	}
	annotations := metadata["annotations"].(map[string]interface{})
	annotations["michelangelo.ai/current-model"] = modelName
	annotations["michelangelo.ai/last-updated"] = fmt.Sprintf("%d", time.Now().Unix())

	_, err = r.DynamicClient.Resource(vsGVR).Namespace(namespace).Update(ctx, vs, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update VirtualService: %w", err)
	}

	log.Info("Traffic switched to new Triton instance successfully", "model", modelName, "service", serviceName)
	return nil
}

func (r TritonInferenceServerProvider) createVirtualServiceForTriton(ctx context.Context, log logr.Logger, name, namespace, serviceName string) error {
	vsGVR := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1beta1",
			"kind":       "VirtualService",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("%s-virtualservice", name),
				"namespace": namespace,
				"labels": map[string]interface{}{
					"michelangelo.ai/inference": name,
					"michelangelo.ai/provider":  "triton",
				},
			},
			"spec": map[string]interface{}{
				"hosts":    []interface{}{"*"},
				"gateways": []interface{}{"ma-gateway"},
				"http": []interface{}{
					map[string]interface{}{
						"match": []interface{}{
							map[string]interface{}{
								"uri": map[string]interface{}{
									"prefix": fmt.Sprintf("/%s-endpoint/", name),
								},
							},
						},
						"rewrite": map[string]interface{}{
							"uri": "/",
						},
						"route": []interface{}{
							map[string]interface{}{
								"destination": map[string]interface{}{
									"host": fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, namespace),
									"port": map[string]interface{}{
										"number": int64(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := r.DynamicClient.Resource(vsGVR).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create VirtualService for Triton: %w", err)
	}

	log.Info("VirtualService created for new Triton instance", "service", serviceName)
	return nil
}

func (r TritonInferenceServerProvider) cleanupTritonInstance(ctx context.Context, log logr.Logger, name, namespace, modelName string) error {
	log.Info("Cleaning up old Triton instance", "model", modelName)

	instanceName := fmt.Sprintf("%s-%s", name, modelName)
	serviceName := fmt.Sprintf("%s-service", instanceName)

	// Delete Service
	serviceGVR := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "services",
	}

	err := r.DynamicClient.Resource(serviceGVR).Namespace(namespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
	if err != nil && !utils.IsNotFoundError(err) {
		log.Error(err, "Failed to delete Triton Service during cleanup", "service", serviceName)
	}

	// Delete Deployment
	deploymentGVR := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}

	err = r.DynamicClient.Resource(deploymentGVR).Namespace(namespace).Delete(ctx, instanceName, metav1.DeleteOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			log.Info("Triton instance already cleaned up", "model", modelName)
			return nil
		}
		return fmt.Errorf("failed to delete old Triton instance: %w", err)
	}

	log.Info("Old Triton instance cleaned up successfully", "model", modelName)
	return nil
}

// === Lazy Loading Helper Functions ===

// getRolloutState retrieves the current rollout state from ConfigMap
func (r TritonInferenceServerProvider) getRolloutState(ctx context.Context, log logr.Logger, name, namespace string) (*RolloutState, error) {
	stateConfigMapName := fmt.Sprintf("%s-rollout-state", name)
	configMapGVR := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	configMap, err := r.DynamicClient.Resource(configMapGVR).Namespace(namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			return nil, nil // No active rollout
		}
		return nil, fmt.Errorf("failed to get rollout state: %w", err)
	}

	stateData, found, err := unstructured.NestedString(configMap.Object, "data", "rollout-state.json")
	if err != nil || !found {
		return nil, fmt.Errorf("rollout state data not found in ConfigMap")
	}

	var state RolloutState
	if err := json.Unmarshal([]byte(stateData), &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal rollout state: %w", err)
	}

	// Check for timeout
	if time.Now().After(state.Timeout) {
		log.Info("Rollout stage timed out", "stage", state.Stage, "model", state.TargetModel)
		state.Stage = "failed"
		state.ErrorMessage = fmt.Sprintf("Stage '%s' timed out", state.Stage)

		// Save the failed state
		r.saveRolloutState(ctx, log, name, namespace, &state)
	}

	return &state, nil
}

// saveRolloutState saves rollout state to ConfigMap
func (r TritonInferenceServerProvider) saveRolloutState(ctx context.Context, log logr.Logger, name, namespace string, state *RolloutState) error {
	state.LastUpdate = time.Now()

	stateConfigMapName := fmt.Sprintf("%s-rollout-state", name)
	configMapGVR := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	stateData, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal rollout state: %w", err)
	}

	configMap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      stateConfigMapName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"michelangelo.ai/inference": name,
					"michelangelo.ai/component": "rollout-state",
				},
			},
			"data": map[string]interface{}{
				"rollout-state.json": string(stateData),
			},
		},
	}

	// Try to update first, create if not exists
	_, err = r.DynamicClient.Resource(configMapGVR).Namespace(namespace).Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			_, err = r.DynamicClient.Resource(configMapGVR).Namespace(namespace).Create(ctx, configMap, metav1.CreateOptions{})
		}
	}

	if err != nil {
		return fmt.Errorf("failed to save rollout state: %w", err)
	}

	log.Info("Rollout state saved", "stage", state.Stage, "target", state.TargetModel)
	return nil
}

// clearRolloutState removes rollout state ConfigMap
func (r TritonInferenceServerProvider) clearRolloutState(ctx context.Context, log logr.Logger, name, namespace string) error {
	stateConfigMapName := fmt.Sprintf("%s-rollout-state", name)
	configMapGVR := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	err := r.DynamicClient.Resource(configMapGVR).Namespace(namespace).Delete(ctx, stateConfigMapName, metav1.DeleteOptions{})
	if err != nil && !utils.IsNotFoundError(err) {
		return fmt.Errorf("failed to clear rollout state: %w", err)
	}

	log.Info("Rollout state cleared")
	return nil
}

// isServerReady checks if the inference server is ready to accept model deployments
func (r TritonInferenceServerProvider) isServerReady(ctx context.Context, log logr.Logger, name, namespace string) (bool, error) {
	deploymentGVR := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}

	deployment, err := r.DynamicClient.Resource(deploymentGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			return false, fmt.Errorf("inference server deployment not found")
		}
		return false, err
	}

	readyReplicas, found, err := unstructured.NestedInt64(deployment.Object, "status", "readyReplicas")
	if err != nil || !found {
		readyReplicas = 0
	}

	replicas, found, err := unstructured.NestedInt64(deployment.Object, "status", "replicas")
	if err != nil || !found {
		replicas = 1
	}

	return readyReplicas >= replicas && replicas > 0, nil
}

// getCurrentlyLoadedModel gets the currently loaded model via Triton HTTP API
func (r TritonInferenceServerProvider) getCurrentlyLoadedModel(ctx context.Context, log logr.Logger, name, namespace string) (string, error) {
	serviceName := fmt.Sprintf("%s-service", name)
	url := fmt.Sprintf("http://%s.%s.svc.cluster.local/v2/models", serviceName, namespace)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to query loaded models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("triton server returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var models []map[string]interface{}
	if err := json.Unmarshal(body, &models); err != nil {
		return "", fmt.Errorf("failed to parse models response: %w", err)
	}

	// Return the first ready model (assuming single model per server for now)
	for _, model := range models {
		if state, ok := model["state"].(string); ok && state == "READY" {
			if name, ok := model["name"].(string); ok {
				return name, nil
			}
		}
	}

	return "", nil // No models loaded
}

// checkForNewDeploymentRequests checks if there are pending deployment requests
func (r TritonInferenceServerProvider) checkForNewDeploymentRequests(ctx context.Context, log logr.Logger, name, namespace string) error {
	// This would typically check for deployment annotations or separate deployment requests
	// For now, we'll just log that no active rollout is happening
	log.Info("No active rollout and no pending deployment requests", "inferenceServer", name)
	return nil
}

// startModelRollout begins the model deployment process
func (r TritonInferenceServerProvider) startModelRollout(ctx context.Context, log logr.Logger, name, namespace, currentModel, targetModel, modelPath string) error {
	log.Info("Starting model rollout", "from", currentModel, "to", targetModel)

	state := &RolloutState{
		Stage:        "downloading",
		TargetModel:  targetModel,
		CurrentModel: currentModel,
		ModelPath:    modelPath,
		StartTime:    time.Now(),
		Timeout:      time.Now().Add(10 * time.Minute), // 10 minute timeout for download
	}

	// Start download job
	jobName := fmt.Sprintf("model-download-%s-%d", targetModel, time.Now().Unix())
	state.JobName = jobName

	err := r.createModelDownloadJob(ctx, log, jobName, namespace, targetModel, modelPath)
	if err != nil {
		state.Stage = "failed"
		state.ErrorMessage = fmt.Sprintf("Failed to create download job: %v", err)
		r.saveRolloutState(ctx, log, name, namespace, state)
		return err
	}

	return r.saveRolloutState(ctx, log, name, namespace, state)
}

// checkDownloadProgress monitors model download progress
func (r TritonInferenceServerProvider) checkDownloadProgress(ctx context.Context, log logr.Logger, name, namespace string, state *RolloutState) error {
	log.Info("Checking download progress", "job", state.JobName)

	jobComplete, jobFailed, err := r.checkJobStatus(ctx, log, state.JobName, namespace)
	if err != nil {
		log.Error(err, "Error checking job status", "job", state.JobName)
		return nil // Don't fail, will retry next reconciliation
	}

	if jobFailed {
		state.Stage = "failed"
		state.ErrorMessage = "Download job failed"
		return r.saveRolloutState(ctx, log, name, namespace, state)
	}

	if jobComplete {
		log.Info("Download completed, moving to loading stage", "model", state.TargetModel)
		state.Stage = "loading"
		state.Timeout = time.Now().Add(5 * time.Minute) // 5 minute timeout for loading
		return r.saveRolloutState(ctx, log, name, namespace, state)
	}

	log.Info("Download still in progress", "job", state.JobName)
	return nil // Will check again in next reconciliation
}

// checkModelLoadProgress monitors model loading via Triton API
func (r TritonInferenceServerProvider) checkModelLoadProgress(ctx context.Context, log logr.Logger, name, namespace string, state *RolloutState) error {
	log.Info("Checking model load progress", "model", state.TargetModel)

	serviceName := fmt.Sprintf("%s-service", name)

	// Try to load the model
	err := r.loadModelViaHTTP(ctx, log, serviceName, namespace, state.TargetModel)
	if err != nil {
		log.Error(err, "Failed to load model", "model", state.TargetModel)
		return nil // Will retry next reconciliation
	}

	// Check if model is ready
	ready, err := r.checkModelReadyViaHTTP(ctx, log, serviceName, namespace, state.TargetModel)
	if err != nil {
		log.Error(err, "Error checking if model is ready", "model", state.TargetModel)
		return nil // Will retry next reconciliation
	}

	if ready {
		log.Info("Model is ready, completing rollout", "model", state.TargetModel)

		// Unload old model if it exists
		if state.CurrentModel != "" && state.CurrentModel != state.TargetModel {
			err := r.unloadModelViaHTTP(ctx, log, serviceName, namespace, state.CurrentModel)
			if err != nil {
				log.Error(err, "Failed to unload old model, continuing anyway", "model", state.CurrentModel)
			}
		}

		state.Stage = "ready"
		return r.saveRolloutState(ctx, log, name, namespace, state)
	}

	log.Info("Model not ready yet", "model", state.TargetModel)
	return nil // Will check again in next reconciliation
}

// handleFailedRollout handles rollout failures
func (r TritonInferenceServerProvider) handleFailedRollout(ctx context.Context, log logr.Logger, name, namespace string, state *RolloutState) error {
	log.Error(fmt.Errorf("rollout failed: %s", state.ErrorMessage), "Handling failed rollout", "model", state.TargetModel)

	// For now, just clear the state to allow retry
	// In a real implementation, you might want to implement retry logic or cleanup
	log.Info("Clearing failed rollout state to allow retry")
	return r.clearRolloutState(ctx, log, name, namespace)
}

// === HTTP Model Management Functions ===

// loadModelViaHTTP loads a model via Triton HTTP API
func (r TritonInferenceServerProvider) loadModelViaHTTP(ctx context.Context, log logr.Logger, serviceName, namespace, modelName string) error {
	url := fmt.Sprintf("http://%s.%s.svc.cluster.local/v2/repository/models/%s/load", serviceName, namespace, modelName)

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create load request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to load model %s: %w", modelName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to load model %s: status %d, body: %s", modelName, resp.StatusCode, string(body))
	}

	log.Info("Model load request sent", "model", modelName)
	return nil
}

// unloadModelViaHTTP unloads a model via Triton HTTP API
func (r TritonInferenceServerProvider) unloadModelViaHTTP(ctx context.Context, log logr.Logger, serviceName, namespace, modelName string) error {
	url := fmt.Sprintf("http://%s.%s.svc.cluster.local/v2/repository/models/%s/unload", serviceName, namespace, modelName)

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create unload request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to unload model %s: %w", modelName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to unload model %s: status %d, body: %s", modelName, resp.StatusCode, string(body))
	}

	log.Info("Model unload request sent", "model", modelName)
	return nil
}

// checkModelReadyViaHTTP checks if a model is ready via Triton HTTP API
func (r TritonInferenceServerProvider) checkModelReadyViaHTTP(ctx context.Context, log logr.Logger, serviceName, namespace, modelName string) (bool, error) {
	url := fmt.Sprintf("http://%s.%s.svc.cluster.local/v2/models/%s/ready", serviceName, namespace, modelName)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create ready check request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to check model ready %s: %w", modelName, err)
	}
	defer resp.Body.Close()

	// Triton returns 200 if model is ready, 400 if not ready
	if resp.StatusCode == http.StatusOK {
		log.Info("Model is ready", "model", modelName)
		return true, nil
	} else if resp.StatusCode == http.StatusBadRequest {
		log.Info("Model is not ready yet", "model", modelName)
		return false, nil
	} else {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("unexpected status checking model ready %s: status %d, body: %s", modelName, resp.StatusCode, string(body))
	}
}

// createModelDownloadJob creates a Kubernetes Job to download model from S3
func (r TritonInferenceServerProvider) createModelDownloadJob(ctx context.Context, log logr.Logger, jobName, namespace, modelName, modelPath string) error {
	jobGVR := schema.GroupVersionResource{
		Group:    "batch",
		Version:  "v1",
		Resource: "jobs",
	}

	job := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name":      jobName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app":                       "model-downloader",
					"michelangelo.ai/model":     modelName,
					"michelangelo.ai/operation": "download",
				},
			},
			"spec": map[string]interface{}{
				"ttlSecondsAfterFinished": 300, // Clean up after 5 minutes
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"restartPolicy": "Never",
						"containers": []map[string]interface{}{
							{
								"name":    "model-downloader",
								"image":   "amazon/aws-cli:2.15.50",
								"command": []string{"/bin/sh", "-c"},
								"args": []string{
									fmt.Sprintf(`
# Install required tools
yum install -y jq

# Read AWS configuration
CONFIG_FILE=/secret/localMinIO.json
ACCESS_KEY=$(jq -r '.access_key_id' $CONFIG_FILE)
SECRET_KEY=$(jq -r '.secret_access_key' $CONFIG_FILE)
ENDPOINT=$(jq -r '.endpoint_url' $CONFIG_FILE)
REGION=$(jq -r '.region' $CONFIG_FILE)

# Configure AWS CLI
aws configure set aws_access_key_id $ACCESS_KEY
aws configure set aws_secret_access_key $SECRET_KEY
aws configure set default.region $REGION
aws configure set default.s3.endpoint_url $ENDPOINT

# Download model
MODEL_DIR="/shared/models/%s"
echo "Downloading model %s from %s to $MODEL_DIR"

# Create model directory
mkdir -p "$MODEL_DIR"

# Download model files
echo "Starting S3 sync..."
aws s3 sync "%s/%s/" "$MODEL_DIR/" --delete --exact-timestamps --endpoint-url "$ENDPOINT"

if [ $? -eq 0 ]; then
    echo "Model download completed successfully"
    ls -la "$MODEL_DIR"
else
    echo "Model download failed"
    exit 1
fi`, modelName, modelName, modelPath, modelPath, modelName),
								},
								"resources": map[string]interface{}{
									"requests": map[string]interface{}{"cpu": "200m", "memory": "512Mi"},
									"limits":   map[string]interface{}{"cpu": "1", "memory": "2Gi"},
								},
								"volumeMounts": []map[string]interface{}{
									{
										"name":      "shared-storage",
										"mountPath": "/shared/models",
									},
									{
										"name":      "storage-secret",
										"mountPath": "/secret",
										"readOnly":  true,
									},
								},
							},
						},
						"volumes": []map[string]interface{}{
							{
								"name": "shared-storage",
								"persistentVolumeClaim": map[string]interface{}{
									"claimName": "triton-model-storage", // This PVC should be shared with Triton pods
								},
							},
							{
								"name": "storage-secret",
								"secret": map[string]interface{}{
									"secretName": "storage-config",
									"items": []map[string]interface{}{
										{
											"key":  "localMinIO",
											"path": "localMinIO.json",
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

	_, err := r.DynamicClient.Resource(jobGVR).Namespace(namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create model download job: %w", err)
	}

	log.Info("Model download job created", "job", jobName, "model", modelName)
	return nil
}

// checkJobStatus checks the status of a Kubernetes Job
func (r TritonInferenceServerProvider) checkJobStatus(ctx context.Context, log logr.Logger, jobName, namespace string) (complete, failed bool, err error) {
	jobGVR := schema.GroupVersionResource{
		Group:    "batch",
		Version:  "v1",
		Resource: "jobs",
	}

	job, err := r.DynamicClient.Resource(jobGVR).Namespace(namespace).Get(ctx, jobName, metav1.GetOptions{})
	if err != nil {
		if utils.IsNotFoundError(err) {
			return false, true, nil // Job not found, consider it failed
		}
		return false, false, err
	}

	conditions, found, err := unstructured.NestedSlice(job.Object, "status", "conditions")
	if err != nil || !found {
		return false, false, nil // No status yet
	}

	for _, condition := range conditions {
		conditionMap := condition.(map[string]interface{})
		conditionType, found, err := unstructured.NestedString(conditionMap, "type")
		if err != nil || !found {
			continue
		}

		status, found, err := unstructured.NestedString(conditionMap, "status")
		if err != nil || !found {
			continue
		}

		if conditionType == "Complete" && status == "True" {
			return true, false, nil
		}

		if conditionType == "Failed" && status == "True" {
			return false, true, nil
		}
	}

	return false, false, nil // Still running
}
