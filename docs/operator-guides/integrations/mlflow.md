# Using MLflow with Michelangelo

Michelangelo does not have a native MLflow integration — there are no Michelangelo config fields for MLflow and no built-in synchronization between the two systems. However, MLflow can be deployed alongside Michelangelo and called directly from user code running inside `@uniflow.task()` functions.

This guide covers what operators need to set up so that users can reach an MLflow tracking server from their Uniflow tasks.

## What Operators Need to Do

### 1. Deploy an MLflow tracking server

Deploy MLflow separately, outside of Michelangelo. The tracking server needs to be accessible from the Kubernetes worker pods that run Uniflow tasks:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mlflow
  namespace: mlflow
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mlflow
  template:
    metadata:
      labels:
        app: mlflow
    spec:
      containers:
      - name: mlflow
        image: ghcr.io/mlflow/mlflow:latest
        args:
        - mlflow
        - server
        - --backend-store-uri=postgresql://user:pass@postgres:5432/mlflow
        - --default-artifact-root=s3://your-bucket/mlflow
        - --host=0.0.0.0
        ports:
        - containerPort: 5000
---
apiVersion: v1
kind: Service
metadata:
  name: mlflow
  namespace: mlflow
spec:
  selector:
    app: mlflow
  ports:
  - port: 5000
```

Point `--default-artifact-root` at the same S3 bucket Michelangelo uses if you want model artifacts to live in one place.

### 2. Check network connectivity

Uniflow tasks run as pods in the Kubernetes cluster. Ensure pods in the namespace where jobs run can reach the MLflow service:

```bash
# Test connectivity from a pod in the jobs namespace
kubectl run connectivity-test --rm -it --restart=Never \
  --image=curlimages/curl -- \
  curl -s http://mlflow.mlflow.svc.cluster.local:5000/health
```

If you have `NetworkPolicy` resources that restrict egress, add a rule allowing pods in the jobs namespace to reach the MLflow service.

### 3. Make the tracking URI available to task pods

Users need `MLFLOW_TRACKING_URI` accessible in their task containers. The simplest approach is to document the URI so users can set it in their code or pipeline configuration.

## What Users Can Do

Once network connectivity is in place, users call the standard MLflow Python client from their `@uniflow.task()` functions. Michelangelo does not intercept or modify these calls:

```python
import mlflow
import michelangelo.uniflow.core as uniflow

@uniflow.task()
def train_model(data_path: str, learning_rate: float = 0.001):
    mlflow.set_tracking_uri("http://mlflow.mlflow.svc.cluster.local:5000")
    mlflow.set_experiment("my-training-experiment")

    with mlflow.start_run():
        mlflow.log_param("data_path", data_path)
        mlflow.log_param("learning_rate", learning_rate)

        # ... training code ...

        mlflow.log_metric("accuracy", accuracy)
        mlflow.log_metric("val_loss", loss)
        mlflow.pytorch.log_model(model, artifact_path="model")

    return model
```

## Troubleshooting

### MLflow server unreachable from task pods

```bash
# Identify which namespace jobs run in
kubectl get pods -A | grep ray

# Test connectivity from a pod in that namespace
kubectl -n <jobs-namespace> run test --rm -it --restart=Never \
  --image=curlimages/curl -- \
  curl -sv http://mlflow.mlflow.svc.cluster.local:5000/health
```

Check your `NetworkPolicy` resources if the connection is refused or times out.

### S3 artifact upload failures

If MLflow is configured to store artifacts in S3 and uploads fail from task pods, verify that the task pod's IAM role or service account has `s3:PutObject` on the MLflow artifact bucket/prefix. This is a separate permission from the one Michelangelo uses for its own artifact storage.

### MLflow experiment name conflicts

MLflow experiments are global to the tracking server. If multiple teams share one MLflow server, establish a naming convention: `<team>/<project>/<experiment>`, for example `ml-team/fraud-detection/v2-training`.
