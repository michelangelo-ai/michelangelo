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
  
  if [ "$http_code" -eq 200 ]; then
    echo "Model $model_name loaded successfully"
  else
    echo "Failed to load model $model_name (HTTP $http_code)"
  fi
}

# Function to unload model in Triton
unload_model() {
  local triton_service=$1
  local model_name=$2
  echo "Unloading model $model_name from Triton at $triton_service"
  curl -s -X POST "http://${triton_service}:80/v2/repository/models/$model_name/unload" -H "Content-Type: application/json" -d '{}'
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
    
    if ! curl -s -f "http://${triton_service}:80/v2/health/ready" > /dev/null 2>&1; then
      echo "Triton server $triton_service not ready yet, skipping"
      continue
    fi
    
    CONFIG_FILE="/config/${INFERENCE_SERVER}/model-list.json"
    if [ -f "$CONFIG_FILE" ]; then
      cp "$CONFIG_FILE" /tmp/model-list.json
    else
      echo "[]" > /tmp/model-list.json
    fi
    
    DESIRED_MODELS=$(jq -r '.[].name' /tmp/model-list.json 2>/dev/null | grep -v '^$' | sort -u || echo "")
    LOADED_MODELS=$(get_loaded_models "$triton_service")
    
    for desired_model in $DESIRED_MODELS; do
      if [ ! -z "$desired_model" ]; then
        storage_path=$(jq -r --arg model "$desired_model" '.[] | select(.name == $model) | .s3_path' /tmp/model-list.json 2>/dev/null)
        
        # Handle different storage path formats
        if [ "$storage_path" = "null" ] || [ -z "$storage_path" ]; then
          if [ "$STORAGE_TYPE" = "gcs" ] || [ "$CLOUD_PROVIDER" = "gcp" ]; then
            storage_path="gs://deploy-models/$desired_model/"
          else
            storage_path="s3://deploy-models/$desired_model/"
          fi
        fi
        
        model_dir="$SERVER_MODEL_DIR/$desired_model"
        
        if [ ! -d "$model_dir" ] || [ -z "$(ls -A $model_dir 2>/dev/null)" ]; then
          echo "SYNC: Syncing model $desired_model from $storage_path to $model_dir/"
          mkdir -p "$model_dir"
          sync_from_storage "$storage_path" "$model_dir/"
        fi
      fi
    done
    
    for loaded_model in $LOADED_MODELS; do
      if ! echo "$DESIRED_MODELS" | grep -q "^$loaded_model$"; then
        unload_model "$triton_service" "$loaded_model"
      fi
    done
    
    for desired_model in $DESIRED_MODELS; do
      if [ ! -z "$desired_model" ]; then
        CURRENT_LOADED_MODELS=$(get_loaded_models "$triton_service")
        if ! echo "$CURRENT_LOADED_MODELS" | grep -q "^$desired_model$"; then
          load_model "$triton_service" "$desired_model"
        fi
      fi
    done
    
    echo "--- Completed sync for $INFERENCE_SERVER ---"
  done < "$INFERENCE_SERVERS_FILE"
  
  echo ""
  echo "Model sync cycle completed, sleeping for 60 seconds"
  sleep 60
done
