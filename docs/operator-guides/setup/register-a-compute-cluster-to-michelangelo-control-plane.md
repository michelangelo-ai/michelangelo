# Register a Compute Cluster

Register an existing compute Kubernetes cluster with the Michelangelo AI control plane to enable running Ray jobs.

### Prerequisites
- Existing Kubernetes compute cluster accessible via kubectl
- The `michelangelo-compute` chart installed in the compute cluster, as described in [Install the Compute Cluster Chart](install-compute-cluster-charts.md). It installs KubeRay, the `ray-manager` ServiceAccount and its token, and the runtime objects every job reads. A cluster set up with an earlier version of this guide moves to the chart as described in [Migrating existing clusters](install-compute-cluster-charts.md#migrating-existing-clusters).
- Michelangelo AI control plane running, on the release that ships the `michelangelo-compute` chart or a later one
- Network connectivity between control plane and compute cluster (control plane must be able to reach compute cluster API server)

### What gets configured
- Cluster CRD in control plane (`ma-system`) pointing to the compute cluster
- Secrets in control plane for compute cluster CA and client token

The compute cluster's side is already in place, in namespace `default`:
- ServiceAccount `ray-manager`, its Role, and its token Secret `ray-manager-token`
- `michelangelo-config` ConfigMap, rendered from the chart's `runtime.config`
- The object-storage credentials Secret named by controllermgr's `logPersistence.credentialsSecret`, from your secret management (or from the chart, with `runtime.objectStorageCredentials.create: true`)

```bash
# Example names
COMPUTE_CLUSTER=michelangelo-compute-0
COMPUTE_CONTEXT=my-compute-cluster-context  # Your kubectl context for the compute cluster
CONTROL_PLANE_CONTEXT=my-control-plane-context  # Your kubectl context for the control plane
```

### 1) Read the `ray-manager` token from the compute cluster

The chart renders the Secret `ray-manager-token` (type `kubernetes.io/service-account-token`) in `default`, and Kubernetes populates it with a token that does not expire. To rotate the token, see [Rotate the ray-manager token](install-compute-cluster-charts.md#rotate-the-ray-manager-token).

```bash
# Wait for Kubernetes to populate the token (usually takes a few seconds)
kubectl --context "${COMPUTE_CONTEXT}" -n default wait --for=jsonpath='{.data.token}' --timeout=30s secret/ray-manager-token

# Extract the token
kubectl --context "${COMPUTE_CONTEXT}" -n default get secret ray-manager-token -o jsonpath='{.data.token}' | base64 -d > /tmp/${COMPUTE_CLUSTER}-token
```

### 2) Extract CA data from the compute cluster
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

### 3) Register the compute cluster in the control plane (Cluster CRD)
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

### 4) Create Secrets in the control plane for CA and token
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
- Runtime contract in place: the credentials Secret sets both keys, and `AWS_ENDPOINT_URL` and `REGISTRY_ENDPOINT` are reachable from `default`:
  - `helm test michelangelo-compute --kube-context ${COMPUTE_CONTEXT} --namespace michelangelo-compute`
  - If it fails, read the failed checks: `kubectl --context ${COMPUTE_CONTEXT} -n default logs michelangelo-compute-test-runtime-contract`
- Cluster registered in control plane:
  - `kubectl --context ${CONTROL_PLANE_CONTEXT} -n ma-system get clusters`
- Optional: Run a pipeline as in [Run a Pipeline on a Compute Cluster](../jobs/run-uniflow-pipeline-on-compute-cluster.md).

### Troubleshooting
- **context mismatch**: Ensure you target `${COMPUTE_CONTEXT}` vs `${CONTROL_PLANE_CONTEXT}` correctly.
- **network connectivity**: Control plane must be able to reach the compute cluster API server. Verify network connectivity and firewall rules.
- **token Secret not populated**: If the token Secret is not automatically populated, verify the ServiceAccount exists and check the Secret's annotations. The Secret should have `kubernetes.io/service-account.name: ray-manager` annotation.
- **API server access**: Ensure the host and port in the Cluster CRD match the actual compute cluster API server endpoint accessible from the control plane.
- **token rotation**: The token does not expire. To rotate it, see [Rotate the ray-manager token](install-compute-cluster-charts.md#rotate-the-ray-manager-token).
- **`helm test` fails**: See [Ray pods not starting on the compute cluster](../operations/troubleshooting.md#ray-pods-not-starting-on-the-compute-cluster).
