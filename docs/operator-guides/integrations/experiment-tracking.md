# Experiment Tracking Integration

This guide explains how platform operators can make an experiment tracking server available to Michelangelo workloads. It covers network setup, configuration injection, and the boundary between what operators configure and what users do in their `@uniflow.task()` code.

Michelangelo does not bundle an experiment tracking server. If your organization runs one — such as a self-hosted tracking server or a managed SaaS endpoint — this guide explains how to expose it to task pods running inside Michelangelo's compute clusters.

---

## How Experiment Tracking Works with Uniflow Tasks

Experiment tracking in Michelangelo follows a clear separation of concerns:

- **Operators** configure network access and make the tracking server URI available to task pods via environment variables or ConfigMaps.
- **Users** call their tracking server's client library inside `@uniflow.task()` functions. Michelangelo does not intercept or wrap these calls.

```
┌─────────────────────────────────────────────┐
│ Operator Responsibility                     │
│ ├─ Deploy or configure tracking server      │
│ ├─ Ensure network reachability from pods    │
│ └─ Inject URI via env var or ConfigMap      │
└─────────────────────────────────────────────┘
                     ↓
┌─────────────────────────────────────────────┐
│ User Responsibility (task code)             │
│ ├─ Import their tracking client library     │
│ ├─ Read URI from environment variable       │
│ └─ Log metrics, params, artifacts           │
└─────────────────────────────────────────────┘
```

---

## Prerequisites

- A running experiment tracking server accessible from your Kubernetes cluster.
- The server URI (e.g., `http://tracking.internal:5000` or `https://tracking.your-domain.com`).
- Sufficient RBAC to create ConfigMaps and patch namespace-scoped resources.

---

## Step 1: Verify Network Reachability

Task pods run inside the compute cluster namespace registered with Michelangelo (see [Register a Compute Cluster](../jobs/register-a-compute-cluster-to-michelangelo-control-plane.md)). Confirm that pods in that namespace can reach your tracking server.

```bash
# Run a connectivity test from a pod in the compute namespace
kubectl run connectivity-test \
  --image=curlimages/curl \
  --namespace=<compute-namespace> \
  --restart=Never \
  --rm -it -- \
  curl -sv http://tracking.internal:5000/health
```

If the tracking server is outside the cluster (e.g., a SaaS endpoint), verify that egress is allowed — check NetworkPolicy rules and any cluster-level egress controls.

If you need to create an explicit NetworkPolicy to allow egress from task pods:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-tracking-server-egress
  namespace: <compute-namespace>
spec:
  podSelector:
    matchLabels:
      # Label applied to Uniflow task pods — adjust to match your cluster
      app.kubernetes.io/managed-by: michelangelo
  policyTypes:
    - Egress
  egress:
    - to:
        - ipBlock:
            cidr: <tracking-server-ip>/32
      ports:
        - protocol: TCP
          port: 5000
```

---

## Step 2: Add the Tracking URI to the `michelangelo-config` ConfigMap

Michelangelo injects environment variables into every task pod (Ray head, Ray workers, Spark drivers, and Spark executors) via the `michelangelo-config` ConfigMap. This ConfigMap is mounted as an `envFrom` source, so every key in it becomes an environment variable in the pod.

You created this ConfigMap when you [registered the compute cluster](../jobs/register-a-compute-cluster-to-michelangelo-control-plane.md). Add the tracking server URI as a new key:

```bash
kubectl patch configmap michelangelo-config \
  --namespace=<compute-namespace> \
  --type=merge \
  -p '{"data":{"TRACKING_URI":"http://tracking.internal:5000"}}'
```

Or, if you manage the ConfigMap declaratively, add the key to your existing manifest:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: michelangelo-config
  namespace: <compute-namespace>
data:
  # Existing keys — do not remove these
  MA_FILE_SYSTEM: s3://default
  MA_FILE_SYSTEM_S3_SCHEME: http
  AWS_ACCESS_KEY_ID: minioadmin
  AWS_SECRET_ACCESS_KEY: minioadmin
  AWS_ENDPOINT_URL: << MINIO STORAGE URL >>
  # Add your tracking URI
  TRACKING_URI: "http://tracking.internal:5000"
```

New task pods will pick up the updated ConfigMap automatically — no worker restart is needed. Already-running pods will not see the change until they are replaced.

---

## Step 3: Handle Credentials (If Required)

If your tracking server requires authentication, the simplest approach is to add the credential to `michelangelo-config` alongside the URI:

```bash
kubectl patch configmap michelangelo-config \
  --namespace=<compute-namespace> \
  --type=merge \
  -p '{"data":{"TRACKING_URI":"http://tracking.internal:5000","TRACKING_API_KEY":"<your-api-key>"}}'
```

Note that `michelangelo-config` is a ConfigMap, not a Secret — values are stored in plaintext. This is the same ConfigMap that holds AWS credentials for storage access. If your security requirements demand encrypted-at-rest credential storage, consider using [workload identity](https://kubernetes.io/docs/concepts/security/service-accounts/) (e.g., IRSA on AWS, Workload Identity on GKE) so that task pods authenticate to the tracking server via IAM roles rather than static keys.

**Never hardcode credentials in task code.**

---

## What Users Do (Task Code)

Once the operator has completed the steps above, users can access the tracking server from any `@uniflow.task()` function by reading the environment variable.

```python
import os
import michelangelo.uniflow.core as uniflow

@uniflow.task()
def train_model(train_data, config: dict):
    # Read tracking URI injected by the operator
    tracking_uri = os.environ.get("TRACKING_URI")

    # Users initialize their tracking client — Michelangelo does not do this
    import your_tracking_client as tracker
    tracker.set_tracking_uri(tracking_uri)

    with tracker.start_run(run_name="training"):
        tracker.log_params(config)

        model = _train(train_data, config)

        tracker.log_metric("accuracy", model.accuracy)
        tracker.log_artifact("model.pkl", model)

    return model
```

Users are responsible for:
- Installing the tracking client library in their task's Docker image.
- Initializing the client and managing run lifecycle inside the task function.
- Ensuring their library is compatible with the server version your organization runs.

---

## Multi-Cluster Environments

If you have registered multiple compute clusters with Michelangelo, ensure the tracking server URI is injected consistently across all clusters. Each cluster's compute namespace needs the ConfigMap and any required NetworkPolicy entries.

You can manage this with a Kustomize overlay per cluster:

```
overlays/
├── cluster-a/
│   └── michelangelo-config-patch.yaml   # cluster-A tracking URI
└── cluster-b/
    └── michelangelo-config-patch.yaml   # cluster-B tracking URI (can differ)
```

---

## Verification

After applying the configuration, verify that the environment variable is visible inside a task pod:

```bash
kubectl exec -it <task-pod-name> -n <compute-namespace> -- env | grep TRACKING_URI
```

You can also run a minimal test task that prints the variable:

```python
@uniflow.task()
def check_tracking_config():
    import os
    uri = os.environ.get("TRACKING_URI", "NOT SET")
    print(f"Tracking URI: {uri}")
    assert uri != "NOT SET", "TRACKING_URI environment variable is not set"
```

---

## Related

- [Register a Compute Cluster](../jobs/register-a-compute-cluster-to-michelangelo-control-plane.md)
- [Worker Configuration](../platform-setup.md#worker-configuration)
