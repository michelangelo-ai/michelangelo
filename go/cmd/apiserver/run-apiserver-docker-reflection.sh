#!/bin/bash

# Script to run the apiserver Docker container with k3d cluster connection and reflection support
set -e

IMAGE_NAME="michelangelo-apiserver-reflection:latest"
CONTAINER_NAME="michelangelo-apiserver-reflection"

# Mode selection
MODE=${1:-"test"}

echo " Starting apiserver Docker container with reflection support..."
echo "Mode: $MODE"

# Function to cleanup
cleanup() {
    echo "🧹 Cleaning up..."
    docker stop $CONTAINER_NAME 2>/dev/null || true
    docker rm $CONTAINER_NAME 2>/dev/null || true
    if [[ -n "$TEMP_KUBECONFIG" && -f "$TEMP_KUBECONFIG" ]]; then
        rm -f "$TEMP_KUBECONFIG"
        echo "Cleaned up temporary kubeconfig"
    fi
    if [[ -n "$TEMP_KUBE_DIR" && -d "$TEMP_KUBE_DIR" ]]; then
        rm -rf "$TEMP_KUBE_DIR"
        echo "Cleaned up temporary kube directory"
    fi
}

# Trap cleanup on exit
trap cleanup EXIT

# Always build from project root, regardless of current directory
build_image() {
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
    cd "$PROJECT_ROOT"
    docker build -f go/cmd/apiserver/Dockerfile.apiserver-reflection -t $IMAGE_NAME .
}

# Setup kubeconfig
echo "📋 Setting up kubeconfig for k3d cluster..."
TEMP_KUBECONFIG=$(mktemp)
k3d kubeconfig get michelangelo-sandbox > "$TEMP_KUBECONFIG"

# Create a temporary directory for proper kubeconfig mounting
TEMP_KUBE_DIR=$(mktemp -d)
cp "$TEMP_KUBECONFIG" "$TEMP_KUBE_DIR/config"

# Get k3d cluster endpoint  
K3D_ENDPOINT="https://127.0.0.1:65394"

# Debug: check what we're mounting
echo " Debug: Temp kube dir contents:"
ls -la "$TEMP_KUBE_DIR"

case $MODE in
    "build")
        build_image
        ;;
    "test")
        build_image
        docker run --name $CONTAINER_NAME \
            --network host \
            -v "$TEMP_KUBE_DIR:/root/.kube:ro" \
            -e KUBECONFIG=/root/.kube/config \
            -e KUBERNETES_MASTER="$K3D_ENDPOINT" \
            --rm \
            $IMAGE_NAME
        ;;
    "server")
        docker run --name $CONTAINER_NAME \
            --network host \
            -v "$TEMP_KUBE_DIR:/root/.kube:ro" \
            -e KUBECONFIG=/root/.kube/config \
            -e KUBERNETES_MASTER="$K3D_ENDPOINT" \
            -d \
            $IMAGE_NAME
        
        echo "Container started. Test with: grpcurl -plaintext -H rpc-caller:grpcurl -H rpc-service:ma-apiserver -H rpc-encoding:proto 127.0.0.1:15566 list"
        ;;
    "bash")
        docker run --name $CONTAINER_NAME \
            --network host \
            -v "$TEMP_KUBE_DIR:/root/.kube:ro" \
            -e KUBECONFIG=/root/.kube/config \
            -e KUBERNETES_MASTER="$K3D_ENDPOINT" \
            --rm -it \
            $IMAGE_NAME /bin/bash
        ;;
    "quit")
        docker stop $CONTAINER_NAME 2>/dev/null || true
        docker rm $CONTAINER_NAME 2>/dev/null || true
        ;;
    *)
        echo "Usage: $0 [build|test|server|bash|quit]"
        echo "  build  - Build the Docker image"
        echo "  test   - Build and run container with reflection test"
        echo "  server - Run container in background"
        echo "  bash   - Run container with interactive shell"
        echo "  quit   - Stop and remove container"
        exit 1
        ;;
esac 