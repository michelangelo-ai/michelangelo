---
sidebar_position: 0
sidebar_label: Overview
---

# Python SDK Reference

Auto-generated reference for the `michelangelo` Python package, generated from source docstrings via `pydoc-markdown` (config at repo root: `pydoc-markdown.yml`). Run `bun run docs:api` from the `website/` directory to regenerate the pages below after docstring changes.

Some modules below aren't linked yet — coming soon.

## Uniflow — Tasks & Workflows

The core authoring surface for pipeline tasks and workflows.

| Module | Description |
|--------|-------------|
| [`uniflow.core.decorator`](reference/uniflow/core/decorator.md) | `@task` and `@workflow` decorators |
| [`uniflow.core.task_config`](reference/uniflow/core/task_config.md) | Base task configuration types |
| [`uniflow.core.context`](reference/uniflow/core/context.md) | Execution context passed into tasks |
| [`uniflow.core.image_spec`](reference/uniflow/core/image_spec.md) | `ImageSpec` for per-task container images |
| [`uniflow.core.io_registry`](reference/uniflow/core/io_registry.md) | Dataset IO type registry |

## Uniflow — Plugins

Compute-backend plugins that extend `@task` with Ray, Spark, or Pandas execution.

| Module | Description |
|--------|-------------|
| [`uniflow.plugins.ray.task`](reference/uniflow/plugins/ray/task.md) | `RayTask` config for Ray-backed tasks |
| `uniflow.plugins.ray.run_config` | Ray run configuration |
| `uniflow.plugins.ray.io` | Ray dataset IO |
| [`uniflow.plugins.spark.task`](reference/uniflow/plugins/spark/task.md) | `SparkTask` config for Spark-backed tasks |
| [`uniflow.plugins.spark.io`](reference/uniflow/plugins/spark/io.md) | Spark dataset IO |
| `uniflow.plugins.pandas.io` | Pandas dataset IO |

## CanvasFlex — Pusher

The pusher component for CanvasFlex template-driven workflows.

| Module | Description |
|--------|-------------|
| `workflow.tasks.pusher.task` | Pusher task definition |
| `workflow.tasks.pusher.registry` | Pusher plugin registry |
| [`workflow.tasks.pusher.exceptions`](reference/workflow/tasks/pusher/exceptions.md) | Pusher exception types |

## Trainer

PyTorch training utilities used with `LightningTrainer`.

| Module | Description |
|--------|-------------|
| [`lib.trainer.torch.data_collate_functions`](reference/lib/trainer/torch/data_collate_functions.md) | Collate functions for data loading |
| `lib.trainer.torch.utils` | Trainer utilities |

## Native Transform

TorchScript- and ONNX-exportable transform layers for train/serve parity.

| Module | Description |
|--------|-------------|
| `lib.native_transform.torch.base_layers` | Base transform layer classes |
| `lib.native_transform.torch.id_hash_tokenizer` | ID hashing tokenizer layer |
| `lib.native_transform.torch.utils` | Transform utilities |

## Workflow Variables

Typed dataset and metadata variables for pipeline IO.

| Module | Description |
|--------|-------------|
| `workflow.variables.types` | `DatasetVariable` and related types |
| `workflow.variables.metadata` | Variable metadata types |
