---
sidebar_position: 0
sidebar_label: Overview
---

# Python SDK Reference

Auto-generated reference for the `michelangelo` Python package — task and workflow
decorators, compute-backend plugins, trainer utilities, and workflow variable types.
Start here if you're authoring pipelines with the Python SDK.

> New to Michelangelo AI? See [Core Concepts and Key Terms](../../getting-started/core-concepts-and-key-terms.md)
> for what a task, a workflow, and Uniflow actually are before diving into the raw
> signatures below.

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

Distributed trainers and the utilities used alongside them.

| Module | Description |
|--------|-------------|
| [`lib.trainer.torch.pytorch_lightning.lightning_trainer`](reference/lib/trainer/torch/pytorch_lightning/lightning_trainer.md) | `LightningTrainer`, `LightningTrainerWithStateDict` and `LightningTrainerParam` |
| [`lib.trainer.torch.pytorch_lightning.schema`](reference/lib/trainer/torch/pytorch_lightning/schema.md) | Warm-start schema types — `TransferLearningSpec`, `IncrementalTrainingSpec`, `ModelSpec`, `TrainingObserver`, `ExperimentStore` |
| [`lib.trainer.torch.pytorch_lightning.experiment_store`](reference/lib/trainer/torch/pytorch_lightning/experiment_store.md) | `FsspecExperimentStore` — filesystem auto-resume backend |
| [`lib.trainer.xgboost.xgboost_trainer`](reference/lib/trainer/xgboost/xgboost_trainer.md) | `XGBoostTrainer` and `XGBoostTrainerParam` |
| [`lib.trainer.torch.data_collate_functions`](reference/lib/trainer/torch/data_collate_functions.md) | Collate functions for data loading |
| `lib.trainer.torch.utils` | Trainer utilities |

> Distributed strategy selection (DDP, FSDP, FSDP2) is passed through
> `LightningTrainerParam.lightning_trainer_kwargs` to `pytorch_lightning.Trainer`, and the
> strategy classes themselves are private, so they have no generated page here. See the
> `lightning_trainer_kwargs` entry on `LightningTrainerParam` for how to set one.

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

## Regenerating this reference

This section is generated from source docstrings via `pydoc-markdown`, configured by
`pydoc-markdown.yml` at the repo root. If you're updating docstrings, regenerate the
pages above by running `bun run docs:api` from the `website/` directory.

`pydoc-markdown` isn't currently declared as a project dependency, so on a fresh
checkout you may need to install it yourself (`pip install pydoc-markdown`) before
the script succeeds.
