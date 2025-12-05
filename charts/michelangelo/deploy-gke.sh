#!/bin/bash
# Deploy Michelangelo to Google Kubernetes Engine (GKE)

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

echo_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

echo_error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# Configuration
PROJECT_ID=${PROJECT_ID:-michelanglo-oss-196506}
REGION=${REGION:-us-east1}
ZONE=${ZONE:-us-east1-d}
CLUSTER_NAME=${CLUSTER_NAME:-kubernetes-gke-dev01}
NAMESPACE=${NAMESPACE:-michelangelo}
IMAGE_TAG=${IMAGE_TAG:-main}
# Set USE_ZONE=true for zonal clusters
USE_ZONE=${USE_ZONE:-false}

# Commands
CMD_CREATE=false
CMD_DELETE=false
CMD_DEPLOY_INFERENCE=false

show_help() {
    echo "Usage: $0 COMMAND [OPTIONS]"
    echo ""
    echo "Commands:"
    echo "  --create              Create/setup cluster and deploy Michelangelo"
    echo "  --delete              Delete Michelangelo resources from cluster"
    echo "  --deploy-inference    Deploy inference server with GPU tolerations"
    echo ""
    echo "Options:"
    echo "  --project-id ID       GCP project ID (default: michelanglo-oss-196506)"
    echo "  --region REGION       GCP region (default: us-east1)"
    echo "  --zone ZONE           GCP zone for zonal clusters (default: us-east1-d)"
    echo "  --cluster-name NAME   GKE cluster name (default: kubernetes-gke-dev01)"
    echo "  --namespace NS        Kubernetes namespace (default: michelangelo)"
    echo "  --image-tag TAG       Image tag to deploy (default: main)"
    echo "  --help                Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 --create                          # Full setup and deployment"
    echo "  $0 --deploy-inference                # Deploy inference server only"
    echo "  $0 --delete                          # Clean up all resources"
    echo "  $0 --create --project-id my-project  # Deploy to specific project"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --create)
            CMD_CREATE=true
            shift
            ;;
        --delete)
            CMD_DELETE=true
            shift
            ;;
        --deploy-inference)
            CMD_DEPLOY_INFERENCE=true
            shift
            ;;
        --project-id)
            PROJECT_ID="$2"
            shift 2
            ;;
        --region)
            REGION="$2"
            shift 2
            ;;
        --zone)
            ZONE="$2"
            shift 2
            ;;
        --cluster-name)
            CLUSTER_NAME="$2"
            shift 2
            ;;
        --namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        --image-tag)
            IMAGE_TAG="$2"
            shift 2
            ;;
        --help)
            show_help
            exit 0
            ;;
        *)
            echo_error "Unknown option: $1. Use --help for usage information."
            ;;
    esac
done

# Validate at least one command is specified
if [ "$CMD_CREATE" = false ] && [ "$CMD_DELETE" = false ] && [ "$CMD_DEPLOY_INFERENCE" = false ]; then
    echo_error "No command specified. Use --create, --delete, or --deploy-inference. Use --help for more info."
fi

# Check prerequisites
check_prerequisites() {
    command -v gcloud >/dev/null 2>&1 || echo_error "gcloud CLI not found. Please install: https://cloud.google.com/sdk/docs/install"
    command -v kubectl >/dev/null 2>&1 || echo_error "kubectl not found. Please install: https://kubernetes.io/docs/tasks/tools/"
    command -v helm >/dev/null 2>&1 || echo_error "helm not found. Please install: https://helm.sh/docs/intro/install/"
}

# Get cluster credentials
get_credentials() {
    echo_info "Getting cluster credentials for $CLUSTER_NAME"
    gcloud config set project $PROJECT_ID
    if [ "$USE_ZONE" = true ]; then
        gcloud container clusters get-credentials $CLUSTER_NAME --zone $ZONE
    else
        gcloud container clusters get-credentials $CLUSTER_NAME --region $REGION
    fi
}

# ============================================================================
# SETUP CLUSTER CRD (Required for Ray job scheduling)
# ============================================================================
setup_cluster_crd() {
    echo_info "Setting up Cluster CRD for Ray job scheduling..."
    
    # Create ma-system namespace (where Cluster CRDs live)
    echo_info "Creating ma-system namespace..."
    kubectl create namespace ma-system 2>/dev/null || echo_info "ma-system namespace already exists"

    # Create ray-manager ServiceAccount in default namespace
    echo_info "Creating ray-manager ServiceAccount..."
    kubectl create serviceaccount ray-manager -n default 2>/dev/null || echo_info "ray-manager ServiceAccount already exists"

    # Apply RBAC for ray-manager to manage Ray resources
    echo_info "Applying RBAC for ray-manager..."
    kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ray-manager-role
rules:
- apiGroups: ["ray.io"]
  resources: ["rayclusters", "rayjobs", "rayservices"]
  verbs: ["*"]
- apiGroups: [""]
  resources: ["pods", "pods/log", "services", "configmaps", "secrets"]
  verbs: ["*"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ray-manager-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ray-manager-role
subjects:
- kind: ServiceAccount
  name: ray-manager
  namespace: default
EOF

    # Create a long-lived token for ray-manager
    echo_info "Creating ray-manager token..."
    TOKEN=$(kubectl create token ray-manager -n default --duration=87600h 2>/dev/null || echo "")
    
    if [ -z "$TOKEN" ]; then
        echo_warn "Could not create token (may need newer kubectl version), trying alternative method..."
        # Alternative: create a secret-based token
        kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: ray-manager-token
  namespace: default
  annotations:
    kubernetes.io/service-account.name: ray-manager
type: kubernetes.io/service-account-token
EOF
        sleep 2
        TOKEN=$(kubectl get secret ray-manager-token -n default -o jsonpath='{.data.token}' | base64 -d)
    fi

    # Get cluster CA certificate
    echo_info "Extracting cluster CA certificate..."
    CA_DATA=$(kubectl config view --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' | base64 -d)

    # Create secrets in default namespace for the Cluster CRD to reference
    CLUSTER_ID="${CLUSTER_NAME}"
    
    echo_info "Creating cluster secrets..."
    # Delete existing secrets if they exist, then recreate
    kubectl delete secret "cluster-${CLUSTER_ID}-client-token" -n default 2>/dev/null || true
    kubectl create secret generic "cluster-${CLUSTER_ID}-client-token" \
        --from-literal=token="$TOKEN" \
        -n default

    kubectl delete secret "cluster-${CLUSTER_ID}-ca-data" -n default 2>/dev/null || true
    kubectl create secret generic "cluster-${CLUSTER_ID}-ca-data" \
        --from-literal=cadata="$CA_DATA" \
        -n default

    # Create the Cluster CRD
    # This tells the scheduler where to run Ray jobs
    echo_info "Creating Cluster CRD for ${CLUSTER_ID}..."
    kubectl apply -f - <<EOF
apiVersion: michelangelo.api/v2
kind: Cluster
metadata:
  name: ${CLUSTER_ID}
  namespace: ma-system
spec:
  kubernetes:
    rest:
      host: "https://kubernetes.default.svc"
      port: "443"
      tokenTag: "cluster-${CLUSTER_ID}-client-token"
      caDataTag: "cluster-${CLUSTER_ID}-ca-data"
    skus: []
EOF

    echo_info "✅ Cluster CRD '${CLUSTER_ID}' created successfully"
    
    # Verify
    kubectl get clusters.michelangelo.api -n ma-system
}

# ============================================================================
# CREATE COMMAND
# ============================================================================
do_create() {
    echo_info "=== Creating Michelangelo deployment ==="
    echo ""
    echo_info "Configuration:"
    echo "  Project ID: $PROJECT_ID"
    echo "  Region: $REGION"
    echo "  Zone: $ZONE"
    echo "  Cluster Name: $CLUSTER_NAME"
    echo "  Namespace: $NAMESPACE"
    echo "  Image Tag: $IMAGE_TAG"
    echo ""

    check_prerequisites
    get_credentials

    # Create GCS buckets (with project-specific prefix to ensure uniqueness)
    echo_info "Creating GCS buckets..."
    BUCKET_PREFIX="${PROJECT_ID}"
    for bucket in "default" "deploy-models" "log-viewer" "logs"; do
        FULL_BUCKET="${BUCKET_PREFIX}-${bucket}"
        if gsutil ls -p $PROJECT_ID "gs://${FULL_BUCKET}" >/dev/null 2>&1; then
            echo_info "  Bucket gs://${FULL_BUCKET} already exists"
        else
            gsutil mb -p $PROJECT_ID -l $REGION "gs://${FULL_BUCKET}" || echo_warn "Failed to create bucket gs://${FULL_BUCKET}"
            echo_info "  Created bucket gs://${FULL_BUCKET}"
        fi
    done

    # Add Helm repositories
    echo_info "Adding Helm repositories..."
    helm repo add istio https://istio-release.storage.googleapis.com/charts 2>/dev/null || true
    helm repo add kuberay https://ray-project.github.io/kuberay-helm 2>/dev/null || true
    helm repo add spark-operator https://kubeflow.github.io/spark-operator 2>/dev/null || true
    helm repo update

    # -------------------------------------------------------------------------
    # Install KubeRay Operator (required for Ray cluster management)
    # -------------------------------------------------------------------------
    echo_info "Installing KubeRay operator..."
    if ! kubectl get namespace ray-system >/dev/null 2>&1; then
        kubectl create namespace ray-system
    fi
    helm upgrade --install kuberay-operator kuberay/kuberay-operator \
        --namespace ray-system \
        --set 'tolerations[0].key=nvidia.com/gpu' \
        --set 'tolerations[0].operator=Exists' \
        --set 'tolerations[0].effect=NoSchedule' \
        --wait --timeout 5m || echo_warn "KubeRay operator installation may have issues, continuing..."
    
    echo_info "Waiting for KubeRay operator to be ready..."
    kubectl rollout status deployment/kuberay-operator -n ray-system --timeout=120s || echo_warn "KubeRay operator rollout timed out"

    # -------------------------------------------------------------------------
    # Create Cluster CRD (required for scheduler to assign Ray jobs)
    # The scheduler looks for Cluster resources to know where to run jobs
    # -------------------------------------------------------------------------
    setup_cluster_crd

    # Create namespace
    echo_info "Creating namespace: $NAMESPACE"
    kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

    # -------------------------------------------------------------------------
    # Check for GHCR image pull secret (required to pull images from ghcr.io)
    # -------------------------------------------------------------------------
    if ! kubectl get secret ghcr-secret -n $NAMESPACE >/dev/null 2>&1; then
        echo ""
        echo_warn "============================================================"
        echo_warn "GHCR image pull secret 'ghcr-secret' not found!"
        echo_warn "============================================================"
        echo ""
        echo "Container images are hosted on GitHub Container Registry (GHCR)"
        echo "which requires authentication. Create the secret before continuing:"
        echo ""
        echo "  kubectl create secret docker-registry ghcr-secret \\"
        echo "    --namespace $NAMESPACE \\"
        echo "    --docker-server=ghcr.io \\"
        echo "    --docker-username=YOUR_GITHUB_USERNAME \\"
        echo "    --docker-password=YOUR_GITHUB_PAT"
        echo ""
        echo "To create a GitHub PAT:"
        echo "  1. Go to https://github.com/settings/tokens"
        echo "  2. Generate a new token (classic) with 'read:packages' scope"
        echo "  3. Use that token as YOUR_GITHUB_PAT above"
        echo ""
        read -p "Press Enter after creating the secret, or Ctrl+C to abort..."
        
        # Verify secret was created
        if ! kubectl get secret ghcr-secret -n $NAMESPACE >/dev/null 2>&1; then
            echo_error "Secret 'ghcr-secret' still not found. Please create it and re-run the script."
        fi
        echo_info "Secret 'ghcr-secret' found!"
    else
        echo_info "GHCR image pull secret found ✓"
    fi

    # Deploy Michelangelo
    echo_info "Installing Michelangelo Helm chart..."
    helm upgrade --install michelangelo ./charts/michelangelo \
        -f charts/michelangelo/values-gke.yaml \
        --namespace $NAMESPACE \
        --set image.tag="${IMAGE_TAG}" \
        --wait \
        --timeout 15m

    echo ""
    echo_info "=== Deployment complete! ==="
    echo ""
    echo_info "To check status:"
    echo "  kubectl get pods -n $NAMESPACE"
    echo ""
    echo_info "To deploy inference server:"
    echo "  $0 --deploy-inference"
}

# ============================================================================
# DELETE COMMAND
# ============================================================================
do_delete() {
    echo_info "=== Deleting Michelangelo resources ==="
    echo ""
    echo_info "Configuration:"
    echo "  Namespace: $NAMESPACE"
    echo ""

    check_prerequisites
    get_credentials

    # Confirm deletion
    echo_warn "This will delete all Michelangelo resources in namespace '$NAMESPACE'."
    read -p "Are you sure? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo_info "Deletion cancelled."
        exit 0
    fi

    # Delete inference server CRs
    echo_info "Deleting InferenceServer resources..."
    kubectl delete inferenceservers --all -n $NAMESPACE 2>/dev/null || echo_warn "No InferenceServer resources found"

    # Delete Helm release
    echo_info "Uninstalling Helm release..."
    helm uninstall michelangelo -n $NAMESPACE 2>/dev/null || echo_warn "Helm release not found"

    # Delete any remaining resources in namespace
    echo_info "Cleaning up remaining resources..."
    kubectl delete deployments --all -n $NAMESPACE 2>/dev/null || true
    kubectl delete services --all -n $NAMESPACE 2>/dev/null || true
    kubectl delete configmaps --all -n $NAMESPACE 2>/dev/null || true
    kubectl delete secrets --all -n $NAMESPACE 2>/dev/null || true
    kubectl delete pvc --all -n $NAMESPACE 2>/dev/null || true

    # Optionally delete namespace
    read -p "Delete namespace '$NAMESPACE'? (y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo_info "Deleting namespace..."
        kubectl delete namespace $NAMESPACE 2>/dev/null || echo_warn "Namespace not found"
    fi

    # Optionally delete Cluster CRD and related resources
    read -p "Delete Cluster CRD and ray-manager resources? (y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo_info "Deleting Cluster CRD..."
        kubectl delete clusters.michelangelo.api $CLUSTER_NAME -n ma-system 2>/dev/null || echo_warn "Cluster CRD not found"
        kubectl delete secret "cluster-${CLUSTER_NAME}-client-token" -n default 2>/dev/null || true
        kubectl delete secret "cluster-${CLUSTER_NAME}-ca-data" -n default 2>/dev/null || true
        kubectl delete clusterrolebinding ray-manager-binding 2>/dev/null || true
        kubectl delete clusterrole ray-manager-role 2>/dev/null || true
        kubectl delete serviceaccount ray-manager -n default 2>/dev/null || true
        kubectl delete namespace ma-system 2>/dev/null || true
    fi

    # Optionally delete Istio
    read -p "Delete Istio (istio-system namespace)? (y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo_info "Deleting Istio..."
        helm uninstall istiod -n istio-system 2>/dev/null || true
        helm uninstall istio-base -n istio-system 2>/dev/null || true
        kubectl delete namespace istio-system 2>/dev/null || echo_warn "istio-system namespace not found"
        # Delete Gateway API CRDs
        kubectl delete -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.1.0/standard-install.yaml 2>/dev/null || true
    fi

    echo ""
    echo_info "=== Deletion complete! ==="
    echo ""
    echo_info "Note: GCS buckets were NOT deleted. To delete them manually:"
    echo "  gsutil rm -r gs://${PROJECT_ID}-default"
    echo "  gsutil rm -r gs://${PROJECT_ID}-deploy-models"
    echo "  gsutil rm -r gs://${PROJECT_ID}-log-viewer"
    echo "  gsutil rm -r gs://${PROJECT_ID}-logs"
}

# ============================================================================
# DEPLOY INFERENCE COMMAND
# ============================================================================
do_deploy_inference() {
    echo_info "=== Deploying Inference Server ==="
    echo ""
    echo_info "Configuration:"
    echo "  Namespace: $NAMESPACE"
    echo ""

    check_prerequisites
    get_credentials

    # Check if controllermgr is running (required to handle InferenceServer CR)
    if ! kubectl get deployment -n $NAMESPACE -l app.kubernetes.io/component=controllermgr -o name | grep -q deployment; then
        echo_error "Controller manager not found. Run '$0 --create' first."
    fi

    # -------------------------------------------------------------------------
    # Step 1: Install Istio with Gateway API support
    # -------------------------------------------------------------------------
    echo_info "Setting up Istio with Gateway API..."

    # Install Istio base (CRDs and cluster roles)
    echo_info "Installing Istio base..."
    helm upgrade --install istio-base istio/base \
        --namespace istio-system \
        --create-namespace \
        --wait \
        --timeout 5m \
        --set defaultRevision=default

    # Install Gateway API CRDs
    echo_info "Installing Gateway API CRDs..."
    kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.1.0/standard-install.yaml
    
    # Wait for Gateway API CRDs to be established
    echo_info "Waiting for Gateway API CRDs..."
    kubectl wait --for=condition=Established \
        crd/gateways.gateway.networking.k8s.io \
        crd/httproutes.gateway.networking.k8s.io \
        crd/gatewayclasses.gateway.networking.k8s.io \
        --timeout=60s

    # Install Istio control plane (istiod) with GPU tolerations
    echo_info "Installing Istio control plane..."
    helm upgrade --install istiod istio/istiod \
        --namespace istio-system \
        --wait \
        --timeout 5m \
        --set pilot.tolerations[0].key=nvidia.com/gpu \
        --set pilot.tolerations[0].operator=Exists \
        --set pilot.tolerations[0].effect=NoSchedule \
        --set global.proxy.resources.requests.cpu=10m \
        --set global.proxy.resources.requests.memory=64Mi \
        --set pilot.resources.requests.cpu=100m \
        --set pilot.resources.requests.memory=256Mi

    # Wait for Istio to be ready
    echo_info "Waiting for Istio control plane..."
    kubectl wait --for=condition=available deployment --namespace=istio-system --all --timeout=300s

    echo_info "✅ Istio installed successfully"

    # -------------------------------------------------------------------------
    # Step 2: Create Gateway
    # -------------------------------------------------------------------------
    echo_info "Creating Gateway API Gateway..."
    kubectl apply -f charts/michelangelo/demo/gateway-api-setup-gke.yaml

    # Wait for Gateway to be programmed
    echo_info "Waiting for Gateway to be ready..."
    for i in {1..30}; do
        if kubectl get gateway ma-gateway -n $NAMESPACE -o jsonpath='{.status.conditions[?(@.type=="Programmed")].status}' 2>/dev/null | grep -q "True"; then
            echo_info "✅ Gateway is ready"
            break
        fi
        if [ $i -eq 30 ]; then
            echo_warn "Gateway not yet ready, continuing anyway..."
        fi
        echo "  Waiting for Gateway... ($i/30)"
        sleep 5
    done

    # Show Gateway status
    kubectl get gateway ma-gateway -n $NAMESPACE -o wide 2>/dev/null || true

    # -------------------------------------------------------------------------
    # Step 3: Deploy InferenceServer CR
    # -------------------------------------------------------------------------
    echo_info "Applying InferenceServer CR..."
    kubectl apply -f charts/michelangelo/demo/inference-server-gke.yaml

    # Wait for the controller to create the deployment
    echo_info "Waiting for inference server deployment to be created..."
    DEPLOYMENT_NAME="triton-inference-server-bert-cola"
    for i in {1..60}; do
        if kubectl get deployment $DEPLOYMENT_NAME -n $NAMESPACE >/dev/null 2>&1; then
            echo_info "Deployment found!"
            break
        fi
        if [ $i -eq 60 ]; then
            echo_error "Timeout waiting for deployment. Check controller logs: kubectl logs -n $NAMESPACE -l app.kubernetes.io/component=controllermgr"
        fi
        echo "  Waiting for controller to create deployment... ($i/60)"
        sleep 2
    done

    # Patch the deployment with GPU tolerations
    # NOTE: THIS is a workaround to add GPU tolerations to the deployment. AKA HACKY.
    echo_info "Patching deployment with GPU tolerations..."
    # First check if tolerations already exist
    if ! kubectl get deployment $DEPLOYMENT_NAME -n $NAMESPACE -o jsonpath='{.spec.template.spec.tolerations}' | grep -q "nvidia.com/gpu"; then
        kubectl patch deployment $DEPLOYMENT_NAME -n $NAMESPACE --type='json' -p='[
          {"op": "add", "path": "/spec/template/spec/tolerations", "value": [
            {"key": "nvidia.com/gpu", "operator": "Exists", "effect": "NoSchedule"}
          ]}
        ]'
        echo_info "GPU tolerations added"
    else
        echo_info "GPU tolerations already exist"
    fi

    # Reduce resource requests to fit on dev cluster
    echo_info "Reducing resource requests for dev cluster..."
    kubectl patch deployment $DEPLOYMENT_NAME -n $NAMESPACE --type='json' -p='[
      {"op": "replace", "path": "/spec/template/spec/containers/0/resources/requests/cpu", "value": "100m"},
      {"op": "replace", "path": "/spec/template/spec/containers/0/resources/requests/memory", "value": "512Mi"}
    ]' 2>/dev/null || echo_info "Resource patch skipped (may already be set)"

    # Wait for deployment rollout
    echo_info "Waiting for inference server deployment rollout..."
    kubectl rollout status deployment/$DEPLOYMENT_NAME -n $NAMESPACE --timeout=300s || echo_warn "Deployment rollout timed out"

    # Wait for InferenceServer CR to reach SERVING state
    INFERENCE_SERVER_NAME="inference-server-bert-cola"
    echo_info "Waiting for InferenceServer '$INFERENCE_SERVER_NAME' to reach SERVING state..."
    echo "   (This may take 5-10 minutes for first-time Triton image pull)"
    
    if kubectl wait --for=jsonpath='{.status.state}'=INFERENCE_SERVER_STATE_SERVING \
        inferenceservers.michelangelo.api/$INFERENCE_SERVER_NAME \
        -n $NAMESPACE \
        --timeout=720s; then
        echo_info "✅ Inference server is ready and serving!"
    else
        echo_warn "Inference server did not reach SERVING state within timeout"
        echo_info "Check status with: kubectl get inferenceservers $INFERENCE_SERVER_NAME -n $NAMESPACE -o yaml"
        echo_info "Check logs with: kubectl logs -n $NAMESPACE -l app=$DEPLOYMENT_NAME"
    fi

    # -------------------------------------------------------------------------
    # Step 4: Fix HTTPRoute namespace reference
    # TODO: This is a workaround for a bug in go/components/inferenceserver/gateways/backends/triton.go
    #       The HTTPRoute is created with parentRefs.namespace="default" instead of the actual namespace.
    #       Fix the Go code to use request.Namespace instead of hardcoded "default".
    # -------------------------------------------------------------------------
    HTTPROUTE_NAME="${INFERENCE_SERVER_NAME}-httproute"
    echo_info "Patching HTTPRoute to fix gateway namespace reference..."
    if kubectl get httproute $HTTPROUTE_NAME -n $NAMESPACE >/dev/null 2>&1; then
        kubectl patch httproute $HTTPROUTE_NAME -n $NAMESPACE --type='json' -p="[
          {\"op\": \"replace\", \"path\": \"/spec/parentRefs/0/namespace\", \"value\": \"$NAMESPACE\"}
        ]"
        echo_info "HTTPRoute patched successfully"
    else
        echo_warn "HTTPRoute $HTTPROUTE_NAME not found, skipping patch"
    fi

    # Check InferenceServer status
    echo_info "Checking InferenceServer status..."
    kubectl get inferenceservers -n $NAMESPACE

    # Enable and deploy model-sync
    echo_info "Enabling model-sync deployment..."
    helm upgrade michelangelo ./charts/michelangelo \
        -f charts/michelangelo/values-gke.yaml \
        --namespace $NAMESPACE \
        --set inference.modelSync.enabled=true \
        --reuse-values

    # Wait for model-sync to be ready
    echo_info "Waiting for model-sync to be ready..."
    kubectl rollout status deployment/michelangelo-model-sync -n $NAMESPACE --timeout=120s || echo_warn "Model-sync rollout timed out"

    echo ""
    echo_info "=== Inference Server Deployment Complete! ==="
    echo ""
    echo_info "What was deployed:"
    echo "  ✅ Istio service mesh (istio-system namespace)"
    echo "  ✅ Gateway API CRDs"
    echo "  ✅ Gateway (ma-gateway)"
    echo "  ✅ InferenceServer CR (inference-server-bert-cola)"
    echo "  ✅ Triton deployment with GPU tolerations"
    echo "  ✅ Model-sync deployment"
    echo ""
    echo_info "To check status:"
    echo "  kubectl get inferenceservers -n $NAMESPACE"
    echo "  kubectl get gateway -n $NAMESPACE"
    echo "  kubectl get pods -n $NAMESPACE"
    echo ""
    echo_info "To view logs:"
    echo "  kubectl logs -n $NAMESPACE -l app=$DEPLOYMENT_NAME        # Triton"
    echo "  kubectl logs -n $NAMESPACE -l app.kubernetes.io/component=model-sync  # Model-sync"
    echo ""
    echo_info "To access the inference server (if using LoadBalancer):"
    echo "  GATEWAY_IP=\$(kubectl get gateway ma-gateway -n $NAMESPACE -o jsonpath='{.status.addresses[0].value}')"
    echo "  curl http://\$GATEWAY_IP:8889/v2/health/ready"
}

# ============================================================================
# MAIN
# ============================================================================

if [ "$CMD_CREATE" = true ]; then
    do_create
fi

if [ "$CMD_DELETE" = true ]; then
    do_delete
fi

if [ "$CMD_DEPLOY_INFERENCE" = true ]; then
    do_deploy_inference
fi
