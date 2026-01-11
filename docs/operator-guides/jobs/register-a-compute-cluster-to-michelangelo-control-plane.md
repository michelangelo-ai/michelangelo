# Register a Compute Cluster

Register an existing compute Kubernetes cluster with the Michelangelo control plane to enable running Ray jobs.

### Prerequisites
- Existing Kubernetes compute cluster accessible via kubectl
- KubeRay operator installed in the compute cluster (`ray-system` namespace)
- Michelangelo control plane running
- Access to object storage (S3/MinIO) used by the control plane
- RBAC manifest for service account with permissions to run Ray Jobs / Ray Clusters. 
- Network connectivity between control plane and compute cluster (control plane must be able to reach compute cluster API server)

### What gets configured
- RBAC for `ray-manager` ServiceAccount in `default` namespace
- Storage configuration in compute cluster:
  - `michelangelo-config` ConfigMap (S3 endpoint/credentials)
  - `aws-credentials` Secret
- Cluster CRD in control plane (`ma-system`) pointing to the compute cluster
- Secrets in control plane for compute cluster CA and client token

```bash
# Example names
COMPUTE_CLUSTER=michelangelo-compute-0
COMPUTE_CONTEXT=my-compute-cluster-context  # Your kubectl context for the compute cluster
CONTROL_PLANE_CONTEXT=my-control-plane-context  # Your kubectl context for the control plane
```

### 1) Configure storage in the compute cluster
Ensure Ray pods inherit the same storage configuration used by the control plane.

Create the `michelangelo-config` ConfigMap with your storage configuration:

```bash
cat <<EOF | kubectl --context "${COMPUTE_CONTEXT}" apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: michelangelo-config
data:
  MA_FILE_SYSTEM: s3://default
  MA_FILE_SYSTEM_S3_SCHEME: http
  AWS_ACCESS_KEY_ID: minioadmin
  AWS_SECRET_ACCESS_KEY: minioadmin
  AWS_ENDPOINT_URL: << MINIO STORAGE URL >>
EOF
```

Create the `aws-credentials` Secret for AWS CLI access (adjust values as needed for your environment):

```bash
kubectl --context "${COMPUTE_CONTEXT}" create secret generic aws-credentials \
  --from-literal=AWS_ACCESS_KEY_ID=minioadmin \
  --from-literal=AWS_SECRET_ACCESS_KEY=minioadmin
```

### 2) Apply RBAC for Ray management in the compute cluster
```bash
kubectl --context "${COMPUTE_CONTEXT}" apply -f docs/batchjobs/Resources/rbac-ray.yaml
```

This creates `ServiceAccount ray-manager` and grants permissions on `rayclusters` and `rayjobs`.

### 3) Create a token Secret for the `ray-manager` ServiceAccount

**Production approach**: Create a Secret of type `kubernetes.io/service-account-token` that Kubernetes will automatically populate with a token. This creates a long-lived token that persists until the Secret is deleted.

```bash
cat <<EOF | kubectl --context "${COMPUTE_CONTEXT}" apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: ray-manager-token
  namespace: default
  annotations:
    kubernetes.io/service-account.name: ray-manager
type: kubernetes.io/service-account-token
EOF

# Wait for Kubernetes to populate the token (usually takes a few seconds)
kubectl --context "${COMPUTE_CONTEXT}" -n default wait --for=jsonpath='{.data.token}' --timeout=30s secret/ray-manager-token

# Extract the token
kubectl --context "${COMPUTE_CONTEXT}" -n default get secret ray-manager-token -o jsonpath='{.data.token}' | base64 -d > /tmp/${COMPUTE_CLUSTER}-token
```

**Note for testing/development only**: If you need a token with a specific expiration time for testing, you can use `kubectl create token`, but this is not recommended for production as it creates short-lived tokens (default 1 hour, max configurable duration varies by cluster):

```bash
# Testing only - not recommended for production
kubectl --context "${COMPUTE_CONTEXT}" -n default create token ray-manager --duration=87600h > /tmp/${COMPUTE_CLUSTER}-token
```

### 4) Extract CA data from the compute cluster
Extract the kubeconfig for the compute cluster and parse the certificate authority data.

```bash
# Get kubeconfig (adjust command based on your setup)
kubectl --context "${COMPUTE_CONTEXT}" config view --minify --raw > /tmp/${COMPUTE_CLUSTER}-kubeconfig

# Extract and decode CA data
# Parse clusters[0].cluster.certificate-authority-data from the kubeconfig
# Base64 decode and save as /tmp/${COMPUTE_CLUSTER}-cadata
```

Alternatively, if you have the kubeconfig file directly:
```bash
# Extract server URL and CA data from kubeconfig
kubectl config view --kubeconfig=/path/to/compute-cluster-kubeconfig --minify --raw > /tmp/${COMPUTE_CLUSTER}-kubeconfig
```

Parse `clusters[0].cluster.certificate-authority-data` from the kubeconfig (base64 decode) and save as `/tmp/${COMPUTE_CLUSTER}-cadata`.

### 5) Register the compute cluster in the control plane (Cluster CRD)
First, derive the API `host` and `port` from `clusters[0].cluster.server` in the compute kubeconfig (e.g., `https://compute-cluster.example.com:6443`).

Create the Cluster CRD in the control plane:
```bash
cat <<EOF | kubectl --context "${CONTROL_PLANE_CONTEXT}" apply -f -
apiVersion: michelangelo.api/v2
kind: Cluster
metadata:
  name: ${COMPUTE_CLUSTER}
  namespace: ma-system
spec:
  kubernetes:
    rest:
      host: https://compute-cluster.example.com   # replace with your compute cluster API server host
      port: "6443"                                # replace with your compute cluster API server port
      tokenTag: cluster-${COMPUTE_CLUSTER}-client-token
      caDataTag: cluster-${COMPUTE_CLUSTER}-ca-data
    skus: []
EOF
```

### 6) Create Secrets in the control plane for CA and token
```bash
# CA secret with key 'cadata'
kubectl --context "${CONTROL_PLANE_CONTEXT}" apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: cluster-${COMPUTE_CLUSTER}-ca-data
  namespace: default
stringData:
  cadata: "$(cat /tmp/${COMPUTE_CLUSTER}-cadata)"
EOF

# Token secret with key 'token'
kubectl --context "${CONTROL_PLANE_CONTEXT}" apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: cluster-${COMPUTE_CLUSTER}-client-token
  namespace: default
stringData:
  token: "$(cat /tmp/${COMPUTE_CLUSTER}-token)"
EOF
```

### Verification
- RBAC present:
  - `kubectl --context ${COMPUTE_CONTEXT} -n default get sa ray-manager`
- Token Secret present and populated:
  - `kubectl --context ${COMPUTE_CONTEXT} -n default get secret ray-manager-token`
  - Verify the Secret has a `token` key: `kubectl --context ${COMPUTE_CONTEXT} -n default get secret ray-manager-token -o jsonpath='{.data.token}' | wc -c` (should be > 0)
- Storage configuration present:
  - `kubectl --context ${COMPUTE_CONTEXT} get configmap michelangelo-config`
  - `kubectl --context ${COMPUTE_CONTEXT} get secret aws-credentials`
- Cluster registered in control plane:
  - `kubectl --context ${CONTROL_PLANE_CONTEXT} -n ma-system get clusters`
- Optional: Run a pipeline as in `docs/batchjobs/Run Uniflow Pipeline on Compute Cluster.md`.

### Troubleshooting
- **context mismatch**: Ensure you target `${COMPUTE_CONTEXT}` vs `${CONTROL_PLANE_CONTEXT}` correctly.
- **network connectivity**: Control plane must be able to reach the compute cluster API server. Verify network connectivity and firewall rules.
- **token Secret not populated**: If the token Secret is not automatically populated, verify the ServiceAccount exists and check the Secret's annotations. The Secret should have `kubernetes.io/service-account.name: ray-manager` annotation.
- **API server access**: Ensure the host and port in the Cluster CRD match the actual compute cluster API server endpoint accessible from the control plane.
- **token rotation**: For production, implement a token rotation policy. The Secret-based token persists until the Secret is deleted, so plan for periodic rotation.

### Appendix

#### Ray RBAC Manifest

The RBAC manifest (`docs/batchjobs/Resources/rbac-ray.yaml`) should contain:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ray-manager
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
# ClusterRole for Spark/Ray jobs across namespaces; one federated client uses a single ServiceAccount secret to create jobs in multiple namespaces.
kind: ClusterRole
metadata:
  name: ray-manager
rules:
- apiGroups: ["ray.io"]
  resources: ["rayclusters", "rayjobs"]
  verbs: ["create","get","list","watch","update","patch","delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ray-manager-binding
subjects:
- kind: ServiceAccount
  name: ray-manager
  namespace: default
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ray-manager
```
