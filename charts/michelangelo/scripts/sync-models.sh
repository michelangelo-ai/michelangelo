#!/bin/sh
set -e

echo "Installing jq and curl..."
yum install -y jq curl 2>/dev/null || apt-get update && apt-get install -y jq curl 2>/dev/null || apk add --no-cache jq curl 2>/dev/null || true
if ! command -v jq >/dev/null 2>&1; then
  echo "ERROR: jq installation failed. Please ensure network access"
  exit 1
fi
echo "jq and curl installed successfully"

MODEL_BASE_DIR="/mnt/models"
mkdir -p "$MODEL_BASE_DIR"

echo "Model sync daemon started on node: $NODE_NAME"
echo "Cloud provider: ${CLOUD_PROVIDER:-local}"
echo "Storage type: ${STORAGE_TYPE:-minio}"

# Configure storage client based on cloud provider
configure_storage() {
  if [ "$STORAGE_TYPE" = "gcs" ] || [ "$CLOUD_PROVIDER" = "gcp" ]; then
    echo "Configuring for Google Cloud Storage (GCS)..."
    # gsutil is pre-installed in google/cloud-sdk images
    # With Workload Identity, no explicit credentials needed
    if ! command -v gsutil >/dev/null 2>&1; then
      echo "Installing Google Cloud SDK..."
      curl -sSL https://sdk.cloud.google.com | bash -s -- --disable-prompts
      export PATH=$PATH:/root/google-cloud-sdk/bin
    fi
    echo "GCS configured (using Workload Identity for authentication)"
  else
    # Default: Use AWS CLI for S3-compatible storage (MinIO, etc.)
    echo "Configuring AWS CLI for S3-compatible storage..."
    echo "Endpoint: $AWS_ENDPOINT_URL"
    aws configure set aws_access_key_id "$AWS_ACCESS_KEY_ID"
    aws configure set aws_secret_access_key "$AWS_SECRET_ACCESS_KEY"
    if [ -n "$AWS_ENDPOINT_URL" ]; then
      aws configure set default.s3.endpoint_url "$AWS_ENDPOINT_URL"
    fi
    echo "AWS CLI configured"
  fi
}

# Sync from object storage (cloud-agnostic)
sync_from_storage() {
  local source_path=$1
  local dest_path=$2
  
  if [ "$STORAGE_TYPE" = "gcs" ] || [ "$CLOUD_PROVIDER" = "gcp" ]; then
    # GCS path format: gs://bucket/path
    gsutil -m rsync -r "$source_path" "$dest_path"
  else
    # S3/MinIO path format: s3://bucket/path
    aws s3 sync "$source_path" "$dest_path" --exact-timestamps ${AWS_ENDPOINT_URL:+--endpoint-url "$AWS_ENDPOINT_URL"}
  fi
}

# Validate Triton model directory structure:
# model-name/
#   config.pbtxt (optional but recommended)
#   <version>/        (at least one numeric version directory)
#     model.pt (or other model file)
validate_model_structure() {
  local dir=$1
  
  # Check if directory exists
  if [ ! -d "$dir" ]; then
    return 1
  fi
  
  # Check for at least one version directory (numeric name like 1, 2, etc.)
  local version_dirs=$(find "$dir" -maxdepth 1 -type d -regex '.*/[0-9]+' 2>/dev/null)
  if [ -z "$version_dirs" ]; then
    echo "  Invalid: No version directories found"
    return 1
  fi
  
  # Check each version directory has a valid model file
  for version_dir in $version_dirs; do
    # Check for common model file types
    if [ -f "$version_dir/model.pt" ] || \
       [ -f "$version_dir/model.onnx" ] || \
       [ -f "$version_dir/model.plan" ] || \
       [ -f "$version_dir/model.savedmodel" ] || \
       [ -d "$version_dir/model.savedmodel" ]; then
      # Found valid model file
      return 0
    fi
  done
  
  echo "  Invalid: No model file found in version directories"
  return 1
}

configure_storage

# Read inference servers list from mounted ConfigMap
INFERENCE_SERVERS_FILE="/config/inference-servers/servers.txt"
if [ ! -f "$INFERENCE_SERVERS_FILE" ]; then
  echo "No inference servers configured, exiting"
  exit 0
fi

echo "Configured inference servers:"
cat "$INFERENCE_SERVERS_FILE"

# Function to get currently loaded models from Triton
get_loaded_models() {
  local triton_service=$1
  curl -X POST -s "http://${triton_service}:80/v2/repository/index" -H "Content-Type: application/json" -d '{"ready": true}' 2>/dev/null | jq -r '.[].name' 2>/dev/null || echo ""
}

# Function to load model in Triton
load_model() {
  local triton_service=$1
  local model_name=$2
  echo "Loading model $model_name in Triton at $triton_service"
  response=$(curl -s -w "\n%{http_code}" -X POST "http://${triton_service}:80/v2/repository/models/$model_name/load" -H "Content-Type: application/json" -d '{}')
  http_code=$(echo "$response" | tail -n1)
  body=$(echo "$response" | sed '$d')
  
  if [ "$http_code" -eq 200 ]; then
    echo "✓ Model $model_name loaded successfully"
  else
    echo "✗ Failed to load model $model_name (HTTP $http_code)"
    echo "Response: $body"
  fi
}

# Function to unload model in Triton
unload_model() {
  local triton_service=$1
  local model_name=$2
  echo "Unloading model $model_name from Triton at $triton_service"
  curl -s -X POST "http://${triton_service}:80/v2/repository/models/$model_name/unload" -H "Content-Type: application/json" -d '{}'
  if [ $? -eq 0 ]; then
    echo "✓ Model $model_name unloaded successfully"
  else
    echo "✗ Failed to unload model $model_name"
  fi
}

# Main sync loop
while true; do
  echo "========================================"
  echo "Starting model sync cycle"
  echo "========================================"
  
  while IFS= read -r INFERENCE_SERVER || [ -n "$INFERENCE_SERVER" ]; do
    [ -z "$INFERENCE_SERVER" ] && continue
    echo "$INFERENCE_SERVER" | grep -q "^#" && continue
    
    echo ""
    echo "--- Processing inference server: $INFERENCE_SERVER ---"
    
    SERVER_MODEL_DIR="$MODEL_BASE_DIR/$INFERENCE_SERVER"
    mkdir -p "$SERVER_MODEL_DIR"
    
    triton_service="${INFERENCE_SERVER}-inference-service"
    
    echo "Checking Triton server at http://${triton_service}:80/v2/health/ready"
    
    if ! curl -s -f "http://${triton_service}:80/v2/health/ready" > /dev/null 2>&1; then
      echo "Triton server $triton_service not ready yet, skipping"
      echo "This is normal if Triton is still starting up."
      continue
    fi
    echo "✓ Triton server $triton_service is ready"
    
    CONFIG_FILE="/config/${INFERENCE_SERVER}/model-list.json"
    if [ -f "$CONFIG_FILE" ]; then
      cp "$CONFIG_FILE" /tmp/model-list.json
    else
      echo "No config file found at $CONFIG_FILE, using empty config"
      echo "[]" > /tmp/model-list.json
    fi
    
    echo "ConfigMap contents for $INFERENCE_SERVER:"
    cat /tmp/model-list.json | jq '.' 2>/dev/null || cat /tmp/model-list.json
    
    DESIRED_MODELS=$(jq -r '.[].name' /tmp/model-list.json 2>/dev/null | grep -v '^$' | sort -u || echo "")
    LOADED_MODELS=$(get_loaded_models "$triton_service")
    
    echo "Active models from ConfigMap: $DESIRED_MODELS"
    echo "Currently loaded models in Triton: $LOADED_MODELS"
    
    # Sync models from storage
    for desired_model in $DESIRED_MODELS; do
      if [ ! -z "$desired_model" ]; then
        storage_path=$(jq -r --arg model "$desired_model" '.[] | select(.name == $model) | .s3_path' /tmp/model-list.json 2>/dev/null)
        
        # Handle different storage path formats
        if [ "$storage_path" = "null" ] || [ -z "$storage_path" ]; then
          # Use MODELS_BUCKET env var, or default with GCP_PROJECT_ID prefix for GCS
          if [ "$STORAGE_TYPE" = "gcs" ] || [ "$CLOUD_PROVIDER" = "gcp" ]; then
            MODELS_BUCKET="${MODELS_BUCKET:-${GCP_PROJECT_ID}-deploy-models}"
            storage_path="gs://${MODELS_BUCKET}/$desired_model/"
          else
            MODELS_BUCKET="${MODELS_BUCKET:-deploy-models}"
            storage_path="s3://${MODELS_BUCKET}/$desired_model/"
          fi
        fi
        
        model_dir="$SERVER_MODEL_DIR/$desired_model"
        
        # Check if model structure is valid
        needs_sync=false
        if ! validate_model_structure "$model_dir"; then
          needs_sync=true
          if [ -d "$model_dir" ]; then
            echo "CLEANUP: Invalid model structure detected, removing and re-downloading: $model_dir"
            rm -rf "$model_dir"
          fi
        fi
        
        if [ "$needs_sync" = true ]; then
          echo "SYNC: Syncing model $desired_model from $storage_path to $model_dir/"
          mkdir -p "$model_dir"
          sync_from_storage "$storage_path" "$model_dir/"
          
          # Verify sync completed with valid structure
          if validate_model_structure "$model_dir"; then
            echo "✓ Model synced successfully with valid structure:"
          else
            echo "⚠ WARNING: Sync completed but model structure still invalid. Check storage source."
          fi
          ls -la "$model_dir/" 2>/dev/null || echo "Directory is empty or doesn't exist"
          # Show contents of version directories
          for vdir in $(find "$model_dir" -maxdepth 1 -type d -regex '.*/[0-9]+' 2>/dev/null); do
            echo "  Version $(basename $vdir) contents:"
            ls -la "$vdir/" 2>/dev/null || true
          done
        else
          echo "SKIP: Model $desired_model has valid structure, skipping download"
        fi
      fi
    done
    
    # Unload models that are no longer in config
    for loaded_model in $LOADED_MODELS; do
      if ! echo "$DESIRED_MODELS" | grep -q "^$loaded_model$"; then
        echo "Model $loaded_model no longer in config, unloading"
        unload_model "$triton_service" "$loaded_model"
      fi
    done
    
    # Load models from config ONLY if they're not already loaded
    for desired_model in $DESIRED_MODELS; do
      if [ ! -z "$desired_model" ]; then
        # Get fresh list of loaded models before checking each model
        CURRENT_LOADED_MODELS=$(get_loaded_models "$triton_service")
        if ! echo "$CURRENT_LOADED_MODELS" | grep -q "^$desired_model$"; then
          echo "Model $desired_model not loaded, loading now"
          load_model "$triton_service" "$desired_model"
        else
          echo "Model $desired_model already loaded, skipping"
        fi
      fi
    done
    
    echo "--- Completed sync for $INFERENCE_SERVER ---"
  done < "$INFERENCE_SERVERS_FILE"
  
  echo ""
  echo "Model sync cycle completed for all servers, sleeping for 60 seconds"
  sleep 60
done
