# MLflow Integration

Michelangelo integrates with [MLflow](https://mlflow.org) for experiment tracking, model registry synchronization, and evaluation. This guide explains how to configure the integration and use it from your workflows.

## Prerequisites

- Michelangelo control plane deployed and running
- MLflow tracking server accessible from the Michelangelo worker pods (check network policies)
- MLflow version 2.x or later

## Configuration

Set the MLflow tracking server URI in the controller manager ConfigMap overlay:

```yaml
mlflow:
  trackingUri: http://mlflow.your-domain.com:5000
```

Apply the updated overlay and restart the controller manager:

```bash
kubectl rollout restart deployment/michelangelo-controllermgr -n ma-system
```

Verify the connection:

```bash
kubectl -n ma-system logs deployment/michelangelo-controllermgr | grep mlflow
```

### Using a Secured MLflow Server

If your MLflow server requires authentication, provide credentials via a Kubernetes Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mlflow-credentials
  namespace: ma-system
stringData:
  MLFLOW_TRACKING_USERNAME: your-username
  MLFLOW_TRACKING_PASSWORD: your-password
```

Reference it in the controller manager deployment via environment variables. Do not hardcode credentials in the ConfigMap.

## Experiment Tracking

Log parameters, metrics, and artifacts from any `@uniflow.task()` using the standard MLflow Python client. The tracking server URI is inherited from the pod environment — no additional configuration is needed in your task code.

```python
import mlflow
import michelangelo.uniflow.core as uniflow

@uniflow.task()
def train_model(data_path: str, learning_rate: float = 0.001):
    mlflow.set_experiment("my-training-experiment")

    with mlflow.start_run():
        mlflow.log_param("data_path", data_path)
        mlflow.log_param("learning_rate", learning_rate)

        # ... training code ...
        accuracy = evaluate(model, val_data)
        loss = compute_loss(model, val_data)

        mlflow.log_metric("accuracy", accuracy)
        mlflow.log_metric("val_loss", loss)

        # Log the model artifact
        mlflow.pytorch.log_model(model, artifact_path="model")

    return model
```

Each `PipelineRun` maps to one or more MLflow runs within the configured experiment. Use `mlflow.set_experiment()` to group runs by pipeline or use case.

### Tagging Runs with Pipeline Metadata

Link MLflow runs back to Michelangelo pipeline runs for traceability:

```python
import os
import mlflow
import michelangelo.uniflow.core as uniflow

@uniflow.task()
def train_model(data_path: str):
    with mlflow.start_run():
        # Tag with Michelangelo metadata available as environment variables
        mlflow.set_tag("michelangelo.pipeline", os.getenv("MA_PIPELINE_NAME", "unknown"))
        mlflow.set_tag("michelangelo.run_id", os.getenv("MA_PIPELINE_RUN_ID", "unknown"))

        # ... training and logging ...
```

## Model Registry Integration

Register a trained model to both the Michelangelo model registry and the MLflow Model Registry in a single workflow step.

```python
import mlflow
import michelangelo.uniflow.core as uniflow
from michelangelo.sdk.model import ModelRegistryClient

@uniflow.task()
def register_model(model, model_name: str, version_description: str):
    # 1. Log and register in MLflow
    with mlflow.start_run():
        model_info = mlflow.pytorch.log_model(
            model,
            artifact_path="model",
            registered_model_name=model_name,   # registers in MLflow Model Registry
        )

    # 2. Register in Michelangelo model registry
    # The S3 artifact path from MLflow and Michelangelo's storage are compatible
    # when both point to the same S3 bucket
    client = ModelRegistryClient()
    client.register(
        name=model_name,
        artifact_uri=model_info.model_uri,       # s3://your-bucket/mlflow/...
        description=version_description,
    )

    return model_info.model_uri
```

Both registries store model artifacts in S3-compatible storage. When `minio.awsEndpointUrl` in the controller manager ConfigMap matches the MLflow artifact store configuration, the same S3 paths are accessible from both registries.

## Evaluation Integration

Use MLflow's search API to compare outcomes across PipelineRuns for a given experiment:

```python
import mlflow

def compare_runs(experiment_name: str, metric: str = "accuracy", n_top: int = 5):
    """Retrieve the top N runs by a given metric for an experiment."""
    runs = mlflow.search_runs(
        experiment_names=[experiment_name],
        order_by=[f"metrics.{metric} DESC"],
        max_results=n_top,
    )
    return runs[["run_id", "params.learning_rate", f"metrics.{metric}",
                  "tags.michelangelo.pipeline", "tags.michelangelo.run_id"]]
```

This makes it straightforward to identify which PipelineRun configuration produced the best model before promoting to the Michelangelo model registry.

### Automated Evaluation Pipelines

Integrate evaluation into a Uniflow workflow:

```python
import mlflow
import michelangelo.uniflow.core as uniflow

@uniflow.task()
def evaluate_and_promote(model_name: str, experiment_name: str, accuracy_threshold: float = 0.90):
    """Promote a model only if it exceeds the accuracy threshold."""
    runs = mlflow.search_runs(
        experiment_names=[experiment_name],
        filter_string=f"metrics.accuracy > {accuracy_threshold}",
        order_by=["metrics.accuracy DESC"],
        max_results=1,
    )

    if runs.empty:
        raise ValueError(f"No runs met the accuracy threshold of {accuracy_threshold}")

    best_run_id = runs.iloc[0]["run_id"]
    model_uri = f"runs:/{best_run_id}/model"

    # Transition the best run's model to Production stage in MLflow
    client = mlflow.MlflowClient()
    latest_version = client.get_latest_versions(model_name, stages=["None"])[0]
    client.transition_model_version_stage(
        name=model_name,
        version=latest_version.version,
        stage="Production",
    )

    return model_uri

@uniflow.workflow()
def training_pipeline(data_path: str, model_name: str):
    model = train_model(data_path)
    _ = register_model(model, model_name, "Trained on latest dataset")
    best_uri = evaluate_and_promote(model_name, "my-training-experiment")
    return best_uri
```

## Troubleshooting

### MLflow server unreachable from worker pods

```bash
# Test connectivity from a worker pod
kubectl -n ma-system exec deployment/michelangelo-worker -- \
  curl -s http://mlflow.your-domain.com:5000/health
```

If the request times out, check your `NetworkPolicy` resources in `ma-system`. The worker pods must be able to reach the MLflow tracking server on port 5000 (or whichever port you configured).

### S3 artifact path mismatches

MLflow stores artifacts at paths like `s3://bucket/mlflow/<experiment_id>/<run_id>/artifacts/`. Michelangelo's storage root is configured separately via `minio.awsEndpointUrl`. If both are using the same S3 bucket and endpoint, paths are directly compatible. If they differ (e.g., different buckets or endpoints), set `MLFLOW_S3_ENDPOINT_URL` in your task pod environment to explicitly point MLflow at the correct S3 endpoint.

### Experiment name conflicts

MLflow experiments are global to the tracking server. Use a naming convention that includes the team or project name to avoid conflicts across teams sharing the same MLflow server: `<team>/<project>/<experiment>`, for example `ml-team/fraud-detection/v2-training`.
