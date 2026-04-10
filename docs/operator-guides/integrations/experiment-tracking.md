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

## Step 2: Create a ConfigMap with the Tracking Server URI

Store the tracking server URI in a ConfigMap in the namespace where Michelangelo dispatches jobs. This makes the value easy to update without touching task code.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: experiment-tracking-config
  namespace: <compute-namespace>
data:
  tracking_uri: "http://tracking.internal:5000"
```

Apply it:

```bash
kubectl apply -f experiment-tracking-config.yaml
```

---

## Step 3: Inject the URI into Task Pods

Michelangelo propagates environment variables from the worker configuration to task pods. Add the tracking server URI to the worker ConfigMap so it is available as an environment variable inside every task pod.

In your `worker-configmap.yaml` overlay, add:

```yaml
worker:
  extraEnv:
    - name: TRACKING_URI
      valueFrom:
        configMapKeyRef:
          name: experiment-tracking-config
          key: tracking_uri
```

> **Note**: The exact field path for `extraEnv` depends on your Michelangelo version and worker configuration schema. Consult the [Worker Configuration reference](../platform-setup.md#worker-configuration) for the current field name.

If your worker does not yet support `extraEnv`, you can inject the variable directly as a literal value:

```yaml
worker:
  extraEnv:
    - name: TRACKING_URI
      value: "http://tracking.internal:5000"
```

After updating the ConfigMap, restart the worker deployment to apply:

```bash
kubectl rollout restart deployment/michelangelo-worker -n <michelangelo-namespace>
```

---

## Step 4: Handle Credentials (If Required)

If your tracking server requires authentication, store credentials in a Kubernetes Secret rather than the ConfigMap.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: tracking-server-credentials
  namespace: <compute-namespace>
type: Opaque
stringData:
  api_key: "<your-api-key>"
```

Then reference it in the worker `extraEnv`:

```yaml
worker:
  extraEnv:
    - name: TRACKING_URI
      valueFrom:
        configMapKeyRef:
          name: experiment-tracking-config
          key: tracking_uri
    - name: TRACKING_API_KEY
      valueFrom:
        secretKeyRef:
          name: tracking-server-credentials
          key: api_key
```

**Never hardcode credentials in ConfigMaps or task code.** Use Secrets and consider using an IAM role or workload identity where your tracking server supports it.

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
│   └── experiment-tracking-config.yaml   # cluster-A tracking URI
└── cluster-b/
    └── experiment-tracking-config.yaml   # cluster-B tracking URI (can differ)
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
- [Model Registry Integration](model-registry.md)
