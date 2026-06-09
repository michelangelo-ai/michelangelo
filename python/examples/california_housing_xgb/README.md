# California Housing XGBoost

Train workflow for California Housing price prediction using XGBoost.
Demonstrates the Michelangelo data pipeline: feature preparation, Spark
preprocessing, and distributed Ray training.

## Pipeline

```
feature_prep  →  preprocess  →  train
   (Ray)           (Spark)       (Ray)
```

| Step | File | Runtime | Description |
|---|---|---|---|
| `feature_prep` | `feature_prep.py` | Ray | Load dataset, train/test split, Ray Datasets |
| `preprocess` | `preprocess.py` | Spark | Cast columns to float |
| `train` | `train.py` | Ray | Distributed XGBoost training |

The workflow is orchestrated in `california_housing_xgb.py`, which imports each step from its own module.

## Dataset

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
    ...
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

## Requirements

- Python 3.9+
- Java 17 with `JAVA_HOME` set — required for Spark. Java 21 is incompatible with PySpark 3.5 + Hadoop 3.3 (`getSubject is not supported`). On macOS: `brew install openjdk@17` then `export JAVA_HOME=$(brew --prefix openjdk@17)/libexec/openjdk.jdk/Contents/Home`
- Ray and PySpark installed via the project venv

## Local Run

```bash
cd michelangelo-ai/michelangelo/python
PYTHONPATH=. poetry run python examples/california_housing_xgb/california_housing_xgb.py
```

## Remote Run

Pass environment variables via `--environ` flags — they are serialized into the
Cadence/Temporal workflow and injected into every Ray task's `runtime_env`,
reaching the remote workers. Shell `export` statements before the command only
affect the local launcher and do not propagate.

```bash
cd michelangelo-ai/michelangelo/python
PYTHONPATH=. poetry run python examples/california_housing_xgb/california_housing_xgb.py \
  remote-run \
  --image docker.io/library/my-workflow:latest \
  --storage-url s3://my-bucket/workflows \
  --yes
```

> **Sandbox values:** UniFlow's internal checkpoint storage (`--storage-url`) uses
> MinIO at `s3://michelangelo` with endpoint `minio:9091`. From the host, use
> `localhost:30009` via its NodePort (set during `ma sandbox start`; check your
> k3d port mapping for custom installations).

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
```
