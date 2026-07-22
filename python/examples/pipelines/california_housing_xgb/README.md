# California Housing XGBoost

End-to-end ML pipeline for California Housing price prediction using XGBoost.
Demonstrates the full Michelangelo AI workflow: feature preparation, Spark
preprocessing, distributed Ray training, and a pusher step that exports the
model, evaluation report, and preprocessed datasets to storage and registry.

## Pipeline

```
feature_prep  →  preprocess  →  train  →  push_step
   (Ray)           (Spark)       (Ray)      (Spark)
```

| Step | File | Runtime | Description |
|---|---|---|---|
| `feature_prep` | `feature_prep.py` | Ray | Load dataset, train/test split, Ray Datasets |
| `preprocess` | `preprocess.py` | Spark | Cast columns to float |
| `train` | `train.py` | Ray | Distributed XGBoost training |
| `push_step` | `push.py` | Spark | Push model, eval report, and preprocessed datasets to storage/registry |

The workflow is orchestrated in `california_housing_xgb.py`, which imports each step from its own module.

## Prerequisites

- A Michelangelo AI sandbox running (`ma sandbox create`)
- A project created: `ma project apply -f examples/config/project.yaml`
- Python 3.9+
- Java 17 with `JAVA_HOME` set — required for Spark. Java 21 is incompatible with PySpark 3.5 + Hadoop 3.3 (`getSubject is not supported`). On macOS: `brew install openjdk@17` then `export JAVA_HOME=$(brew --prefix openjdk@17)/libexec/openjdk.jdk/Contents/Home`
- Ray and PySpark installed via the project venv

Uses the [California Housing dataset](https://scikit-learn.org/stable/datasets/real_world.html#california-housing-dataset) from scikit-learn — 20,640 samples with 8 features (median income, house age, average rooms/bedrooms, population, average occupancy, latitude, longitude) and median house value as the regression target.

## How It Works

### UniFlow decorators

Each step is a plain Python function decorated with `@uniflow.task`. The decorator
registers the function with a runtime config (`RayTask` or `SparkTask`) and
optionally enables caching:

```python
@uniflow.task(
    config=RayTask(head_cpu=1, worker_cpu=1, worker_instances=0),
    cache_enabled=True,   # skip re-execution when inputs are unchanged
)
def feature_prep(columns: list[str], ...) -> tuple[DatasetVariable, DatasetVariable]:
    ...
```

`@uniflow.workflow` composes tasks into a DAG that UniFlow transpiles to
Starlark for deterministic execution on Cadence/Temporal:

```python
@uniflow.workflow()
def train_workflow(dataset_cols: str = ...):
    train_dv, validation_dv = feature_prep(columns=...)
    pr = preprocess(train_dv=train_dv, ...)
    train_result = train(pr, params={...})
    return push_step(pr, train_result)
```

To override resources for a specific run without changing the task definition,
use `.with_overrides()`:

```python
feature_prep_overrides = feature_prep.with_overrides(
    alias="feature_prep_overrides",
    config=RayTask(head_cpu=2, worker_instances=1),
)
train_dv, validation_dv = feature_prep_overrides(columns=_dataset_cols)
```

### DatasetVariable — passing data between tasks

Tasks pass large datasets by **reference**, not by value. `DatasetVariable` is a
lightweight handle pointing to a Ray or Spark dataset stored in shared storage.
When a task returns a `DatasetVariable`, only the storage URI is serialized and
forwarded to the next task — the actual data never moves through the workflow
engine.

Inside a task, call the appropriate load method to materialize the data:

```python
train_dv.load_ray_dataset()
train_data: ray.data.Dataset = train_dv.value   # now a real Ray Dataset

validation_dv.load_spark_dataframe()
df: DataFrame = validation_dv.value             # now a real Spark DataFrame
```

> **UniFlow codec constraint:** stateful objects (storage clients, gRPC
> channels) cannot be serialized across the workflow→task boundary — passing a
> live client as a task argument would raise a codec error at runtime.

### push_step — single pusher for all artifacts

`push_step` receives both `PreprocessResult` (for the datasets) and `TrainResult`
(for the model checkpoint) and pushes four artifacts in one Spark task:

| Artifact | Plugin | Sink |
|---|---|---|
| `model` | `ModelPusherPlugin` | `StorageBackend` (MinIO or local) + registry |
| `eval_report` | `EvalReportPusherPlugin` | `StorageBackend` (MinIO or local) + registry |
| `train_data` | `DatasetPusherPlugin` | `S3Sink` → MinIO (remote) / `LocalFileSink` → temp dir (local/CI) |
| `validation_data` | `DatasetPusherPlugin` | `S3Sink` → MinIO (remote) / `LocalFileSink` → temp dir (local/CI) |

Datasets are loaded as pandas DataFrames and serialized to Parquet before upload.

The model and eval report storage backend is selected at runtime:
`AWS_ENDPOINT_URL` set → `MinioStorageBackend` (remote);
absent → `LocalStorageBackend` writing to a temp directory (local / CI).

To use a different storage backend (GCS, Azure Blob, HDFS), subclass `StorageBackend`:

```python
from michelangelo.lib.artifact_manager.storage_backend import StorageBackend

class GCSStorageBackend(StorageBackend):
    def upload(self, local_path: str, destination_key: str) -> str:
        # Upload to GCS and return the gs:// URI
        ...
    def download(self, uri: str, local_path: str) -> None:
        # Download from GCS to local_path
        ...
```

## Local Run

```bash
cd michelangelo-ai/michelangelo/python
JAVA_HOME=/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home \
PYTHONPATH=. poetry run python examples/pipelines/california_housing_xgb/california_housing_xgb.py
```

Without `AWS_ENDPOINT_URL`, `push_step` uses `LocalStorageBackend` for all artifacts
and writes datasets as Parquet to a temporary directory (no external services required).

## Remote Run

Pass environment variables via `--environ` flags — they are serialized into the
Cadence/Temporal workflow and injected into every task's runtime environment,
reaching remote workers. Shell `export` statements before the command only
affect the local launcher and do not propagate.

```bash
cd michelangelo-ai/michelangelo/python
JAVA_HOME=/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home \
PYTHONPATH=. poetry run python examples/pipelines/california_housing_xgb/california_housing_xgb.py \
  remote-run \
  --image docker.io/library/my-workflow:latest \
  --storage-url s3://your-bucket/workflows \
  --environ AWS_ENDPOINT_URL=http://your-minio-endpoint:9000 \
  --environ AWS_ACCESS_KEY_ID=your-access-key \
  --environ AWS_SECRET_ACCESS_KEY=your-secret-key \
  --environ REGISTRY_ENDPOINT=your-apiserver-host:15566 \
  --yes
```

### k3d sandbox

```bash
cd michelangelo-ai/michelangelo/python
JAVA_HOME=/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home \
PYTHONPATH=. poetry run python examples/pipelines/california_housing_xgb/california_housing_xgb.py \
  remote-run \
  --image docker.io/library/my-workflow:latest \
  --storage-url s3://michelangelo/workflows \
  --environ AWS_ENDPOINT_URL=http://minio:9091 \
  --environ AWS_ACCESS_KEY_ID=minioadmin \
  --environ AWS_SECRET_ACCESS_KEY=minioadmin \
  --environ REGISTRY_ENDPOINT=michelangelo-apiserver:15566 \
  --yes
```

Before running, rebuild and import the image into the cluster:

```bash
docker build -t my-workflow:latest -f examples/Dockerfile .
k3d image import my-workflow:latest -c michelangelo-sandbox
kubectl delete cachedoutputs --all   # clear stale cached task outputs
```

### Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `AWS_ENDPOINT_URL` | No | — | S3-compatible endpoint URL (include scheme, e.g. `http://minio:9091`). Unset → local storage |
| `AWS_ACCESS_KEY_ID` | If `AWS_ENDPOINT_URL` set | — | Access key ID |
| `AWS_SECRET_ACCESS_KEY` | If `AWS_ENDPOINT_URL` set | — | Secret access key |
| `AWS_S3_BUCKET` | No | Parsed from `MA_FILE_SYSTEM` or `UF_STORAGE_URL` | Target bucket name |
| `REGISTRY_ENDPOINT` | No | — | Model registry gRPC endpoint (`host:port`). Unset → in-memory only |
| `REGISTRY_INSECURE` | No | `true` | Set `false` to enable TLS for the registry connection |
| `REGISTRY_NAMESPACE` | No | `MA_NAMESPACE` (the pipeline's own namespace), else `default` | Model registry namespace |

> **Sandbox note:** in a k3d sandbox, `AWS_ENDPOINT_URL`, `AWS_ACCESS_KEY_ID`,
> and `AWS_SECRET_ACCESS_KEY` are automatically injected into Ray/Spark pods
> via the `michelangelo-config` ConfigMap — no `--environ` flags needed for
> `ma pipeline run`. For `remote-run`, pass them explicitly with `--environ`.

With `AWS_ENDPOINT_URL` set, `push_step` uploads all artifacts to MinIO/S3:

```
s3://your-bucket/models/california-housing-xgb/<push-id>/raw   ← model checkpoint
s3://your-bucket/datasets/california-housing/<run-id>/train/data.parquet
s3://your-bucket/datasets/california-housing/<run-id>/validation/data.parquet
```

Each run writes to a unique path derived from the Ray training run ID — concurrent
runs never overwrite each other.

> **Sandbox MinIO NodePort:** MinIO is exposed at `localhost:30007` from the host
> (for browsing via the MinIO console at `localhost:30008`). Tasks inside the cluster
> reach it at `minio:9091` (the in-cluster service address).

## Debugging

To pause a specific task and attach a debugger, pass `breakpoint=True` via
`with_overrides()` in the workflow:

```python
feature_prep_overrides = feature_prep.with_overrides(
    alias="feature_prep_debug",
    config=RayTask(head_cpu=1, breakpoint=True),
)
```

UniFlow halts the workflow before the task runs and waits for you to attach.
The same pattern works for any `@uniflow.task` in the pipeline.

## Expected Output

```
INFO     feature_prep  Train dataset schema: Schema(MedInc: double, HouseAge: double, ...)
INFO     feature_prep  Train dataset sample: [{'MedInc': 8.3252, 'HouseAge': 41.0, ...}]
INFO     preprocess    Processed Train Spark schema:
                       root
                        |-- MedInc: float (nullable = true)
                        |-- HouseAge: float (nullable = true)
                        ...
INFO     train         scaling_config: ScalingConfig(num_workers=1, ...)
INFO     train         run_config: RunConfig(storage_path='s3://.../ray_results')
INFO     train         TrainResult(path='.../ray_results/...', metrics={'validation-rmse': 0.876})
INFO     push_step     push_step: using LocalStorageBackend (local/CI) → /tmp/california_push_...
INFO     push_step     Found model checkpoint: /tmp/checkpoint_.../model.ubj
INFO     push_step     push model (model_plugin): success=True value=... error=None
INFO     push_step     push eval_report (eval_report_plugin): success=True value=... error=None
INFO     push_step     push train_data (dataset_plugin): success=True value=... error=None
INFO     push_step     push validation_data (dataset_plugin): success=True value=... error=None
```
