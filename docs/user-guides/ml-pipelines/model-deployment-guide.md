# Model Deployment Guide

## What you'll learn

* How to package trained models for deployment
* How to register models in Michelangelo's model registry
* How to deploy models to inference servers
* How to serve predictions (batch and real-time)
* How to monitor and scale deployments
* Best practices for production ML

---

## Overview: From Model to Production

Michelangelo provides an end-to-end deployment workflow that takes your trained model from development to serving predictions at scale:

```
Train Model → Package Model → Register → Deploy → Serve Predictions → Monitor
```

Each step is designed for safety, reproducibility, and scalability in production.

---

## Step 1: Train and Export Your Model

First, you need a trained model. This typically happens in your training pipeline:

```python
from michelangelo.uniflow.core import task, workflow
from michelangelo.uniflow.plugins.ray import RayTask
import xgboost as xgb

@task(config=RayTask(head_cpu=4, head_memory="8Gi"))
def train_model(train_data):
    """
    Train your model and return the trained object
    Uniflow handles serialization automatically
    """
    model = xgb.XGBRegressor(
        max_depth=6,
        learning_rate=0.1,
        n_estimators=100
    )
    model.fit(train_data)
    return model

@task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def evaluate_model(model, test_data):
    """Evaluate model performance"""
    predictions = model.predict(test_data)
    mse = ((predictions - test_data.target) ** 2).mean()
    return {"mse": mse, "model": model}

@workflow()
def training_pipeline(train_data, test_data):
    model = train_model(train_data)
    evaluation = evaluate_model(model, test_data)
    return evaluation
```

Your trained model is now ready for the next step!

---

## Step 2: Package Your Model

Michelangelo packages models in **Triton-compatible format** for inference serving. This standardizes how models are deployed regardless of framework.

### What is Model Packaging?

Packaging converts your trained model into a standardized format that includes:
- **Model artifact** - The trained weights/coefficients
- **Model schema** - Input/output specifications
- **Metadata** - Version, framework, dependencies
- **Custom code** - Any preprocessing/postprocessing logic

### Simple Packaging Example

```python
from michelangelo.uniflow.core import task, workflow
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.sdk.model.packaging import CustomTritonPackager

@task(config=RayTask(...))
def package_model(model):
    """
    Package trained model for deployment
    Creates Triton-compatible model artifact
    """
    packager = CustomTritonPackager(
        model=model,
        model_name="my_model",
        model_version="1.0.0"
    )

    # Creates standardized model package
    packaged_model = packager.package()
    return packaged_model

@workflow()
def deployment_pipeline(model):
    packaged = package_model(model)
    return packaged
```

### What Gets Packaged

The packager creates:
- **model.pt** or **model.pkl** - Model artifact (weights)
- **config.pbtxt** - Triton configuration
- **schema.json** - Input/output schema
- **requirements.txt** - Dependencies (if needed)

This standardized format allows Triton inference server to serve your model without needing the original training code.

---

## Step 3: Register Your Model

Once packaged, register your model in Michelangelo's **Model Registry** for versioning and tracking:

### Manual Registration

```python
from michelangelo.sdk.model import ModelRegistry

@task(config=RayTask(...))
def register_model(packaged_model):
    """
    Register packaged model in Michelangelo registry
    Creates versioned model artifact
    """
    registry = ModelRegistry(
        namespace="my-project",
        name="recommendation-model"
    )

    model_version = registry.register(
        model_artifact=packaged_model,
        description="XGBoost recommender v1.0",
        metrics={
            "auc": 0.92,
            "precision": 0.88,
            "recall": 0.85
        },
        tags=["production", "xgboost"]
    )

    return model_version

@workflow()
def full_pipeline(model):
    packaged = package_model(model)
    registered = register_model(packaged)
    return registered
```

### What Gets Registered

The registry stores:
- **Model artifact** - Packaged model files
- **Version** - Semantic versioning (1.0.0, 1.1.0, etc.)
- **Metadata** - Description, tags, creation date
- **Metrics** - Performance metrics from evaluation
- **Schema** - Input/output specifications
- **Lineage** - Training job that created it

### Querying the Registry

```python
from michelangelo.sdk.model import ModelRegistry

# Get latest model version
registry = ModelRegistry(namespace="my-project", name="recommendation-model")
latest_model = registry.get_latest()

# Get specific version
v1_0_0 = registry.get_version("1.0.0")

# List all versions
all_versions = registry.list_versions()

# Find by tag
production_models = registry.find_by_tag("production")
```

---

## Step 4: Deploy Your Model

Once registered, deploy your model to a **Triton Inference Server** for serving predictions:

### Create Deployment Configuration

```yaml
# deployment.yaml
apiVersion: michelangelo.ai/v1
kind: Deployment
metadata:
  namespace: my-project
  name: recommendation-model-v1

spec:
  # Reference registered model
  model:
    namespace: my-project
    name: recommendation-model
    version: "1.0.0"

  # Inference server configuration
  inferenceServer:
    type: Triton
    replicas: 3  # Number of server instances
    resources:
      cpu: "2"
      memory: "4Gi"
      gpu: "1"  # Optional: GPU support

  # Serving configuration
  serving:
    batchSize: 32
    maxLatency: "100ms"
    timeoutMs: 5000

  # Auto-scaling (optional)
  autoscaling:
    minReplicas: 2
    maxReplicas: 10
    targetCPU: "70%"
```

### Deploy Using CLI

```bash
# Apply deployment configuration
ma deployment apply -f deployment.yaml

# Check deployment status
ma deployment describe --namespace my-project --name recommendation-model-v1

# View deployment logs
ma deployment logs --namespace my-project --name recommendation-model-v1

# Update deployment (e.g., new model version)
ma deployment update --namespace my-project --name recommendation-model-v1 \
  --model-version "1.1.0"
```

### Deploy Using Code

```python
from michelangelo.sdk.deployment import DeploymentManager

@task(config=RayTask(...))
def deploy_model(model_version):
    """Deploy model to production"""
    deployer = DeploymentManager(namespace="my-project")

    deployment = deployer.create_deployment(
        name="recommendation-model-v1",
        model_version=model_version,
        replicas=3,
        resources={
            "cpu": "2",
            "memory": "4Gi",
            "gpu": "1"
        },
        autoscaling={
            "min_replicas": 2,
            "max_replicas": 10,
            "target_cpu": "70%"
        }
    )

    return deployment

@workflow()
def full_pipeline(model):
    packaged = package_model(model)
    registered = register_model(packaged)
    deployed = deploy_model(registered)
    return deployed
```

---

## Step 5: Serve Predictions

Once deployed, your model is ready to serve predictions. Michelangelo supports both **batch** and **real-time** serving:

### Real-Time Predictions (REST API)

```bash
# Get prediction server endpoint
ma deployment describe --namespace my-project --name recommendation-model-v1
# Returns: http://inference.my-project.svc:8000

# Send prediction request
curl -X POST http://inference.my-project.svc:8000/predict \
  -H "Content-Type: application/json" \
  -d '{
    "instances": [
      {"user_id": 123, "product_id": 456, "context": "search"},
      {"user_id": 789, "product_id": 101, "context": "browse"}
    ]
  }'

# Response
{
  "predictions": [
    {"score": 0.92},
    {"score": 0.78}
  ]
}
```

### Real-Time Predictions (Python Client)

```python
from michelangelo.sdk.inference import InferenceClient

# Create client
client = InferenceClient(
    namespace="my-project",
    deployment="recommendation-model-v1"
)

# Make predictions
predictions = client.predict(
    instances=[
        {"user_id": 123, "product_id": 456, "context": "search"},
        {"user_id": 789, "product_id": 101, "context": "browse"}
    ]
)

print(predictions)
# [{"score": 0.92}, {"score": 0.78}]
```

### Batch Predictions

For large-scale prediction jobs, use batch inference:

```python
from michelangelo.uniflow.core import task, workflow
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.sdk.inference import BatchInferenceClient

@task(config=RayTask(head_cpu=4, worker_instances=4))
def batch_predict(data_path: str):
    """
    Run batch prediction on large dataset
    Uses deployed model for inference
    """
    client = BatchInferenceClient(
        namespace="my-project",
        deployment="recommendation-model-v1"
    )

    # Predict on entire dataset
    predictions = client.predict_batch(
        input_path=data_path,
        output_path="s3://my-bucket/predictions/",
        batch_size=100
    )

    return predictions

@workflow()
def batch_inference_pipeline(data_path: str):
    predictions = batch_predict(data_path)
    return predictions
```

---

## Step 6: Monitor and Scale

### Monitor Model Performance

```python
from michelangelo.sdk.monitoring import ModelMonitor

monitor = ModelMonitor(
    namespace="my-project",
    deployment="recommendation-model-v1"
)

# Get prediction statistics
stats = monitor.get_metrics(
    start_time="2026-03-01T00:00:00Z",
    end_time="2026-03-07T00:00:00Z"
)

print(f"Total predictions: {stats['total_predictions']}")
print(f"Average latency: {stats['avg_latency_ms']:.1f}ms")
print(f"Error rate: {stats['error_rate']:.2%}")
print(f"P99 latency: {stats['p99_latency_ms']:.1f}ms")
```

### Scale Deployments

```bash
# Manual scaling
ma deployment scale --namespace my-project \
  --name recommendation-model-v1 \
  --replicas 5

# Check auto-scaling status
ma deployment autoscaling-status --namespace my-project \
  --name recommendation-model-v1
# Shows: Current: 3 replicas, Min: 2, Max: 10, Target CPU: 70%
```

### A/B Testing (Canary Deployments)

Deploy a new model version alongside the old one:

```yaml
# deployment-canary.yaml
apiVersion: michelangelo.ai/v1
kind: Deployment
metadata:
  namespace: my-project
  name: recommendation-model-v2-canary

spec:
  model:
    namespace: my-project
    name: recommendation-model
    version: "1.1.0"  # New version

  serving:
    trafficSplit:
      - model: recommendation-model-v1
        weight: "90%"  # 90% to old version
      - model: recommendation-model-v2-canary
        weight: "10%"  # 10% to new version

  # Monitor both versions
  monitoring:
    compareMetrics: true
    alertOnRegression: true
```

Gradually increase traffic to new version once validated:

```bash
# Shift more traffic to new version
ma deployment update-traffic --namespace my-project \
  --name recommendation-model-v2-canary \
  --traffic-split '{v1: 50, v2: 50}'

# Fully promote new version
ma deployment promote --namespace my-project \
  --name recommendation-model-v2-canary
```

---

## Complete End-to-End Example

```python
from michelangelo.uniflow.core import task, workflow
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.sdk.model.packaging import CustomTritonPackager
from michelangelo.sdk.model import ModelRegistry
from michelangelo.sdk.deployment import DeploymentManager

@task(config=RayTask(head_cpu=4, head_memory="8Gi"))
def train_model(train_data):
    import xgboost as xgb
    model = xgb.XGBRegressor()
    model.fit(train_data)
    return model

@task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def evaluate_model(model, test_data):
    from sklearn.metrics import mean_squared_error
    predictions = model.predict(test_data)
    mse = mean_squared_error(test_data.target, predictions)
    return {"model": model, "mse": mse}

@task(config=RayTask(...))
def package_model(model):
    packager = CustomTritonPackager(
        model=model,
        model_name="price-predictor",
        model_version="1.0.0"
    )
    return packager.package()

@task(config=RayTask(...))
def register_model(packaged_model, metrics):
    registry = ModelRegistry(
        namespace="my-project",
        name="price-predictor"
    )
    return registry.register(
        model_artifact=packaged_model,
        description="XGBoost price predictor",
        metrics=metrics
    )

@task(config=RayTask(...))
def deploy_model(model_version):
    deployer = DeploymentManager(namespace="my-project")
    return deployer.create_deployment(
        name="price-predictor-v1",
        model_version=model_version,
        replicas=3
    )

@workflow()
def full_ml_lifecycle(train_data, test_data):
    """
    Complete ML lifecycle:
    Train → Evaluate → Package → Register → Deploy
    """
    model = train_model(train_data)
    evaluation = evaluate_model(model, test_data)
    packaged = package_model(evaluation["model"])
    registered = register_model(packaged, evaluation)
    deployed = deploy_model(registered)
    return deployed
```

---

## Best Practices for Deployment

### 1. Version Your Models

```python
# Always use semantic versioning
# MAJOR.MINOR.PATCH (1.0.0, 1.1.0, 2.0.0)

# Major: Breaking changes to model interface
# Minor: Improvements with backward compatibility
# Patch: Bug fixes
```

### 2. Test Before Production

```bash
# 1. Register model with test tag
ma model register --tag test

# 2. Deploy to staging
ma deployment apply -f staging-deployment.yaml

# 3. Run validation tests
ma deployment test --namespace staging --name model-v1

# 4. Only then promote to production
ma deployment promote --from staging --to production
```

### 3. Monitor Model Drift

```python
from michelangelo.sdk.monitoring import DriftDetector

detector = DriftDetector(namespace="my-project", deployment="model-v1")

# Check for data drift
drift_score = detector.detect_feature_drift(
    baseline_period="2026-02-01",
    current_period="2026-03-07"
)

if drift_score > 0.3:  # High drift
    print("WARNING: Feature distribution shifted significantly")
    # Consider retraining model
```

### 4. Set Up Alerts

```yaml
# alerts.yaml
apiVersion: michelangelo.ai/v1
kind: DeploymentAlert
metadata:
  namespace: my-project
  deployment: model-v1

spec:
  alerts:
    - name: high-latency
      condition: "p99_latency greater than 500ms"
      severity: warning
      action: notify-slack

    - name: prediction-errors
      condition: "error_rate greater than 0.01"
      severity: critical
      action: page-on-call

    - name: model-drift
      condition: "feature_drift_score greater than 0.3"
      severity: warning
      action: notify-ml-team
```

---

## Troubleshooting Deployment

### Issue: Model deployment fails with "Invalid schema"

**Cause**: Model input/output format doesn't match deployment configuration

**Solution**:
1. Verify packager created correct schema
2. Check deployment YAML input/output specs
3. Test with sample inputs before deploying

### Issue: High prediction latency in production

**Cause**: Under-provisioned replicas or inefficient model

**Solution**:
1. Increase number of replicas
2. Increase CPU/GPU allocation
3. Profile model inference time locally
4. Consider model quantization or distillation

### Issue: Predictions differ between local and deployed model

**Cause**: Version mismatch or model state inconsistency

**Solution**:
1. Verify deployed model version matches local
2. Check input preprocessing matches
3. Test with identical inputs
4. Ensure no data transformations differ

---

## Next Steps

* [Model Registry Guide](../model-registry-guide.md) - Deep dive into versioning and tracking
* [Reference System](./reference-system.md) - Understand how models are serialized
* [Type System](./type-system.md) - Learn about supported data types and codecs
* [Getting Started](./getting-started.md) - Complete guide to building your first pipeline
