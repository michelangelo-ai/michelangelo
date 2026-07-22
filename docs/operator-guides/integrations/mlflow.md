# MLflow

This guide explains how platform operators can connect an [MLflow Tracking Server](https://mlflow.org/docs/latest/tracking.html) to Michelangelo AI workloads. MLflow overlaps with two Michelangelo AI capabilities — experiment tracking and the model registry — so this guide covers both, along with the boundary between what operators configure and what users do in their `@uniflow.task()` code.

Michelangelo AI does not bundle an MLflow server. This guide assumes you are running a self-hosted MLflow Tracking Server or a managed endpoint (such as Databricks Managed MLflow).

> **Before you begin:** Complete [Experiment Tracking Setup](../experiment-tracking.md) — the platform-level guide for network reachability, ConfigMap injection, and auth. This MLflow guide builds on those foundations.

---

## How MLflow Works with Michelangelo AI

```text
┌─────────────────────────────────────────────┐
│ Operator Responsibility                     │
│ ├─ Deploy or point to an MLflow server      │
│ └─ Ensure network reachability from pods    │
└─────────────────────────────────────────────┘
                     ↓
┌─────────────────────────────────────────────┐
│ User Responsibility (task code)             │
│ ├─ Set MLFLOW_TRACKING_URI in workflow code │
│ ├─ Import mlflow inside @uniflow.task()     │
│ └─ Log runs, params, metrics, artifacts     │
└─────────────────────────────────────────────┘
```

Michelangelo AI does not intercept or wrap MLflow calls. Users call the MLflow client directly inside `@uniflow.task()` functions and configure the tracking URI themselves. The operator's job is to ensure the MLflow server is reachable from task pods.

---

## Prerequisites

- A running MLflow Tracking Server accessible from your Kubernetes cluster. Replace `http://mlflow.example.com:5000` in the examples below with your actual server address.
- Sufficient RBAC to create NetworkPolicy resources in the compute cluster namespace if egress rules are needed.
- The `mlflow` Python package available in the task's Docker image (users add this to their `requirements.txt`).

---

## Step 1: Verify Network Reachability

Task pods run inside the compute cluster namespace registered with Michelangelo AI. Confirm that pods in that namespace can reach your MLflow server before proceeding.

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

## Step 2: Configure the Tracking URI

`MLFLOW_TRACKING_URI` is a user-space configuration — it belongs in workflow code or the Ray job pod environment, not in the Michelangelo AI system ConfigMap. Users should set it themselves using one of these approaches.

### Option A: Set in workflow code

The simplest approach is to call `mlflow.set_tracking_uri()` directly in the task or at the top of the workflow module:

```python
import mlflow
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def train_model(train_data, config: dict):
    mlflow.set_tracking_uri("http://mlflow.example.com:5000")
    mlflow.set_experiment("fraud-detection")
    ...
```

### Option B: Set via pipeline environment

Users can pass `MLFLOW_TRACKING_URI` as an environment variable when submitting a pipeline run, keeping the URI out of source code:

```bash
ma pipeline dev-run -f pipeline.yaml --env MLFLOW_TRACKING_URI=http://mlflow.example.com:5000
```

In task code, MLflow reads `MLFLOW_TRACKING_URI` from the environment automatically — no explicit `set_tracking_uri()` call is needed when the variable is set.

---

## Step 3: Handle Authentication

### Self-hosted MLflow with basic auth

If your MLflow server requires HTTP basic authentication, pass the credentials as pipeline environment variables:

```bash
ma pipeline dev-run -f pipeline.yaml \
  --env MLFLOW_TRACKING_URI=http://mlflow.example.com:5000 \
  --env MLFLOW_TRACKING_USERNAME=<username> \
  --env MLFLOW_TRACKING_PASSWORD=<password>
```

MLflow's client reads `MLFLOW_TRACKING_USERNAME` and `MLFLOW_TRACKING_PASSWORD` natively.

:::warning
Avoid hardcoding credentials in source code or pipeline YAML files committed to version control. Pass them at runtime via `--env` or a secrets manager integrated with your CI/CD system.
:::

### Databricks Managed MLflow

If you are using Databricks Managed MLflow, pass the following environment variables at pipeline submission time:

```bash
ma pipeline dev-run -f pipeline.yaml \
  --env MLFLOW_TRACKING_URI=databricks \
  --env DATABRICKS_HOST=https://<your-workspace>.azuredatabricks.net \
  --env DATABRICKS_TOKEN=<your-personal-access-token>
```

---

## What Users Do (Task Code)

Once the operator has confirmed network reachability (Step 1), users configure their MLflow tracking URI and log experiments from any `@uniflow.task()` function.

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

## MLflow Model Registry vs Michelangelo AI Model Registry

MLflow includes its own model registry. Michelangelo AI also has a built-in model registry backed by a `Model` Kubernetes custom resource. The two are independent and can be used simultaneously.

| | MLflow Model Registry | Michelangelo AI Model Registry |
|---|---|---|
| Backed by | MLflow Tracking Server database | Kubernetes `Model` CRD + S3 |
| Queried via | MLflow client / MLflow UI | `kubectl get models` / `ma model get` |
| Integrates with serving | MLflow serving (`mlflow models serve`) | Michelangelo AI `InferenceServer` |
| Required for Michelangelo AI pipelines? | No | No |

**When to use MLflow's registry:** If your organization already uses MLflow for model governance, lineage, and stage transitions (Staging → Production), continue using it. Michelangelo AI does not require you to use its own registry.

**When to use Michelangelo AI's registry:** If you want models to be deployable via Michelangelo AI's `InferenceServer` (Triton, vLLM, etc.), register them in Michelangelo AI's registry using the `@uniflow.task()` model registration API. You can do this in addition to logging to MLflow.

**Using both:** Log experiments and register models to MLflow for lineage and governance, and separately register the deployable artifact to Michelangelo AI for serving. Both calls can live in the same task function.

---

## Verification

Verify network reachability from within the compute namespace using a temporary curl pod — the same approach as Step 1:

```bash
kubectl run mlflow-verify \
  --image=curlimages/curl \
  --namespace=<compute-namespace> \
  --restart=Never \
  --rm -it -- \
  curl -sv http://mlflow.example.com:5000/health
```

A `200 OK` response confirms task pods in that namespace can reach the MLflow server. The pod is automatically deleted after the check (`--rm`).

---

## Troubleshooting

| Symptom | Likely cause | Resolution |
|---|---|---|
| `ConnectionRefusedError` or `requests.exceptions.ConnectionError` | MLflow server unreachable from pod | Re-run the connectivity test from Step 1; check NetworkPolicy and firewall rules |
| `RestException: PERMISSION_DENIED` | Credentials missing or incorrect | Verify `MLFLOW_TRACKING_USERNAME` / `MLFLOW_TRACKING_PASSWORD` are set at pipeline submission time |
| `mlflow: command not found` / `ModuleNotFoundError` | `mlflow` not in task's Docker image | Add `mlflow` to `requirements.txt` or the project Dockerfile |
| MLflow run logged but artifacts missing | Artifact store (S3/GCS) unreachable from pod | Confirm task pod has access to the artifact store configured in the MLflow server |
| `INVALID_PARAMETER_VALUE` on `log_model` | Client/server version mismatch | Pin `mlflow` to the same major version as the server |

---

## Next Steps

- [Experiment Tracking Setup](../experiment-tracking.md) — platform-level setup guide: network reachability, ConfigMap injection, and auth patterns for any tracking server
- [Model Registry](../components/model-registry.md) — Michelangelo AI's built-in model registry: storage configuration, RBAC, and serving integration
- [Register a Compute Cluster](../setup/register-a-compute-cluster-to-michelangelo-control-plane.md) — how to add a Kubernetes cluster so Michelangelo AI can dispatch jobs to it
- [Platform Setup](../setup/platform-setup.md) — full ConfigMap reference for all Michelangelo AI components
- [MLflow Documentation](https://mlflow.org/docs/latest/) — official MLflow docs for tracking, model registry, and deployment
