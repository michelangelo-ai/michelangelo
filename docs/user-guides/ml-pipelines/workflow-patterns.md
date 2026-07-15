---
sidebar_position: 4
---

# Workflow Patterns

This guide covers the core patterns for constructing Uniflow workflows — from basic task sequencing to branching, loops, parallelism, and sharing data between heterogeneous tasks. It is aimed at users who are comfortable writing Python and want to build non-trivial pipelines.

## What you'll learn

- How to sequence, branch, and loop over tasks in a workflow
- How to run tasks concurrently and collect results with futures
- How to fan out across many task calls with bounded parallelism
- How to pass datasets between tasks running on different compute backends (Spark ↔ Ray) using `DatasetVariable`

## Prerequisites

- Familiarity with `@uniflow.task` and `@uniflow.workflow` — see [Getting Started with ML Pipelines](../getting-started/getting-started.md)
- Basic understanding of Uniflow's task/workflow distinction — see the [ML Pipelines overview](./index.md)

---

## Sequential task calls

The simplest pattern: call tasks one after another. Each call blocks until it returns, and you pass outputs directly as inputs to the next task.

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.spark import SparkTask
from michelangelo.uniflow.plugins.ray import RayTask


@uniflow.task(config=SparkTask(driver_cpu=2, driver_memory="8G", executor_instances=4))
def preprocess(data_url: str):
    ...

@uniflow.task(config=RayTask(head_cpu=2, head_memory="8Gi", worker_instances=4))
def train(preprocessed_data, epochs: int) -> dict:
    ...

@uniflow.task(config=RayTask(head_cpu=1, head_memory="4Gi"))
def evaluate(model, test_data) -> dict:
    ...


@uniflow.workflow()
def training_pipeline(data_url: str, epochs: int):
    preprocessed = preprocess(data_url)
    model = train(preprocessed, epochs)
    report = evaluate(model, preprocessed)
    return report
```

---

## Branching with if/else

Use standard Python `if`/`else` to decide at runtime which tasks to run. Branching logic lives in the workflow function; the tasks themselves stay focused on their computation.

```python
@uniflow.workflow()
def adaptive_training(data_url: str, use_gpu: bool, epochs: int):
    preprocessed = preprocess(data_url)

    if use_gpu:
        model = train_gpu(preprocessed, epochs)
    else:
        model = train_cpu(preprocessed, epochs)

    return evaluate(model, preprocessed)
```

You can also use branching to short-circuit a pipeline early:

```python
@uniflow.task(config=SparkTask(driver_cpu=1, driver_memory="4G", executor_instances=2))
def load_data(data_url: str):
    from pyspark.sql import SparkSession
    df = SparkSession.getActiveSession().read.parquet(data_url)
    return df, df.count()


@uniflow.workflow()
def pipeline_with_guard(data_url: str, min_rows: int = 1000):
    data, row_count = load_data(data_url)

    if row_count < min_rows:
        return {"status": "skipped", "reason": "insufficient data"}

    model = train(data, epochs=10)
    return evaluate(model, data)
```

:::note Workflow code limitations
Workflow functions run inside a Starlark interpreter for deterministic replay. A few Python constructs are unavailable in workflow code (task functions have no such restrictions):

- **No standard library imports** — use Uniflow builtins (`uniflow.time()`) instead of `time.time()` or other modules.
- **No f-strings** — use `.format()`: `"SELECT * FROM {t}".format(t=table_name)`.
- **No `is` comparisons** — use `==`: `if x == None` not `if x is None`.
- **No `try`/`except`** — handle errors inside `@task` functions instead.
- **No chained comparisons** — use `and`: `if 1 < x and x < 5` not `if 1 < x < 5`.
:::

---

## Loops and fan-out

Standard Python `for` loops work as expected in workflow functions. Use them to call a task multiple times with different inputs.

### Basic for loop

```python
@uniflow.task(config=RayTask(head_cpu=1, head_memory="4Gi"))
def evaluate_checkpoint(checkpoint_url: str, test_data_url: str) -> dict:
    ...


@uniflow.workflow()
def evaluate_all_checkpoints(checkpoint_urls: list[str], test_data_url: str):
    results = []
    for url in checkpoint_urls:
        result = evaluate_checkpoint(url, test_data_url)
        results.append(result)
    return results
```

### Fan-out then fan-in

Call multiple tasks in a loop, collect results, then aggregate:

```python
@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi", worker_instances=2))
def train_with_lr(data_url: str, learning_rate: float) -> dict:
    ...
    return {"learning_rate": learning_rate, "val_loss": 0.42}


@uniflow.workflow()
def hyperparameter_sweep(data_url: str, learning_rates: list[float]):
    results = []
    for lr in learning_rates:
        result = train_with_lr(data_url, lr)
        results.append(result)

    # Fan-in: pick the best result
    best = results[0]
    for r in results[1:]:
        if r["val_loss"] < best["val_loss"]:
            best = r
    return best
```

Note that the for-loop pattern runs each task call sequentially. To run all of them in parallel, see [Parallel batch run](#parallel-batch-run) below.

---

## Concurrent run

`concurrent_run` kicks off a task call immediately and returns a `Future` — the task runs in the background while the workflow continues. Collect the result later with `future.result()`.

Use this when you have two or more independent tasks that can overlap in time.

```python
from michelangelo.uniflow.core.lib.concurrent import run as concurrent_run


@uniflow.task(config=RayTask(head_cpu=1, head_memory="4Gi"))
def load_shard(shard_url: str) -> list:
    import pandas as pd
    return pd.read_parquet(shard_url).to_dict(orient="records")


@uniflow.workflow()
def load_two_shards(shard_a_url: str, shard_b_url: str):
    # Both load tasks start at the same time
    future_a = concurrent_run(load_shard, shard_a_url)
    future_b = concurrent_run(load_shard, shard_b_url)

    # Collect — blocks until each task finishes
    data_a = future_a.result()
    data_b = future_b.result()

    return data_a + data_b
```

:::note
In **local execution**, both `concurrent_run` and `concurrent_batch_run` run tasks sequentially (no true parallelism) — Futures are pre-resolved and `.result()` / `.get()` return immediately. True concurrent execution happens in remote runs via the Cadence/Temporal engine.
:::

---

## Parallel batch run

When you need to run many task calls with a cap on how many execute at once, use `new_callable` + `concurrent_batch_run`. This is the standard fan-out pattern for workloads like evaluating N model configurations or processing N data shards.

```python
from michelangelo.uniflow.core.lib.concurrent import (
    new_callable,
    batch_run as concurrent_batch_run,
)


@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi", worker_instances=2))
def calibrate(data_url: str, config_id: str, learning_rate: float) -> dict:
    ...
    return {"config_id": config_id, "val_loss": 0.42}


@uniflow.workflow()
def parallel_calibration(data_url: str, learning_rates: list[float]):
    # Build deferred calls — nothing runs yet
    callables = [
        new_callable(calibrate, data_url, "lr={0}".format(lr), lr)
        for lr in learning_rates
    ]

    # Run all, at most 3 at a time
    batch_future = concurrent_batch_run(callables, max_concurrency=3)

    # Block until all finish; results are in submission order
    return batch_future.get()
```

**API summary:**

| Call | What it does |
|---|---|
| `concurrent_run(fn, *args, **kwargs)` | Start one task; returns a `Future` |
| `future.result()` | Block until done, return the result |
| `future.done()` | Return `True` if the task has already finished |
| `new_callable(fn, *args)` | Create a deferred call (doesn't execute yet) |
| `concurrent_batch_run(callables, max_concurrency=N)` | Run all with at most N active at once; returns `BatchFuture`. Pass `max_concurrency=None` (the default) for unlimited concurrency |
| `batch_future.get()` | Block until all finish; return results as a list in submission order |
| `batch_future.is_ready()` | Return `True` if all tasks in the batch have finished |
| `batch_future.get_futures()` | Return the individual `Future` objects for fine-grained control |

### Windowed batches

Use this when you want to process results after each window before starting the next:

```python
@uniflow.task(config=RayTask(head_cpu=1, head_memory="4Gi"))
def run_query(query: str, datasource: str) -> list:
    ...


@uniflow.workflow()
def windowed_processing(queries: list[str], datasource: str, window_size: int = 2):
    results = []
    for i in range(0, len(queries), window_size):
        window = queries[i : i + window_size]
        futures = [concurrent_run(run_query, q, datasource) for q in window]
        for f in futures:
            results.append(f.result())
    return results
```

---

## DatasetVariable — sharing datasets between tasks

Tasks on different compute backends (Spark and Ray) cannot return raw DataFrames directly — the types are not serializable across runtimes. `DatasetVariable` is the standard way to pass a dataset from one task to another regardless of backend.

The producing task wraps its output in a `DatasetVariable`, saves it to storage, and returns it. The consuming task receives the variable and loads it in its own format.

```python
from michelangelo.workflow.variables import DatasetVariable


# Producing task (Spark)
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
    dv.save_spark_dataframe()   # persist before returning
    return dv


# Consuming task (Ray)
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
    features.load_ray_dataset()
    ds = features.value             # Ray Dataset

    # ... distributed training with ds ...
    return {"epochs_run": epochs}


@uniflow.workflow()
def training_pipeline(spark_sql: str, epochs: int):
    features = load_and_preprocess(spark_sql)
    return train(features, epochs)
```

### Returning multiple DatasetVariables

Use a `@dataclass` to return more than one:

```python
from dataclasses import dataclass


@dataclass
class SplitResult:
    train_data: DatasetVariable
    val_data: DatasetVariable


@uniflow.task(config=RayTask(head_cpu=2, head_memory="8Gi", worker_instances=2))
def split_dataset(data_url: str, val_fraction: float) -> SplitResult:
    import ray

    ds = ray.data.read_parquet(data_url)
    train_ds, val_ds = ds.train_test_split(test_size=val_fraction)

    train_dv = DatasetVariable.create(train_ds)
    val_dv = DatasetVariable.create(val_ds)
    train_dv.save_ray_dataset()
    val_dv.save_ray_dataset()

    return SplitResult(train_data=train_dv, val_data=val_dv)


@uniflow.workflow()
def pipeline(data_url: str, epochs: int):
    split = split_dataset(data_url, val_fraction=0.2)
    model = train(split.train_data, epochs)
    return evaluate(model, split.val_data)
```

### DatasetVariable API

| Method | When to use |
|---|---|
| `DatasetVariable.create(value, path=None)` | Wrap a pandas DataFrame, PySpark DataFrame, or Ray Dataset. Pass `path` to override the auto-generated storage path |
| `dv.save()` | Persist to storage — auto-dispatches based on the value type (pandas → PyArrow Parquet, Spark → SparkIO, Ray → RayDatasetIO) |
| `dv.save_pandas_dataframe()` | Persist as Parquet via PyArrow (explicit pandas path) |
| `dv.save_spark_dataframe()` | Persist via Spark (explicit Spark path) |
| `dv.save_ray_dataset()` | Persist via Ray (explicit Ray path) |
| `dv.load_pandas_dataframe()` | Load from storage as a pandas DataFrame |
| `dv.load_spark_dataframe()` | Load from storage as a PySpark DataFrame |
| `dv.load_ray_dataset()` | Load from storage as a Ray Dataset |
| `dv.value` | Lazy-loading property — returns the in-memory value, loading from storage if needed. Auto-detects the runtime (Spark session active → Spark; Ray initialized → Ray; fallback → pandas). Call an explicit `load_*()` first when you need a specific backend regardless of runtime context |

---

## Complete example

A realistic pipeline combining branching, parallel fan-out, and `DatasetVariable`:

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.core.lib.concurrent import new_callable, batch_run as concurrent_batch_run
from michelangelo.uniflow.plugins.spark import SparkTask
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.workflow.variables import DatasetVariable


@uniflow.task(config=SparkTask(driver_cpu=2, driver_memory="8G", executor_instances=4))
def preprocess(data_url: str) -> DatasetVariable:
    from pyspark.sql import SparkSession
    df = SparkSession.getActiveSession().read.parquet(data_url)
    dv = DatasetVariable.create(df)
    dv.save_spark_dataframe()
    return dv


@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi", worker_instances=2))
def calibrate(dataset: DatasetVariable, learning_rate: float) -> dict:
    dataset.load_ray_dataset()
    ds = dataset.value
    # ... train with ds ...
    return {"learning_rate": learning_rate, "val_loss": 0.42}


@uniflow.task(config=RayTask(head_cpu=2, head_memory="8Gi", worker_instances=4))
def train_final(dataset: DatasetVariable, learning_rate: float, epochs: int) -> dict:
    dataset.load_ray_dataset()
    ds = dataset.value
    # ... full training run ...
    return {"model_url": "s3://my-bucket/model"}


@uniflow.workflow()
def search_and_train(data_url: str, learning_rates: list[float], epochs: int):
    # Preprocess once (Spark)
    dataset = preprocess(data_url)

    # Calibrate all learning rates in parallel, max 3 at once (Ray)
    callables = [
        new_callable(calibrate, dataset, lr)
        for lr in learning_rates
    ]
    results = concurrent_batch_run(callables, max_concurrency=3).get()

    # Pick the best learning rate
    best = results[0]
    for r in results[1:]:
        if r["val_loss"] < best["val_loss"]:
            best = r

    # Full training run with the best config
    return train_final(dataset, best["learning_rate"], epochs)
```

## Next steps

- **Run your pipeline** — [Running Uniflow Pipelines](./running-uniflow.md)
- **Cache task results** — [Caching and Pipeline Resume](./cache-and-pipelinerun-resume-form.md) to skip unchanged tasks on reruns
- **Explore working examples** — [`python/examples/`](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples) contains runnable end-to-end pipelines, including [California Housing XGBoost](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/pipelines/california_housing_xgb) which uses `DatasetVariable` to pass data between Spark and Ray tasks
