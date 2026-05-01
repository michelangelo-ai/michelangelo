# MLflow Integration

This guide explains how platform operators can connect an [MLflow Tracking Server](https://mlflow.org/docs/latest/tracking.html) to Michelangelo workloads. MLflow overlaps with two Michelangelo capabilities — experiment tracking and the model registry — so this guide covers both, along with the boundary between what operators configure and what users do in their `@uniflow.task()` code.

Michelangelo does not bundle an MLflow server. This guide assumes you are running a self-hosted MLflow Tracking Server or a managed endpoint (such as Databricks Managed MLflow).

---

## How MLflow Works with Michelangelo

```
┌─────────────────────────────────────────────┐
│ Operator Responsibility                     │
│ ├─ Deploy or point to an MLflow server      │
│ ├─ Ensure network reachability from pods    │
│ └─ Inject MLFLOW_TRACKING_URI via ConfigMap │
└─────────────────────────────────────────────┘
                     ↓
┌─────────────────────────────────────────────┐
│ User Responsibility (task code)             │
│ ├─ Import mlflow inside @uniflow.task()     │
│ ├─ Read URI from environment variable       │
│ └─ Log runs, params, metrics, artifacts     │
└─────────────────────────────────────────────┘
```

Michelangelo does not intercept or wrap MLflow calls. Users call the MLflow client directly inside `@uniflow.task()` functions; Michelangelo provides the environment variable injection and network access.

---

## Prerequisites

- A running MLflow Tracking Server accessible from your Kubernetes cluster. Replace `http://mlflow.example.com:5000` in the examples below with your actual server address.
- Sufficient RBAC to create ConfigMaps and patch namespace-scoped resources in the compute cluster namespace.
- The `mlflow` Python package available in the task's Docker image (users add this to their `requirements.txt`).

---

## Step 1: Verify Network Reachability

Task pods run inside the compute cluster namespace registered with Michelangelo. Confirm that pods in that namespace can reach your MLflow server before proceeding.

```bash
kubectl run mlflow-connectivity-test \
  --image=curlimages/curl \
  --namespace=<compute-namespace> \
  --restart=Never \
  --rm -it -- \
  curl -sv http://mlflow.example.com:5000/health
```

A `200 OK` response confirms reachability. If the MLflow server is outside the cluster (for example, Databricks or a SaaS endpoint), also confirm egress is allowed by any NetworkPolicy rules on the namespace.

If you need to add an egress rule for task pods:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-mlflow-egress
  namespace: <compute-namespace>
spec:
  podSelector:
    matchLabels:
      <your-pod-selector-label>: <your-value>
  policyTypes:
    - Egress
  egress:
    # Allow DNS resolution
    - ports:
        - protocol: UDP
          port: 53
        - protocol: TCP
          port: 53
    # Allow egress to the MLflow server
    - to:
        - ipBlock:
            cidr: <mlflow-server-ip>/32
      ports:
        - protocol: TCP
          port: 5000
```

Replace `<your-pod-selector-label>` with labels that match your task pods. Check the actual labels with `kubectl get pods -n <compute-namespace> --show-labels`.

---

## Step 2: Inject the Tracking URI into the ConfigMap

Michelangelo injects the `michelangelo-config` ConfigMap as an `envFrom` source into every task pod. Adding a key here makes it available as an environment variable in all Ray and Spark pods dispatched by Michelangelo.

```bash
kubectl patch configmap michelangelo-config \
  --namespace=<compute-namespace> \
  --type=merge \
  -p '{"data":{"MLFLOW_TRACKING_URI":"http://mlflow.example.com:5000"}}'
```

Or add it to your existing declarative ConfigMap manifest:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: michelangelo-config
  namespace: <compute-namespace>
data:
  # Existing keys
  MA_FILE_SYSTEM: s3://default
  MA_FILE_SYSTEM_S3_SCHEME: http
  AWS_ACCESS_KEY_ID: <your-access-key-id>
  AWS_SECRET_ACCESS_KEY: <your-secret-access-key>
  AWS_ENDPOINT_URL: <your-storage-endpoint>
  # MLflow
  MLFLOW_TRACKING_URI: "http://mlflow.example.com:5000"
```

New pods pick up the change automatically. Already-running pods will not see the update until they are replaced.

:::tip
`MLFLOW_TRACKING_URI` is the environment variable that MLflow's Python client reads natively — no extra configuration is needed in user task code.
:::

---

## Step 3: Handle Authentication

### Self-hosted MLflow with basic auth

If your MLflow server requires HTTP basic authentication, add the credentials to the ConfigMap:

```bash
kubectl patch configmap michelangelo-config \
  --namespace=<compute-namespace> \
  --type=merge \
  -p '{"data":{
    "MLFLOW_TRACKING_URI":"http://mlflow.example.com:5000",
    "MLFLOW_TRACKING_USERNAME":"<username>",
    "MLFLOW_TRACKING_PASSWORD":"<password>"
  }}'
```

MLflow's client reads `MLFLOW_TRACKING_USERNAME` and `MLFLOW_TRACKING_PASSWORD` natively.

:::warning
`michelangelo-config` is a ConfigMap, not a Secret — values are stored in plaintext in etcd. For production environments, consider using [workload identity](https://kubernetes.io/docs/concepts/security/service-accounts/) (IRSA on AWS, Workload Identity on GKE) so that task pods authenticate to MLflow via IAM roles rather than static credentials.
:::

### Databricks Managed MLflow

If you are using Databricks Managed MLflow, set the following keys:

```bash
kubectl patch configmap michelangelo-config \
  --namespace=<compute-namespace> \
  --type=merge \
  -p '{"data":{
    "MLFLOW_TRACKING_URI":"databricks",
    "DATABRICKS_HOST":"https://<your-workspace>.azuredatabricks.net",
    "DATABRICKS_TOKEN":"<your-personal-access-token>"
  }}'
```

---

## What Users Do (Task Code)

Once the operator has completed the steps above, users can use MLflow from any `@uniflow.task()` function without any extra configuration — the MLflow client reads `MLFLOW_TRACKING_URI` from the environment automatically.

```python
import mlflow
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def train_model(train_data, config: dict):
    mlflow.set_experiment("fraud-detection")

    with mlflow.start_run(run_name="xgboost-baseline"):
        mlflow.log_params(config)

        model = _train(train_data, config)

        mlflow.log_metric("auc", model.auc)
        mlflow.log_metric("precision", model.precision)
        mlflow.sklearn.log_model(model, artifact_path="model")

    return model
```

Users are responsible for:
- Including `mlflow` in their task's Docker image (add to `requirements.txt` or the project Dockerfile).
- Starting and ending MLflow runs inside the task function.
- Ensuring their `mlflow` client version is compatible with the server version your organization runs. See the [MLflow compatibility matrix](https://mlflow.org/docs/latest/getting-started/index.html) for details.

---

## MLflow Model Registry vs Michelangelo Model Registry

MLflow includes its own model registry. Michelangelo also has a built-in model registry backed by a `Model` Kubernetes custom resource. The two are independent and can be used simultaneously.

| | MLflow Model Registry | Michelangelo Model Registry |
|---|---|---|
| Backed by | MLflow Tracking Server database | Kubernetes `Model` CRD + S3 |
| Queried via | MLflow client / MLflow UI | `kubectl get models` / `ma model get` |
| Integrates with serving | MLflow serving (`mlflow models serve`) | Michelangelo `InferenceServer` |
| Required for Michelangelo pipelines? | No | No |

**When to use MLflow's registry:** If your organization already uses MLflow for model governance, lineage, and stage transitions (Staging → Production), continue using it. Michelangelo does not require you to use its own registry.

**When to use Michelangelo's registry:** If you want models to be deployable via Michelangelo's `InferenceServer` (Triton, vLLM, etc.), register them in Michelangelo's registry using the `@uniflow.task()` model registration API. You can do this in addition to logging to MLflow.

**Using both:** Log experiments and register models to MLflow for lineage and governance, and separately register the deployable artifact to Michelangelo for serving. Both calls can live in the same task function.

---

## Verification

After applying the configuration, confirm the environment variable is visible inside a task pod:

```bash
kubectl exec -it <task-pod-name> -n <compute-namespace> -- env | grep MLFLOW
```

You can also verify end-to-end reachability from a task pod by running a connectivity check against the MLflow health endpoint:

```bash
kubectl exec -it <task-pod-name> -n <compute-namespace> -- \
  curl -sv http://mlflow.example.com:5000/health
```

A `200 OK` response confirms both the environment variable injection and network reachability are working correctly.

---

## Troubleshooting

| Symptom | Likely cause | Resolution |
|---|---|---|
| `MLFLOW_TRACKING_URI` not set in pod | ConfigMap patch not applied, or pod predates the patch | Verify with `kubectl get configmap michelangelo-config -n <compute-namespace> -o yaml`; restart pods if needed |
| `ConnectionRefusedError` or `requests.exceptions.ConnectionError` | MLflow server unreachable from pod | Re-run the connectivity test from Step 1; check NetworkPolicy and firewall rules |
| `RestException: PERMISSION_DENIED` | Credentials missing or incorrect | Verify `MLFLOW_TRACKING_USERNAME` / `MLFLOW_TRACKING_PASSWORD` are set; check MLflow server auth config |
| `mlflow: command not found` / `ModuleNotFoundError` | `mlflow` not in task's Docker image | Add `mlflow` to `requirements.txt` or the project Dockerfile |
| MLflow run logged but artifacts missing | Artifact store (S3/GCS) unreachable from pod | Confirm task pod has access to the artifact store configured in the MLflow server |
| `INVALID_PARAMETER_VALUE` on `log_model` | Client/server version mismatch | Pin `mlflow` to the same major version as the server |

---

## Next Steps

- [Experiment Tracking Integration](experiment-tracking.md) — general guide for connecting any experiment tracking server to Michelangelo
- [Model Registry Integration](model-registry.md) — Michelangelo's built-in model registry: storage configuration, RBAC, and serving integration
- [Register a Compute Cluster](../jobs/register-a-compute-cluster-to-michelangelo-control-plane.md) — how to add a Kubernetes cluster so Michelangelo can dispatch jobs to it
- [Platform Setup](../platform-setup.md) — full ConfigMap reference for all Michelangelo components
- [MLflow Documentation](https://mlflow.org/docs/latest/) — official MLflow docs for tracking, model registry, and deployment
