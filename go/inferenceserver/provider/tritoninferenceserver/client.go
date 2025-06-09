package tritoninferenceserver

import (
	"context"
	"fmt"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"time"

	"github.com/go-logr/logr"
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

func (r TritonInferenceServerProvider) CreateInferenceServer(ctx context.Context, log logr.Logger, name, namespace string, configMapName string) error {
	// Create Kubernetes Deployment for Triton Server
	err := r.createTritonDeployment(ctx, log, name, namespace, configMapName)
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

	log.Info("Triton InferenceServer created successfully")
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
  CURRENT_TIME=$(date +%s)
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
        SYNC_START=$(date +%s)
        aws s3 sync "$s3_path/$name/" "$MODEL_DIR/" --delete --exact-timestamps --endpoint-url "$ENDPOINT" 2>&1 | while IFS= read -r line; do
          echo "[AWS SYNC OUTPUT] $line"
        done
        SYNC_EXIT_CODE=${PIPESTATUS[0]}
        SYNC_END=$(date +%s)
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
        DRYRUN_START=$(date +%s)
        SYNC_OUTPUT=$(aws s3 sync "$s3_path/$name/" "$MODEL_DIR/" --delete --exact-timestamps --endpoint-url "$ENDPOINT" --dryrun 2>&1)
        DRYRUN_EXIT_CODE=$?
        DRYRUN_END=$(date +%s)
        DRYRUN_DURATION=$((DRYRUN_END - DRYRUN_START))
        echo "[AWS DRYRUN] Completed in ${DRYRUN_DURATION}s with exit code: $DRYRUN_EXIT_CODE"
        echo "[AWS DRYRUN OUTPUT] $SYNC_OUTPUT"
        
        if [ $DRYRUN_EXIT_CODE -ne 0 ]; then
          echo "[AWS DRYRUN ERROR] Failed to check model $name for changes"
        elif echo "$SYNC_OUTPUT" | grep -q "(dryrun)"; then
          echo "Model $name has changes, performing update sync"
          echo "[AWS SYNC] Command: aws s3 sync $s3_path/$name/ $MODEL_DIR/ --delete --exact-timestamps --endpoint-url $ENDPOINT"
          SYNC_START=$(date +%s)
          aws s3 sync "$s3_path/$name/" "$MODEL_DIR/" --delete --exact-timestamps --endpoint-url "$ENDPOINT" 2>&1 | while IFS= read -r line; do
            echo "[AWS SYNC OUTPUT] $line"
          done
          SYNC_EXIT_CODE=${PIPESTATUS[0]}
          SYNC_END=$(date +%s)
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

func (r TritonInferenceServerProvider) UpdateInferenceServer(ctx context.Context, log logr.Logger, name, namespace string) error {
	log.Info("Updating Triton InferenceServer", "name", name, "namespace", namespace)
	// TODO: Implement actual update logic
	return nil
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
