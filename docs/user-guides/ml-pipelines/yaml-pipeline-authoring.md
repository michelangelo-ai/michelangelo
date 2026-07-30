# YAML Pipeline Authoring

Author a Uniflow pipeline as a `pipeline_conf.yaml` file instead of wiring tasks together directly
in Python. This is useful when a pipeline's structure is fixed but its per-task parameters
(learning rate, dataset name, resource sizing, etc.) need to vary across runs or teams without
touching code.

## What you'll learn

* How to write task and workflow functions for YAML-driven configuration
* The `pipeline_conf.yaml` schema: `workflow_function`, `workflow_config`, and `task_configs`
* How to run a YAML-configured pipeline locally

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
  - `job_specs` (optional) — resource sizing (see `michelangelo.canvas.schema.v2alpha1.job_specs`).

## Running a YAML-configured pipeline

```python
from michelangelo.canvas.pipeline.run import run_pipeline

result = run_pipeline("path/to/pipeline_conf.yaml")
```

See `python/examples/canvasflex_pipeline/` for a complete runnable example, including its README.

## Current scope

This is an initial phase of YAML pipeline authoring. Not yet supported (tracked as follow-up work):

- `{{var.}}` / `{{task.}}` / `{{fn.}}` templating in `pipeline_conf.yaml` values.
- A build-system macro for producing deployable pipeline artifacts from a `pipeline_conf.yaml`
  directory — `run_pipeline` runs a pipeline in-process locally only, without the distributed
  (Ray/Spark) environment setup that [Running Uniflow Pipelines](./running-uniflow.md) covers for
  plain Python-authored pipelines.

## Next steps

- [Running Uniflow Pipelines](./running-uniflow.md) — local vs. remote execution once you're ready
  to run at scale.
- [Workflow Patterns](./workflow-patterns.md) — sequencing, branching, and sharing data across
  tasks.
