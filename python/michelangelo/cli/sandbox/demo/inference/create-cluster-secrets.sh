#!/bin/bash
#
# Creates Kubernetes secrets for k3d cluster credentials.
# These secrets are used by the InferenceServer to connect to target clusters.
#
# Usage: ./create-cluster-secrets.sh <k3d-cluster-name> [secret-prefix]
#
# Example:
#   ./create-cluster-secrets.sh k3d-my-cluster k3d-cluster-1
#

set -e

K3D_CLUSTER_NAME="${1:-}"
SECRET_PREFIX="${2:-${K3D_CLUSTER_NAME}}"

if [ -z "$K3D_CLUSTER_NAME" ]; then
    echo "Usage: $0 <k3d-cluster-name> [secret-prefix]"
    echo ""
    echo "Arguments:"
    echo "  k3d-cluster-name  Name of the k3d cluster (required)"
    echo "  secret-prefix     Prefix for the secret names (optional, defaults to cluster name)"
    echo ""
    echo "Example:"
    echo "  $0 k3d-my-cluster k3d-cluster-1"
    exit 1
fi

KUBECONFIG_FILE=$(mktemp)
trap "rm -f $KUBECONFIG_FILE" EXIT

echo "Extracting kubeconfig for k3d cluster: $K3D_CLUSTER_NAME"
k3d kubeconfig get "$K3D_CLUSTER_NAME" > "$KUBECONFIG_FILE"

# Extract the token from the kubeconfig
# k3d uses client certificates, so we need to extract the client cert/key or use a service account token
# For simplicity, we'll create a service account and get its token

echo "Extracting cluster CA data..."
CA_DATA=$(kubectl config view --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' --kubeconfig="$KUBECONFIG_FILE" | base64 -d)

echo "Extracting cluster server address..."
SERVER=$(kubectl config view --raw -o jsonpath='{.clusters[0].cluster.server}' --kubeconfig="$KUBECONFIG_FILE")
echo "  Server: $SERVER"

# Extract host and port from server URL
HOST=$(echo "$SERVER" | sed -E 's|https?://([^:]+):.*|\1|')
PORT=$(echo "$SERVER" | sed -E 's|https?://[^:]+:([0-9]+).*|\1|')
echo "  Host: $HOST"
echo "  Port: $PORT"

# For k3d, we need to create a service account with cluster-admin privileges
# and use its token for authentication
echo ""
echo "Creating service account for remote access in k3d cluster..."

# Switch to the k3d cluster context
export KUBECONFIG="$KUBECONFIG_FILE"

# Create namespace if not exists
kubectl create namespace michelangelo-system --dry-run=client -o yaml | kubectl apply -f -

# Create service account
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: michelangelo-remote-access
  namespace: michelangelo-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: michelangelo-remote-access-admin
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: michelangelo-remote-access
  namespace: michelangelo-system
---
apiVersion: v1
kind: Secret
metadata:
  name: michelangelo-remote-access-token
  namespace: michelangelo-system
  annotations:
    kubernetes.io/service-account.name: michelangelo-remote-access
type: kubernetes.io/service-account-token
EOF

# Wait for the token to be generated
echo "Waiting for service account token..."
sleep 2

# Get the token
TOKEN=$(kubectl get secret michelangelo-remote-access-token -n michelangelo-system -o jsonpath='{.data.token}' | base64 -d)

if [ -z "$TOKEN" ]; then
    echo "Error: Failed to get service account token"
    exit 1
fi

echo "Service account token obtained successfully"

# Switch back to the control plane cluster (default context)
unset KUBECONFIG

echo ""
echo "Creating secrets in control plane cluster..."

# Create token secret
kubectl create secret generic "${SECRET_PREFIX}-token" \
    --from-literal=token="$TOKEN" \
    --dry-run=client -o yaml | kubectl apply -f -

# Create CA data secret
kubectl create secret generic "${SECRET_PREFIX}-ca" \
    --from-literal=cadata="$CA_DATA" \
    --dry-run=client -o yaml | kubectl apply -f -

echo ""
echo "✅ Secrets created successfully!"
echo ""
echo "Secrets:"
echo "  - ${SECRET_PREFIX}-token"
echo "  - ${SECRET_PREFIX}-ca"
echo ""
echo "Update your InferenceServer CR with:"
echo "  clusterTargets:"
echo "    - clusterId: \"${SECRET_PREFIX}\""
echo "      kubernetes:"
echo "        host: \"${HOST}\""
echo "        port: \"${PORT}\""
echo "        tokenTag: \"${SECRET_PREFIX}-token\""
echo "        caDataTag: \"${SECRET_PREFIX}-ca\""

