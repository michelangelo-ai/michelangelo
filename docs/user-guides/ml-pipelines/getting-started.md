# Getting started with ML pipelines

Build and run your first ML pipeline on Michelangelo in minutes. This guide walks you through a complete example -- from defining tasks and workflows to running locally and deploying remotely.

## What you'll build

By the end of this guide you'll have a working pipeline that looks like this:

```
dataset_id
    └─▶ load_dataset()       # Ray task: splits data into train/val/test
            └─▶ train()      # Ray task: trains a model on the splits
                    └─▶ evaluate()   # Ray task: computes metrics, returns results
```

Each step runs as an isolated, containerized task. Michelangelo handles data passing between them, caches intermediate results, and retries on transient failures — your code stays plain Python.

## What you'll learn

* How to define tasks with the `@task` decorator
* How to compose tasks into a workflow with `@workflow`
* How to run pipelines locally and remotely
* How to register and manage pipelines with the `ma` CLI

## Prerequisites

* Python 3.9+
* [Poetry](https://python-poetry.org/) installed
* For remote runs: Docker and access to a Kubernetes cluster (or use the [local sandbox](../../getting-started/sandbox-setup.md))
* [Create a project](../project-management-for-ml-pipelines.md)

## Environment setup

Clone the repository and install dependencies:

```bash
git clone https://github.com/michelangelo-ai/michelangelo.git
cd michelangelo/python
poetry install -E example
```

This creates a `.venv` directory with all dependencies installed. You can activate it directly or run commands via `poetry run`.

## Core concepts

Before writing code, understand the three building blocks of an ML pipeline:

| Concept | What It Does | Defined With |
| --- | --- | --- |
| **Task** | A discrete unit of work (data prep, training, evaluation) that runs in a container | `@uniflow.task()` |
| **Workflow** | Orchestrates tasks in sequence, with branching and loops | `@uniflow.workflow()` |
| **Pipeline** | A deployable instance of a workflow with specific configuration | `pipeline.yaml` |

## Step 1: Define your tasks

Tasks are Python functions decorated with `@uniflow.task()`. Each task runs independently in its own container with configurable compute resources.

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.workflow.variables import DatasetVariable

@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="4Gi",
        worker_cpu=1,
        worker_memory="4Gi",
        worker_instances=0,
    ),
)
def feature_prep(
    columns: list[str],
    test_size: float = 0.25,
) -> tuple[DatasetVariable, DatasetVariable]:
    """Download data and split into train/validation sets."""
    import ray.data
    import pandas as pd
    import numpy as np

    data_url = "http://lib.stat.cmu.edu/datasets/boston"
    raw_df = pd.read_csv(data_url, sep=r"\s+", skiprows=22, header=None)
    X = np.hstack([raw_df.values[::2, :], raw_df.values[1::2, :2]])
    y = raw_df.values[1::2, 2]

    feature_names = columns[:-1]
    dataset = [
        dict(zip(feature_names, features), target=target)
        for features, target in zip(X, y)
    ]
    data = ray.data.from_items(dataset).select_columns(columns)
    train_data, validation_data = data.train_test_split(
        test_size=test_size, shuffle=True, seed=1
    )

    train_dv = DatasetVariable.create(train_data)
    train_dv.save_ray_dataset()
    validation_dv = DatasetVariable.create(validation_data)
    validation_dv.save_ray_dataset()

    return train_dv, validation_dv
```

### Choosing a task type

Michelangelo supports two compute backends for tasks:

| Task Type | Best For | Example Config |
| --- | --- | --- |
| **RayTask** | Distributed training, GPU workloads, general-purpose compute | `RayTask(head_cpu=2, worker_instances=4)` |
| **SparkTask** | Large-scale data preprocessing, ETL, SQL-based transformations | `SparkTask(driver_cpu=2, executor_instances=4)` |

You can mix both types in a single workflow. Data types automatically convert between frameworks.

## Step 2: Compose a workflow

A workflow orchestrates your tasks. It defines the execution order and passes data between tasks.

```python
@uniflow.workflow()
def train_workflow(dataset_cols: str):
    """Orchestrate the full training pipeline."""
    columns = dataset_cols.split(",")

    # Step 1: Prepare features
    train_dv, validation_dv = feature_prep(columns=columns)

    # Step 2: Train the model
    result = train(
        train_dv=train_dv,
        validation_dv=validation_dv,
        params={
            "objective": "reg:linear",
            "max_depth": 5,
            "learning_rate": 0.1,
        },
    )
    return result
```

**Important**: Workflow code has restrictions because it compiles to Starlark for production execution. Inside a workflow function, you can only call task functions, other workflows, and built-in functions. See [Workflow constraints](#workflow-constraints) for details.

## Step 3: Run locally

Add a main block to create a context and run the workflow:

```python
if __name__ == "__main__":
    ctx = uniflow.create_context()

    # Optionally set different parameters for local vs remote runs
    if ctx.is_local_run():
        ctx.environ["DATASET_SIZE"] = "100"  # small dataset for local testing
    else:
        ctx.environ["DATASET_SIZE"] = "1000000"  # full dataset for remote

    ctx.run(
        train_workflow,
        dataset_cols="CRIM,ZN,INDUS,CHAS,NOX,RM,AGE,DIS,RAD,TAX,PTRATIO,B,LSTAT,target",
    )
```

The context provides three methods:

| Method | Purpose |
| --- | --- |
| `ctx.run(fn, **params)` | Execute the workflow function |
| `ctx.environ` | Dict of environment variables to set before execution |
| `ctx.is_local_run()` | Returns `True` if running locally (useful for conditional logic) |

Then run it:

```bash
cd michelangelo/python
PYTHONPATH=. poetry run python examples/boston_housing_xgb/boston_housing_xgb.py
```

Local runs execute everything in your Python interpreter with zero infrastructure setup. This is the fastest way to iterate on your workflow logic.

## Step 4: Run remotely

When you need more compute power or want to validate against production infrastructure, switch to a remote run.

### Build and push a Docker image

```bash
docker build -t my-workflow:latest -f ./examples/Dockerfile .
```

### Run with remote execution

```bash
PYTHONPATH=. poetry run python examples/boston_housing_xgb/boston_housing_xgb.py remote-run \
  --image docker.io/library/my-workflow:latest \
  --storage-url s3://my-bucket/workflows \
  --yes
```

Remote runs execute workflow code in a Cadence/Temporal worker and task code in Kubernetes containers with full resource isolation. For detailed remote setup instructions including sandbox configuration, see [Running Uniflow pipelines](./running-uniflow.md).

## Step 5: Register as a pipeline

To manage your workflow through MA Studio and the `ma` CLI, register it as a pipeline.

### Create pipeline.yaml

```yaml
apiVersion: michelangelo.api/v2
kind: Pipeline
metadata:
  namespace: my-project  # Your project name
  name: boston-housing-xgb
  annotations:
    michelangelo/uniflow-image: ghcr.io/michelangelo-ai/examples:main
spec:
  type: PIPELINE_TYPE_TRAIN
  manifest:
    filePath: examples.boston_housing_xgb.boston_housing_xgb
```

### Register the pipeline

```bash
ma pipeline apply -f pipeline.yaml
```

### Run the registered pipeline

```bash
ma pipeline run --namespace my-project --name boston-housing-xgb
```

## Workflow constraints

Workflow functions compile to Starlark for production execution. This means some Python features are not available inside `@workflow` functions:

| Not Supported | Use Instead |
| --- | --- |
| `import` statements | Call task functions (imports go inside tasks) |
| `try-except` blocks | Use task-level retry |
| `is` / `is not` | `==` / `!=` |
| f-strings | `"{}".format(value)` |
| Chained comparisons (`1 < x < 5`) | `1 < x and x < 5` |
| Standard library calls | Built-in functions (e.g., `uniflow.time()`) |

These constraints only apply to workflow code. Task code runs in containers and can use any Python code.

## Task features

### Caching

Cache task results to skip re-execution on subsequent runs:

```python
@uniflow.task(
    config=RayTask(head_cpu=1, head_memory="4Gi"),
    cache_enabled=True,
    cache_version="v1",
)
def feature_prep(columns: list[str]):
    ...
```

Cached results are available for approximately 28 days (platform-managed). Change `cache_version` to force re-execution.

### Retry

Automatically retry failed tasks:

```python
@uniflow.task(
    config=RayTask(head_cpu=1, head_memory="4Gi"),
    retry_attempts=3,
)
def train(params: dict):
    ...
```

Each retry creates a fresh cluster for better isolation.

### Task overrides

Create task variants with different resource configurations using `with_overrides()`:

```python
@uniflow.workflow()
def train_workflow(dataset_cols: str):
    columns = dataset_cols.split(",")

    # Run feature_prep with more resources
    large_feature_prep = feature_prep.with_overrides(
        alias="large_feature_prep",
        config=RayTask(head_cpu=2, worker_instances=2),
    )
    train_dv, validation_dv = large_feature_prep(columns=columns)
    ...
```

**Important**: The override config **replaces** the original config entirely -- it does not merge fields. Specify all required fields in the override.

## Complete example

See the full Boston Housing XGBoost example at [`python/examples/boston_housing_xgb/`](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/boston_housing_xgb). This example demonstrates:

* **Heterogeneous workflow**: Ray tasks for data prep and training, Spark task for preprocessing
* **Task caching**: Reuse feature preparation results across runs
* **Task overrides**: Customize resource allocation per workflow
* **DatasetVariable**: Pass datasets between tasks across different compute frameworks

## Next steps

* [Pipeline Running Modes](./pipeline-running-modes.md) -- Understand when to use Local, Remote, Dev, and Pipeline runs
* [Pipeline Management](./pipeline-management.md) -- Learn about standard vs custom workflows
* [Caching and Resume](./cache-and-pipelinerun-resume-form.md) -- Resume failed pipeline runs from a specific step
* [Data Preparation](../prepare-your-data.md) -- Deep dive into data preprocessing patterns
* [Model Training](../train-and-register-a-model.md) -- Advanced distributed training with Lightning Trainer SDK

## Troubleshooting

* **Out of memory during training?** Increase `head_memory` or `worker_memory` in your task config, or reduce your dataset size for local runs.
* **Remote run fails to start?** Verify your Docker image exists and is accessible. Check that `--storage-url` points to a valid S3-compatible bucket.
* **Workflow code errors with "not supported in Starlark"?** Move the unsupported syntax (imports, try-except, f-strings) into a task function. See [Workflow constraints](#workflow-constraints).
