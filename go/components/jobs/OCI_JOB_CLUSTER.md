# Adding an OCI OKE Cluster as a Job Cluster

This guide covers how to connect an Oracle Cloud Infrastructure (OCI) OKE cluster as a remote job execution cluster for the Michelangelo job controller. The control plane (controllermgr) remains on your local/sandbox cluster — only jobs run on OKE.

## Prerequisites

- OCI CLI installed (`brew install oci-cli`)
- An OCI account with access to an OKE cluster and a Bastion service
- SSH key pair (e.g. `~/.ssh/id_ed25519_github`)
- `kubectl` and `kubectx` installed
- The Michelangelo sandbox cluster running locally (`k3d-michelangelo-sandbox`)

---

## Part 1: OCI Authentication

### 1.1 Authenticate with OCI

```bash
oci session authenticate --profile-name=oci-ash
```

This opens a browser for SSO login and writes a security token to `~/.oci/`. Tokens expire after ~1 hour.

To verify:

```bash
oci iam region list --config-file ~/.oci/config --profile oci-ash --auth security_token
```

### 1.2 Re-authentication

When the token expires, re-run:

```bash
oci session authenticate --profile-name=oci-ash
```

---

## Part 2: OKE Kubeconfig via Bastion Tunnel

OKE private clusters expose their API endpoint on a private IP (e.g. `10.0.0.9:6443`). Access requires an SSH tunnel through an OCI Bastion service.

### 2.1 Create a Bastion Port-Forwarding Session

```bash
OCI="oci --config-file ~/.oci/config --profile oci-ash --auth security_token --region us-ashburn-1"

SESSION_JSON=$($OCI bastion session create-port-forwarding \
  --bastion-id <BASTION_OCID> \
  --ssh-public-key-file ~/.ssh/id_ed25519_github.pub \
  --target-private-ip 10.0.0.9 \
  --target-port 6443 \
  --session-ttl 10800 \
  --display-name "oke-tunnel-$(date +%Y%m%d-%H%M%S)")

SESSION_ID=$(echo "$SESSION_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['id'])")
```

Wait for the session to become ACTIVE:

```bash
$OCI bastion session get --session-id "$SESSION_ID" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['lifecycle-state'])"
# → ACTIVE
```

### 2.2 Start the SSH Tunnel

**Important:** Use `/usr/bin/ssh`, not the `ssh` command, as it may resolve to an internal wrapper that silently drops connections.

```bash
/usr/bin/ssh -i ~/.ssh/id_ed25519_github \
  -N \
  -L "6443:10.0.0.9:6443" \
  -p 22 \
  -o StrictHostKeyChecking=no \
  -o ServerAliveInterval=30 \
  -o ServerAliveCountMax=6 \
  -o ExitOnForwardFailure=yes \
  "${SESSION_ID}@host.bastion.us-ashburn-1.oci.oraclecloud.com" &
```

Verify the tunnel is up:

```bash
lsof -i :6443 -sTCP:LISTEN
```

### 2.3 Add OKE to kubeconfig

Generate and merge the kubeconfig:

```bash
oci ce cluster create-kubeconfig \
  --cluster-id <CLUSTER_OCID> \
  --file /tmp/oke-kubeconfig \
  --region us-ashburn-1 \
  --token-version 2.0.0 \
  --auth security_token \
  --config-file ~/.oci/config \
  --profile oci-ash
```

Rewrite the server to use the local tunnel endpoint and merge:

```bash
# Rewrite server address to local tunnel
sed -i '' 's|https://10.0.0.9:6443|https://127.0.0.1:6443|g' /tmp/oke-kubeconfig

# Rename context for clarity
# Edit /tmp/oke-kubeconfig: change context/cluster/user names to "oci-oke-dev"

KUBECONFIG=~/.kube/config:/tmp/oke-kubeconfig kubectl config view --flatten > /tmp/merged.yaml
mv /tmp/merged.yaml ~/.kube/config
```

Verify:

```bash
kubectl --context oci-oke-dev get nodes
```

### 2.4 Automate with oke_start.sh

A script at `~/oke_start.sh` automates the full flow: token check → session creation → tunnel start → kubectl verification. Run it any time the tunnel is down:

```bash
~/oke_start.sh
```

Key behaviors:
- If a working tunnel already exists, exits immediately
- If the OCI token is expired, prints re-auth instructions and exits
- If a stale tunnel is on port 6443 (session expired), kills it and creates a new one
- Uses `/usr/bin/ssh` explicitly to avoid internal SSH wrappers
- Session TTL is 3 hours; re-run the script to renew

---

## Part 3: Register OKE as a Job Cluster

The Michelangelo job controller discovers clusters via `Cluster` CRs in the `ma-system` namespace. Each cluster needs:
1. RBAC (ServiceAccount + ClusterRole + ClusterRoleBinding) on the job cluster
2. A long-lived SA token secret on the job cluster
3. Two secrets in the control plane (`default` namespace): CA cert + SA token
4. A `Cluster` CR in `ma-system`

### 3.1 Install KubeRay on the OKE Cluster

KubeRay must be installed on the job cluster to handle `RayCluster` CRDs:

```bash
helm repo add kuberay https://ray-project.github.io/kuberay-helm/
helm repo update
helm install kuberay-operator kuberay/kuberay-operator \
  --namespace ray-system \
  --create-namespace \
  --context oci-oke-dev
```

Verify:

```bash
kubectl --context oci-oke-dev get pods -n ray-system
# → kuberay-operator-... Running
```

### 3.2 Create RBAC on OKE

```bash
kubectl --context oci-oke-dev apply -f - <<'EOF'
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ray-manager
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ray-manager
rules:
- apiGroups: ["ray.io"]
  resources: ["rayclusters", "rayjobs", "rayservices"]
  verbs: ["*"]
- apiGroups: [""]
  resources: ["pods", "pods/log", "services", "configmaps", "events"]
  verbs: ["*"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets"]
  verbs: ["*"]
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ray-manager-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ray-manager
subjects:
- kind: ServiceAccount
  name: ray-manager
  namespace: default
---
apiVersion: v1
kind: Secret
metadata:
  name: ray-manager-token
  namespace: default
  annotations:
    kubernetes.io/service-account.name: ray-manager
type: kubernetes.io/service-account-token
EOF
```

### 3.3 Extract Credentials

```bash
# Wait for the token to be populated
sleep 3

TOKEN=$(kubectl --context oci-oke-dev get secret ray-manager-token -n default \
  -o jsonpath='{.data.token}')

CA_DATA=$(kubectl --context oci-oke-dev config view --raw --minify \
  -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')
```

### 3.4 Create Secrets in the Control Plane

```bash
CLUSTER_NAME="oci-oke-dev"  # must match the Cluster CR name below

kubectl --context k3d-michelangelo-sandbox apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: cluster-${CLUSTER_NAME}-ca-data
  namespace: default
type: Opaque
data:
  cadata: ${CA_DATA}
EOF

kubectl --context k3d-michelangelo-sandbox apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: cluster-${CLUSTER_NAME}-client-token
  namespace: default
type: Opaque
data:
  token: ${TOKEN}
EOF
```

### 3.5 Create the Cluster CR

```bash
kubectl --context k3d-michelangelo-sandbox apply -f - <<EOF
apiVersion: michelangelo.api/v2
kind: Cluster
metadata:
  name: oci-oke-dev
  namespace: ma-system
spec:
  kubernetes:
    rest:
      caDataTag: cluster-oci-oke-dev-ca-data
      tokenTag: cluster-oci-oke-dev-client-token
      host: "https://127.0.0.1"
      port: "6443"
    skus: []
EOF
```

> **Note:** The `host` is `127.0.0.1` because the controllermgr runs locally and reaches OKE through the SSH bastion tunnel on port 6443.

### 3.6 Verify Registration

Start the controllermgr (with the sandbox context active):

```bash
kubectl config use-context k3d-michelangelo-sandbox
CONFIG_DIR=go/cmd/controllermgr/config bazel run //go/cmd/controllermgr
```

Check that the cluster becomes Ready:

```bash
kubectl --context k3d-michelangelo-sandbox get cluster oci-oke-dev -n ma-system -o yaml
# status.statusConditions[0].status should be CONDITION_STATUS_TRUE
```

---

## Part 4: Submitting Jobs to OKE

### 4.1 Cluster Routing

Jobs are routed to a specific cluster via the `ma/affinity-cluster` label on the `RayCluster` CR. The scheduler reads this label through `BatchRayCluster.GetAffinity()`.

### 4.2 Using task.star

Pass `cluster="oci-oke-dev"` to `ray.task()`:

```python
load("//python/michelangelo/uniflow/plugins/ray:task.star", "ray")

my_task = ray.task(
    task_path = "my.module.train",
    cluster = "oci-oke-dev",
    head_cpu = 4,
    head_memory = "16Gi",
    worker_instances = 2,
)
```

This sets `metadata.labels["ma/affinity-cluster"] = "oci-oke-dev"` on the `RayCluster` CR, and the scheduler routes it to the OKE cluster.

Without `cluster=`, the scheduler falls back to the first available cluster.

### 4.3 Image Requirement

OKE nodes pull images from public or private registries — **local images do not work**. Use a publicly available image or push your image to a registry:

```python
# For testing, use the public Ray image:
# rayproject/ray:2.9.0

# For production, push to OCI Container Registry (OCIR) or Docker Hub
# and set IMAGE_PULL_POLICY=IfNotPresent
```

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `OCI security token has expired` | OCI session expired | `oci session authenticate --profile-name=oci-ash` |
| `Tunnel failed to start` | Wrong `ssh` binary (internal wrapper) | Use `/usr/bin/ssh` explicitly |
| `Tunnel is up` but `kubectl` fails | Bastion session expired (3h TTL) | Re-run `~/oke_start.sh` |
| `Cluster` CR has no status | controllermgr not running | Start controllermgr with sandbox context |
| `ErrImageNeverPull` on OKE pods | Local image not available on OKE nodes | Use a public registry image |
| `no matches for kind "RayCluster"` | Wrong kubectl context | `kubectl config use-context k3d-michelangelo-sandbox` before running controllermgr |