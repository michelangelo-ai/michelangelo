# ML Pipelines

ML Pipelines on Michelangelo let you build, run, and manage end-to-end machine learning workflows -- from data preparation to model training and evaluation. Pipelines are built with **Uniflow**, a Python-first framework that lets you define workflows using standard Python functions and run them locally or at production scale.

## What you'll learn

* How to define ML workflows using Python tasks and workflows
* The difference between standard and custom pipelines
* Which running mode to use at each stage of development
* How caching, retry, and resume improve pipeline reliability

## Key concepts

A **pipeline** is a deployable instance of a workflow with its own configuration. A **workflow** is a Python function decorated with `@workflow` that orchestrates one or more tasks. A **task** is a Python function decorated with `@task` that performs a discrete unit of work (data prep, training, evaluation) inside a container. A **context** is the entry point for running a workflow -- it holds runtime information like environment variables and input arguments.

## How it works

An ML pipeline has three layers:

| Layer | What It Does | How You Define It |
| --- | --- | --- |
| **Tasks** | Discrete units of work (data prep, training, evaluation) that run in containers | Python functions with `@task` decorator |
| **Workflows** | Orchestrate tasks with sequencing, branching, and loops | Python functions with `@workflow` decorator |
| **Pipelines** | Deployable instances with configuration, scheduling, and monitoring | `pipeline.yaml` registered via `ma` CLI |

```
Pipeline (pipeline.yaml)
  └── Workflow (@workflow)
        ├── Task 1: Data Preparation (@task + RayTask)
        ├── Task 2: Preprocessing (@task + SparkTask)
        └── Task 3: Model Training (@task + RayTask)
```

## Quick example

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

@uniflow.task(config=RayTask(head_cpu=1, head_memory="4Gi"))
def prepare_data(data_path: str):
    """Load and split data for training."""
    ...

@uniflow.task(config=RayTask(head_cpu=2, worker_instances=4))
def train_model(train_data, params: dict):
    """Train model with distributed compute."""
    ...

@uniflow.workflow()
def training_pipeline(data_path: str):
    train_data, val_data = prepare_data(data_path)
    result = train_model(train_data, params={"lr": 0.01})
    return result
```

```bash
# Run locally
poetry run python my_workflow.py

# Run remotely with cloud compute
poetry run python my_workflow.py remote-run \
  --image my-workflow:latest \
  --storage-url s3://my-bucket
```

## Two types of pipelines

| | Standard Workflows | Custom Workflows |
| --- | --- | --- |
| **Defined by** | YAML configuration (`pipeline_conf.yaml`) | Python code (`@workflow` + `@task`) |
| **Managed by** | Michelangelo (pre-built workflows) | You (fully custom logic) |
| **Best for** | Common ML patterns (train, predict, evaluate) | Unique or complex use cases |
| **UI creation** | Yes | No |
| **Flexibility** | Configurable within pre-defined structure | Unlimited |

Both types support MA Studio UI management, CLI triggers, remote execution, orchestration, and scheduling.

## Running modes

Michelangelo provides four running modes for different stages of development:

| Mode | When to Use | Provisioning Time |
| --- | --- | --- |
| [**Local Run**](./pipeline-running-modes.md#local-run-mode) | Development and debugging | Instant |
| [**Remote Run**](./pipeline-running-modes.md#remote-run-mode) | Testing with larger datasets and compute | 2-5 minutes |
| [**Pipeline Dev Run**](./pipeline-running-modes.md#pipeline-dev-run-mode) | Validating full pipeline including Docker builds | 20+ minutes |
| [**Pipeline Run**](./pipeline-running-modes.md#pipeline-run-mode) | Production deployment | Varies |

## Key features

* **Task Caching** -- Skip re-execution of unchanged tasks. Cached results are available for approximately 28 days (platform-managed). See [Caching and Resume](./cache-and-pipelinerun-resume-form.md).
* **Task Retry** -- Automatically retry failed tasks with fresh cluster isolation.
* **Pipeline Resume** -- Resume a failed pipeline run from a specific step instead of starting over.
* **File Sync** -- Test local code changes on remote infrastructure without rebuilding Docker images. See [File Sync](./file-sync-testing-flow-runbook.md).
* **Triggers and Scheduling** -- Run pipelines on cron schedules or fixed intervals. See [Set Up Triggers](../set-up-triggers.md).
* **Notifications** -- Get notified via email or Slack when pipeline runs succeed, fail, or complete.

## Architecture overview

Uniflow's execution architecture separates workflow orchestration from task execution:

* **Workflow code** (`@workflow` functions) runs in a Cadence/Temporal worker. It is compiled to Starlark for deterministic, replayable execution.
* **Task code** (`@task` functions) runs in containers on Kubernetes using Ray or Spark clusters. Tasks can run any Python code without restrictions.
* **Data checkpoints** are stored in S3-compatible storage (MinIO, S3, HDFS) between task executions, enabling caching and resume.

This separation means you can update workflow logic (task ordering, parameters, branching) without rebuilding Docker images. Only task code changes require a new image build.

## Next steps

Start with the [Getting started](./getting-started.md) guide to build and run your first pipeline, then explore the guides below for specific topics.

## Guides

| Guide | Description |
| --- | --- |
| [**Getting Started**](./getting-started.md) | Build and run your first pipeline end-to-end |
| [**Pipeline Running Modes**](./pipeline-running-modes.md) | Understand Local, Remote, Dev, and Pipeline run modes |
| [**Pipeline Management**](./pipeline-management.md) | Create and manage standard and custom pipelines |
| [**Running Uniflow Pipelines**](./running-uniflow.md) | Environment setup, execution, and debugging |
| [**Caching and Resume**](./cache-and-pipelinerun-resume-form.md) | Cache task results and resume failed runs |
| [**File Sync**](./file-sync-testing-flow-runbook.md) | Sync local code changes to remote runs |
| [**Set Up Triggers**](../set-up-triggers.md) | Schedule and automate pipeline execution |
| [**CLI Reference**](../cli.md) | Command-line tools for pipeline and project management |
| [**Project Management**](../project-management-for-ml-pipelines.md) | Create and configure MA Studio projects |
