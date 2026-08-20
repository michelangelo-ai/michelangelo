---
sidebar_position: 0
sidebar_label: Overview
---

# Python SDK Reference

Auto-generated reference for the `michelangelo` Python package. All pages are generated from source docstrings — run `bun run docs:api` from the `website/` directory to regenerate after SDK changes.

## Uniflow — Tasks & Workflows

The core authoring surface for pipeline tasks and workflows.

| Module | Description |
|--------|-------------|
| [`uniflow.core.decorator`](michelangelo/uniflow/core/decorator) | `@task` and `@workflow` decorators |
| [`uniflow.core.task_config`](michelangelo/uniflow/core/task_config) | Base task configuration types |
| [`uniflow.core.context`](michelangelo/uniflow/core/context) | Execution context passed into tasks |
| [`uniflow.core.image_spec`](michelangelo/uniflow/core/image_spec) | `ImageSpec` for per-task container images |
| [`uniflow.core.io_registry`](michelangelo/uniflow/core/io_registry) | Dataset IO type registry |

## Uniflow — Plugins

Compute-backend plugins that extend `@task` with Ray, Spark, or Pandas execution.

| Module | Description |
|--------|-------------|
| [`uniflow.plugins.ray.task`](michelangelo/uniflow/plugins/ray/task) | `RayTask` config for Ray-backed tasks |
| [`uniflow.plugins.ray.run_config`](michelangelo/uniflow/plugins/ray/run_config) | Ray run configuration |
| [`uniflow.plugins.ray.io`](michelangelo/uniflow/plugins/ray/io) | Ray dataset IO |
| [`uniflow.plugins.spark.task`](michelangelo/uniflow/plugins/spark/task) | `SparkTask` config for Spark-backed tasks |
| [`uniflow.plugins.spark.io`](michelangelo/uniflow/plugins/spark/io) | Spark dataset IO |
| [`uniflow.plugins.pandas.io`](michelangelo/uniflow/plugins/pandas/io) | Pandas dataset IO |

## CanvasFlex — Pusher

The pusher component for CanvasFlex template-driven workflows.

| Module | Description |
|--------|-------------|
| [`workflow.tasks.pusher.task`](michelangelo/workflow/tasks/pusher/task) | Pusher task definition |
| [`workflow.tasks.pusher.registry`](michelangelo/workflow/tasks/pusher/registry) | Pusher plugin registry |
| [`workflow.tasks.pusher.exceptions`](michelangelo/workflow/tasks/pusher/exceptions) | Pusher exception types |

## Trainer

PyTorch training utilities used with `LightningTrainer`.

| Module | Description |
|--------|-------------|
| [`lib.trainer.torch.data_collate_functions`](michelangelo/lib/trainer/torch/data_collate_functions) | Collate functions for data loading |
| [`lib.trainer.torch.utils`](michelangelo/lib/trainer/torch/utils) | Trainer utilities |

## Native Transform

TorchScript- and ONNX-exportable transform layers for train/serve parity.

| Module | Description |
|--------|-------------|
| [`lib.native_transform.torch.base_layers`](michelangelo/lib/native_transform/torch/base_layers) | Base transform layer classes |
| [`lib.native_transform.torch.id_hash_tokenizer`](michelangelo/lib/native_transform/torch/id_hash_tokenizer) | ID hashing tokenizer layer |
| [`lib.native_transform.torch.utils`](michelangelo/lib/native_transform/torch/utils) | Transform utilities |

## Workflow Variables

Typed dataset and metadata variables for pipeline IO.

| Module | Description |
|--------|-------------|
| [`workflow.variables.types`](michelangelo/workflow/variables/types) | `DatasetVariable` and related types |
| [`workflow.variables.metadata`](michelangelo/workflow/variables/metadata) | Variable metadata types |
