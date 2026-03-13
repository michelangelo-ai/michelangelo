#!/bin/bash

# Start Services Script for Michelangelo with CRD Check
# This script starts MinIO, API Server, and Controllermgr with CRD checking enabled

echo "🚀 Starting Michelangelo Services with CRD Check Feature"
echo "================================================"

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo "❌ Docker is not running. Please start Docker Desktop first."
    exit 1
fi

# Check if k3d cluster is running
echo "📦 Checking k3d cluster..."
if ! kubectl get nodes > /dev/null 2>&1; then
    echo "⚠️  k3d cluster not accessible. Trying to start..."
    k3d cluster start michelangelo-local
    if [ $? -ne 0 ]; then
        echo "❌ Failed to start k3d cluster. Please ensure k3d is installed."
        exit 1
    fi
fi

# Start MinIO
echo "🗄️  Starting MinIO..."
docker rm -f minio 2>/dev/null
docker run -d \
    --name minio \
    -p 9090:9090 \
    -p 9091:9091 \
    -e MINIO_ROOT_USER=minioadmin \
    -e MINIO_ROOT_PASSWORD=minioadmin \
    minio/minio:latest server /data --console-address :9090 --address :9091

if [ $? -eq 0 ]; then
    echo "✅ MinIO started successfully"
    echo "   - Console: http://localhost:9090 (minioadmin/minioadmin)"
    echo "   - API: http://localhost:9091"
else
    echo "❌ Failed to start MinIO"
fi

echo ""
echo "📝 Services ready to start. Run these commands in separate terminals:"
echo ""
echo "Terminal 1 - API Server:"
echo "------------------------"
echo "cd /Users/hkriplani/GolandProjects/michelangelo"
echo "bazel run //go/cmd/apiserver"
echo ""
echo "Terminal 2 - Controllermgr with CRD Check (5-minute interval):"
echo "---------------------------------------------------------------"
echo "cd /Users/hkriplani/GolandProjects/michelangelo"
echo "export METRICS_ADDRESS=:8082"
echo "export HEALTH_ADDRESS=:8083"
echo "bazel run //go/cmd/controllermgr"
echo ""
echo "🔍 What to look for:"
echo "- API Server: 'dispatcher startup complete' and '[Fx] RUNNING'"
echo "- Controllermgr: 'Starting CRD schema comparison service' with 'interval:300'"
echo "- CRD Check will run every 5 minutes and report schema differences"
echo ""
echo "📊 Service Endpoints:"
echo "- API Server YARPC: localhost:14566"
echo "- Controllermgr Metrics: localhost:8082"
echo "- Controllermgr Health: localhost:8083"
echo "- MinIO Console: localhost:9090"
echo "- MinIO API: localhost:9091"

