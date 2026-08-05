# YAML Pipeline Authoring

Author a Uniflow pipeline as a `pipeline_conf.yaml` file instead of wiring tasks together directly
in Python. This is useful when a pipeline's structure is fixed but its per-task parameters
(learning rate, dataset name, resource sizing, etc.) need to vary across runs or teams without
touching code.

## What you'll learn

* How to write task and workflow functions for YAML-driven configuration
* The `pipeline_conf.yaml` schema: `workflow_function`, `workflow_config`, `task_configs`, and
  per-task `job_specs`
* How to run a YAML-configured pipeline locally and remotely (distributed Ray/Spark)

## Prerequisites

- **A Uniflow workflow defined** — See [Getting Started with ML Pipelines](../getting-started/getting-started.md) if you haven't defined tasks and a workflow yet.

## Authoring tasks and workflows

Use `pipeline_task` (`michelangelo.canvas.pipeline.task`) instead of `@task` directly. It wraps
`@task` and adds optional lifecycle hooks (`pre_hook`, `post_hook`, `on_error`) without requiring a
separate decorator layered on top:

```python
from michelangelo.canvas.lib.shared.json_data.json_data import JSONData
from michelangelo.canvas.pipeline.task import pipeline_task
from michelangelo.uniflow.core.decorator import workflow
from michelangelo.uniflow.plugins.ray import RayTask


class TrainConfig(JSONData):
    learning_rate: float
    epochs: int


@pipeline_task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def train(config: TrainConfig) -> dict:
    ...


class PipelineConfig(JSONData):
    experiment_name: str


@workflow()
def my_pipeline(config: PipelineConfig, task_configs: dict):
    return train(config=task_configs["train"])
```

A task's `config` parameter must be annotated with its config type (e.g. `TrainConfig` above) —
the loader uses that annotation to know which type to parse the task's YAML `config` block into. The
same applies to the workflow's own config parameter, if it has one.

## Writing `pipeline_conf.yaml`

```yaml
workflow_function: my_package.my_module.my_pipeline
workflow_config:
  experiment_name: my-experiment
task_configs:
  train:
    config:
      learning_rate: 0.01
      epochs: 5
```

- `workflow_function` — fully qualified name of the `@workflow`-decorated function to run.
- `workflow_config` — optional. Only needed if the workflow function takes a workflow-level config
  argument (two-parameter signature: `(config, task_configs)`). Omit both this key and the
  parameter if the workflow only needs `task_configs`.
- `task_configs.<task_name>` — one entry per task the workflow calls by name:
  - `config` — parsed into the task function's annotated config type.
  - `task_function` (optional) — fully qualified name of an alternate implementation to run in
    place of the workflow module's own `<task_name>` function. Omit this to use the task as defined
    in the workflow module.
  - `job_specs` (optional) — per-task resource sizing applied at run time (see
    [Sizing distributed tasks with `job_specs`](#sizing-distributed-tasks-with-job_specs)).

## Sizing distributed tasks with `job_specs`

Tasks decorated with a distributed config (`RayTask`, `SparkTask`) carry resource defaults in
code. A `job_specs` block on a task's `task_configs` entry overrides those at run time, so the
same workflow code can be resized per pipeline without touching Python:

```yaml
task_configs:
  prepare_data:               # a SparkTask-decorated task
    config:
      num_rows: 200
    job_specs:
      spark:
        driver:
          pod:
            resource: {cpu: 2, memory: 2G, disk_size: 20G, gpu: 0, gpu_sku: ""}
        executor:
          pod:
            resource: {cpu: 2, memory: 2G, disk_size: 20G, gpu: 0, gpu_sku: ""}
          instances: 2
  evaluate:                   # a RayTask-decorated task
    config:
      threshold: 0.5
    job_specs:
      ray:
        head:
          pod:
            resource: {cpu: 2, memory: 2Gi, disk_size: 8Gi, gpu: 0, gpu_sku: ""}
        worker:
          pod:
            resource: {cpu: 2, memory: 2Gi, disk_size: 8Gi, gpu: 0, gpu_sku: ""}
          min_instances: 1
          max_instances: 2
```

The shape follows `michelangelo.canvas.schema.v2alpha1.job_specs` — `job_specs.spark` with
`driver`/`executor` pods, or `job_specs.ray` with `head`/`worker` pods. All `resource` fields
are required: set `gpu: 0` and `gpu_sku: ""` explicitly for CPU-only tasks.

Resource values are resolved at run time with increasing precedence:

1. **Decorator defaults** — the `RayTask(...)`/`SparkTask(...)` values in code.
2. **Environment overrides** — e.g. `RAY_OVERRIDE_HEAD_CPU.<task_path>`,
   `SPARK_OVERRIDE_DRIVER_MEMORY.<task_path>`.
3. **`job_specs`** — the YAML block above; wins over both.

## Running a YAML-configured pipeline

Locally, in-process (task Python bodies run in this process — a `SparkTask` starts a local Spark
session, a `RayTask` a local Ray runtime):

```python
from michelangelo.canvas.pipeline.run import run_pipeline

result = run_pipeline("path/to/pipeline_conf.yaml")
```

Remotely, through the same registration/remote-run machinery as Python-authored pipelines —
`michelangelo.canvas.pipeline.register` resolves the YAML into a workflow call and hands it to
the standard Uniflow execution context:

```bash
python -m michelangelo.canvas.pipeline.register path/to/pipeline_conf.yaml \
    remote-run --image <IMAGE> --storage-url <STORAGE_URL> --yes
```

Everything after the YAML path is the standard Uniflow context CLI, so `local-run` and the other
`remote-run` flags work unchanged. For mactl-based registration,
`michelangelo.canvas.pipeline.register.register_pipeline(...)` wraps
`michelangelo.uniflow.registration.register.register()` the same way.

See `python/examples/canvasflex_pipeline/` for a minimal local-only example, and
`python/examples/canvasflex_ray_spark/` for a full Spark + Ray pipeline with `job_specs`
overrides and an optional task, including its README.

## Current scope

This is an initial phase of YAML pipeline authoring. Not yet supported (tracked as follow-up work):

- `{{var.}}` / `{{task.}}` / `{{fn.}}` templating in `pipeline_conf.yaml` values.
- A build-system macro for producing deployable pipeline artifacts from a `pipeline_conf.yaml`
  directory.
- `job_specs.spark.spark_conf` and `job_specs.spark.deps` are accepted by the schema but not yet
  plumbed through to the Spark job submission.

## Next steps

- [Running Uniflow Pipelines](./running-uniflow.md) — local vs. remote execution once you're ready
  to run at scale.
- [Workflow Patterns](./workflow-patterns.md) — sequencing, branching, and sharing data across
  tasks.
