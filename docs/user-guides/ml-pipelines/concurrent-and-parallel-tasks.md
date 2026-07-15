---
sidebar_position: 4
---

# Concurrent and Parallel Tasks

Uniflow workflows run tasks sequentially by default — each task call blocks until it returns. For tasks that can run independently (fetching data from multiple sources, calibrating multiple models, processing shards), the concurrent library lets you kick off several tasks at once and collect results later.

## What you'll learn

- How to run tasks concurrently and collect results with `Future`
- How to run many tasks in parallel with a bounded concurrency limit using `BatchFuture`
- How to express loop and fan-out patterns inside a workflow
- How to pass datasets between tasks of different runtimes using `DatasetVariable`

## Prerequisites

- A basic Uniflow workflow you want to speed up — see [Getting Started with ML Pipelines](../getting-started/getting-started.md)
- Familiarity with `@uniflow.task` and `@uniflow.workflow` — see the [ML Pipelines overview](./index.md)

## Concurrent run

`concurrent_run` kicks off a task call without blocking, and returns a `Future`. Collect results by calling `future.result()` — this blocks until the task finishes.

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.core.lib.concurrent import run as concurrent_run
from michelangelo.uniflow.plugins.ray import RayTask


@uniflow.task(config=RayTask(head_cpu=1, head_memory="4Gi"))
def load_shard(shard_url: str) -> list:
    import pandas as pd
    return pd.read_parquet(shard_url).to_dict(orient="records")


@uniflow.workflow()
def process_shards(shard_a_url: str, shard_b_url: str):
    # Kick off both loads concurrently — neither blocks the other
    future_a = concurrent_run(load_shard, shard_a_url)
    future_b = concurrent_run(load_shard, shard_b_url)

    # Collect results — blocks until each task finishes
    data_a = future_a.result()
    data_b = future_b.result()

    return data_a + data_b
```

**Key points:**
- `concurrent_run` returns immediately with a `Future` — the task runs in the background.
- `.result()` blocks until the task finishes and returns its output.
- In local execution, tasks run sequentially (no true parallelism). True concurrency is provided by the Cadence/Temporal execution engine in remote runs.

## Parallel / batch run

When you need to run many task calls with a cap on how many execute at the same time, use `new_callable` + `concurrent_batch_run`. This is the standard pattern for fan-out workloads (e.g., processing N data shards, calibrating N models).

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.core.lib.concurrent import (
    new_callable,
    batch_run as concurrent_batch_run,
)
from michelangelo.uniflow.plugins.ray import RayTask


@uniflow.task(config=RayTask(head_cpu=1, head_memory="4Gi"))
def calibrate_model(model_id: str, dataset_url: str) -> dict:
    """Run calibration for one model variant."""
    ...
    return {"model_id": model_id, "metric": 0.95}


@uniflow.workflow()
def calibrate_all(model_ids: list[str], dataset_url: str):
    # Build a list of deferred calls — nothing runs yet
    callables = [
        new_callable(calibrate_model, model_id, dataset_url)
        for model_id in model_ids
    ]

    # Run all callables with at most 2 executing concurrently
    batch_future = concurrent_batch_run(callables, max_concurrency=2)

    # Block until all finish; returns a list of results in submission order
    return batch_future.get()
```

**Key points:**
- `new_callable(fn, *args)` creates a deferred call — it does not execute `fn` yet.
- `concurrent_batch_run(callables, max_concurrency=N)` submits all callables and runs up to `N` at a time.
- `batch_future.get()` blocks until all callables finish and returns results as a list, in the same order as `callables`.
- Omit `max_concurrency` (or pass `None`) to run all callables simultaneously with no limit.

## Loop patterns

Because workflow code is standard Python, you can express any looping pattern directly — windowed batches, conditional fan-out, nested loops.

### Windowed parallel batches

Run tasks in windows of `N` at a time when you need finer-grained control than `max_concurrency`:

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.core.lib.concurrent import run as concurrent_run
from michelangelo.uniflow.plugins.ray import RayTask


@uniflow.task(config=RayTask(head_cpu=1, head_memory="4Gi"))
def process_query(query: str, datasource: str) -> dict:
    ...


@uniflow.workflow()
def run_queries_in_windows(queries: list[str], datasource: str, window_size: int = 2):
    results = []
    # Process `window_size` queries at a time
    for i in range(0, len(queries), window_size):
        window = queries[i : i + window_size]
        futures = [concurrent_run(process_query, q, datasource) for q in window]
        # Wait for this window to finish before starting the next
        for f in futures:
            results.append(f.result())
    return results
```

### Conditional fan-out

Use standard `if`/`else` to decide at runtime whether to fan out:

```python
@uniflow.workflow()
def adaptive_pipeline(data_url: str, large_dataset: bool):
    if large_dataset:
        # Fan out into parallel shards
        callables = [
            new_callable(process_shard, data_url, shard_idx)
            for shard_idx in range(8)
        ]
        batch_future = concurrent_batch_run(callables, max_concurrency=4)
        return batch_future.get()
    else:
        # Single sequential pass is fine
        return process_shard(data_url, 0)
```

:::note Workflow code limitations
Workflow functions run inside a Starlark interpreter for deterministic replay. A few Python constructs are not available in workflow code:

- **No standard library imports** — use Uniflow builtins (`uniflow.time()`, `concurrent_run`, etc.) instead of `time.time()` or other modules.
- **No f-strings** — use `.format()` instead: `"SELECT * FROM {t}".format(t=table_name)`.
- **No `is` comparisons** — use `==`: `if x == None` not `if x is None`.
- **No `try`/`except`** — error handling must be done inside `@task` functions.
- **No chained comparisons** — use `and`: `if 1 < x and x < 5` not `if 1 < x < 5`.

Task functions (`@task`) run as normal Python inside a container and have no such restrictions.
:::

## DatasetVariable — sharing datasets between tasks

Tasks running on different compute backends (Spark and Ray, for example) cannot return raw DataFrames directly — the types are not serializable across runtimes. `DatasetVariable` is the standard way to pass a dataset from one task to another regardless of backend.

### How it works

The producing task wraps its output in a `DatasetVariable`, saves it to storage, and returns the variable. The consuming task receives the variable and loads it in its own format.

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.spark import SparkTask
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.workflow.variables import DatasetVariable


# Producing task (Spark) — creates and returns a DatasetVariable
@uniflow.task(
    config=SparkTask(
        driver_cpu=2,
        driver_memory="8G",
        executor_cpu=2,
        executor_memory="4G",
        executor_instances=4,
    )
)
def load_and_preprocess(spark_sql: str) -> DatasetVariable:
    from pyspark.sql import SparkSession

    df = SparkSession.getActiveSession().sql(spark_sql)
    df = df.filter(df["label"].isNotNull())

    dv = DatasetVariable.create(df)
    dv.save_spark_dataframe()   # persist to storage before returning
    return dv


# Consuming task (Ray) — receives the variable and loads it as a Ray Dataset
@uniflow.task(
    config=RayTask(
        head_cpu=2,
        head_memory="8Gi",
        worker_cpu=2,
        worker_memory="8Gi",
        worker_instances=4,
    )
)
def train(features: DatasetVariable, epochs: int) -> dict:
    features.load_ray_dataset()     # load from storage as a Ray Dataset
    ds = features.value             # access the Ray Dataset

    # ... distributed training with ds ...
    return {"epochs_run": epochs}


# Workflow wires the two tasks together
@uniflow.workflow()
def training_pipeline(spark_sql: str, epochs: int):
    features = load_and_preprocess(spark_sql)
    result = train(features, epochs)
    return result
```

### DatasetVariable API

| Method | When to use |
|---|---|
| `DatasetVariable.create(value)` | Wrap a pandas DataFrame, PySpark DataFrame, or Ray Dataset |
| `dv.save()` | Auto-detect backend and persist to storage |
| `dv.save_pandas_dataframe()` | Explicitly persist as Parquet (PyArrow) |
| `dv.save_spark_dataframe()` | Explicitly persist via Spark |
| `dv.save_ray_dataset()` | Explicitly persist via Ray |
| `dv.load_pandas_dataframe()` | Load from storage into a pandas DataFrame |
| `dv.load_spark_dataframe()` | Load from storage into a PySpark DataFrame |
| `dv.load_ray_dataset()` | Load from storage into a Ray Dataset |
| `dv.value` | Access the in-memory value after a `create()` or `load_*()` call |

### Returning multiple DatasetVariables

Return them from a `@dataclass`:

```python
from dataclasses import dataclass

@dataclass
class SplitResult:
    train_data: DatasetVariable
    val_data: DatasetVariable

@uniflow.task(config=RayTask(...))
def split_dataset(raw_data_url: str, val_fraction: float) -> SplitResult:
    import ray

    ds = ray.data.read_parquet(raw_data_url)
    train_ds, val_ds = ds.train_test_split(test_size=val_fraction)

    train_dv = DatasetVariable.create(train_ds)
    val_dv = DatasetVariable.create(val_ds)
    train_dv.save_ray_dataset()
    val_dv.save_ray_dataset()

    return SplitResult(train_data=train_dv, val_data=val_dv)


@uniflow.workflow()
def pipeline(data_url: str):
    split = split_dataset(data_url, val_fraction=0.2)
    result = train(split.train_data, epochs=10)
    eval_report = evaluate(split.val_data, result)
    return eval_report
```

## Complete example: parallel calibration with shared data

This example combines concurrent batch execution and `DatasetVariable` — a common pattern for hyperparameter search or ensemble calibration runs that share a preprocessed dataset.

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.core.lib.concurrent import new_callable, batch_run as concurrent_batch_run
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.uniflow.plugins.spark import SparkTask
from michelangelo.workflow.variables import DatasetVariable
from dataclasses import dataclass


@uniflow.task(config=SparkTask(driver_cpu=2, driver_memory="8G", executor_instances=4))
def preprocess(data_url: str) -> DatasetVariable:
    from pyspark.sql import SparkSession
    df = SparkSession.getActiveSession().read.parquet(data_url)
    dv = DatasetVariable.create(df)
    dv.save_spark_dataframe()
    return dv


@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi", worker_instances=2))
def calibrate(dataset: DatasetVariable, config_id: str, learning_rate: float) -> dict:
    dataset.load_ray_dataset()
    ds = dataset.value
    # ... train with ds and learning_rate ...
    return {"config_id": config_id, "val_loss": 0.42}


@uniflow.workflow()
def search_pipeline(data_url: str):
    # Step 1: preprocess once (Spark)
    dataset = preprocess(data_url)

    # Step 2: calibrate in parallel across learning rates (Ray), max 3 at a time
    learning_rates = [1e-4, 5e-4, 1e-3, 5e-3, 1e-2]
    callables = [
        new_callable(calibrate, dataset, f"lr={lr}", lr)
        for lr in learning_rates
    ]
    batch_future = concurrent_batch_run(callables, max_concurrency=3)
    results = batch_future.get()

    # Pick the best config
    return min(results, key=lambda r: r["val_loss"])
```

## Next steps

- **Run your pipeline** — [Running Uniflow Pipelines](./running-uniflow.md)
- **Cache task results** — [Caching and Pipeline Resume](./cache-and-pipelinerun-resume-form.md) to skip unchanged tasks on reruns
- **Explore examples** — The [California Housing XGBoost](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/pipelines/california_housing_xgb) and [Amazon Books](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/amazon_books_qwen) examples both use `DatasetVariable` to pass data between Spark and Ray tasks
