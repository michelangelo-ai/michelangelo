# Register a Compute Cluster

Register an existing compute Kubernetes cluster with the Michelangelo AI control plane to enable running Ray jobs.

### Prerequisites
- Existing Kubernetes compute cluster accessible via kubectl
- KubeRay operator installed in the compute cluster (`ray-system` namespace)
- Michelangelo AI control plane running
- Access to object storage (S3/MinIO) used by the control plane
- RBAC manifest for service account with permissions to run Ray Jobs / Ray Clusters and Spark applications, and to read pods.
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
  AWS_ACCESS_KEY_ID: michelangeloadmin
  AWS_SECRET_ACCESS_KEY: michelangeloadmin
  AWS_ENDPOINT_URL: << MINIO STORAGE URL >>
EOF
```

Create the `aws-credentials` Secret for AWS CLI access (adjust values as needed for your environment):

```bash
kubectl --context "${COMPUTE_CONTEXT}" create secret generic aws-credentials \
  --from-literal=AWS_ACCESS_KEY_ID=michelangeloadmin \
  --from-literal=AWS_SECRET_ACCESS_KEY=michelangeloadmin
```

### 2) Apply RBAC for Ray and Spark management in the compute cluster

Apply the manifest from the Appendix below using your preferred method (e.g., `kubectl --context "${COMPUTE_CONTEXT}" apply -f -` with the manifest piped from a heredoc, or save it to a file first).

This creates `ServiceAccount ray-manager`, grants permissions on `rayclusters` and `rayjobs` and on the Spark Operator's `sparkapplications` (which the control plane creates, reads, deletes and watches for every SparkJob it runs on this cluster), and grants read-only access (`get`, `list`, `watch`) to `pods`. The control plane watches the compute cluster with this ServiceAccount's token, and the pod watch is what surfaces per-pod failures (`ImagePullBackOff`, `CrashLoopBackOff`, unschedulable pods) into the Michelangelo RayCluster status. Michelangelo never creates or deletes pods directly; KubeRay owns them. The manifest also grants `get` on Kueue's `localqueues`, which the [Kueue backend](../jobs/kueue-scheduler-backend.md) reads before it dispatches a job to a Kueue-managed cluster.

**Upgrading an existing installation**: re-apply this manifest on every registered compute cluster before upgrading the control plane's controllermgr. Earlier versions of the manifest did not grant access to `pods`; without it the pod watch is denied and retried forever, and per-pod failures never reach the RayCluster status. controllermgr reports no error of its own; the only trace is client-go's reflector logging `failed to list *v1.Pod: pods is forbidden` on every retry. Nor did they grant access to `sparkapplications` or `localqueues`. Without `sparkapplications`, every SparkJob dispatched to the cluster fails at once: the create is refused, an authorization failure is not retried, and the SparkJob records `Succeeded=False` with reason `SparkApplicationCreationFailed`. Without `localqueues`, the [Kueue backend](../jobs/kueue-scheduler-backend.md) cannot check a job's LocalQueue on a Kueue-managed cluster, so it dispatches no job there: each one is held with `Enqueued=False` and reason `KueueQueueCheckFailed`, and checked again later.

**Sharing a compute cluster between control planes**: each control plane labels the Ray and Spark objects it creates with `ma/control-plane-env`, taken from the `RUNTIME_ENVIRONMENT` environment variable of its controllermgr, and only watches objects that carry its own value. If more than one control plane (for example staging and production) uses this compute cluster, set a distinct, non-empty `RUNTIME_ENVIRONMENT` on each one's controllermgr. Left unset, they all use the empty value, see each other's objects, and can write status onto each other's RayClusters and RayJobs that have the same project and name. SparkJobs are not exposed to that: a SparkJob takes status only from an application labelled with its own UID (`ma/job-uid`). Whatever the value, though, every SparkApplication lands in the compute cluster's `default` namespace under its SparkJob's name, so two SparkJobs with the same name collide there even when they come from different control planes (see [What the control plane creates](../jobs/kueue-scheduler-backend.md#what-the-control-plane-creates)). The value must be a valid Kubernetes label value (at most 63 characters: letters, digits, `-`, `_` and `.`, starting and ending with a letter or digit), or controllermgr fails at startup.

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
  - `kubectl --context ${COMPUTE_CONTEXT} auth can-i watch pods --as=system:serviceaccount:default:ray-manager` (should print `yes`)
- Token Secret present and populated:
  - `kubectl --context ${COMPUTE_CONTEXT} -n default get secret ray-manager-token`
  - Verify the Secret has a `token` key: `kubectl --context ${COMPUTE_CONTEXT} -n default get secret ray-manager-token -o jsonpath='{.data.token}' | wc -c` (should be > 0)
- Storage configuration present:
  - `kubectl --context ${COMPUTE_CONTEXT} get configmap michelangelo-config`
  - `kubectl --context ${COMPUTE_CONTEXT} get secret aws-credentials`
- Cluster registered in control plane:
  - `kubectl --context ${CONTROL_PLANE_CONTEXT} -n ma-system get clusters`
- Optional: Run a pipeline as in [Run a Pipeline on a Compute Cluster](../jobs/run-uniflow-pipeline-on-compute-cluster.md).

### Troubleshooting
- **context mismatch**: Ensure you target `${COMPUTE_CONTEXT}` vs `${CONTROL_PLANE_CONTEXT}` correctly.
- **network connectivity**: Control plane must be able to reach the compute cluster API server. Verify network connectivity and firewall rules.
- **token Secret not populated**: If the token Secret is not automatically populated, verify the ServiceAccount exists and check the Secret's annotations. The Secret should have `kubernetes.io/service-account.name: ray-manager` annotation.
- **API server access**: Ensure the host and port in the Cluster CRD match the actual compute cluster API server endpoint accessible from the control plane.
- **token rotation**: For production, implement a token rotation policy. The Secret-based token persists until the Secret is deleted, so plan for periodic rotation.

### Appendix

#### RBAC Manifest

The RBAC manifest below should be applied to the compute cluster:

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
# The federated watchers run against this cluster with this ServiceAccount's
# token. The RayCluster watch covers ray.io above; the pod watch is what surfaces
# per-pod failures (ImagePullBackOff, CrashLoopBackOff, unschedulable) into the
# Michelangelo RayCluster CR, and it needs read access to core pods. Read-only:
# Michelangelo never creates or deletes pods directly — KubeRay owns them.
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get","list","watch"]
# The federated client creates, reads and deletes the SparkApplication it
# projects for each SparkJob, and the SparkApplication watcher lists and watches
# them.
- apiGroups: ["sparkoperator.k8s.io"]
  resources: ["sparkapplications"]
  verbs: ["create","get","list","watch","delete"]
# With jobs.scheduler.backend kueue, the control plane checks that a job's
# LocalQueue exists on a Kueue-managed cluster before it dispatches the job
# there. Without this the check is refused, and no job is dispatched to the
# cluster.
- apiGroups: ["kueue.x-k8s.io"]
  resources: ["localqueues"]
  verbs: ["get"]
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
