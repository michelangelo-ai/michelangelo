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

`LightningTrainer` itself isn't in the current generator config and has no page yet;
the **Trainer** section covers the XGBoost trainer and the Torch collate-function and
memory-estimation utilities. The FSDP2 strategy lives in a private module
(`_private/model_parallel.py`) and is intentionally not documented as a public API.

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
| [`uniflow.plugins.ray.run_config`](reference/uniflow/plugins/ray/run_config.md) | Ray run configuration |
| [`uniflow.plugins.ray.io`](reference/uniflow/plugins/ray/io.md) | Ray dataset IO |
| [`uniflow.plugins.spark.task`](reference/uniflow/plugins/spark/task.md) | `SparkTask` config for Spark-backed tasks |
| [`uniflow.plugins.spark.io`](reference/uniflow/plugins/spark/io.md) | Spark dataset IO |
| [`uniflow.plugins.pandas.io`](reference/uniflow/plugins/pandas/io.md) | Pandas dataset IO |

## CanvasFlex — Pusher

The pusher component for CanvasFlex template-driven workflows.

| Module | Description |
|--------|-------------|
| [`workflow.tasks.pusher.task`](reference/workflow/tasks/pusher/task.md) | Pusher task definition |
| [`workflow.tasks.pusher.registry`](reference/workflow/tasks/pusher/registry.md) | Pusher plugin registry |
| [`workflow.tasks.pusher.exceptions`](reference/workflow/tasks/pusher/exceptions.md) | Pusher exception types |

## Trainer

The XGBoost trainer, plus PyTorch training utilities used with `LightningTrainer`
(`LightningTrainer` itself isn't generated into this reference yet — see the note above).

| Module | Description |
|--------|-------------|
| [`lib.trainer.xgboost.xgboost_trainer`](reference/lib/trainer/xgboost/xgboost_trainer.md) | `XGBoostTrainer` and `XGBoostTrainerParam` for distributed XGBoost on Ray |
| [`lib.trainer.torch.data_collate_functions`](reference/lib/trainer/torch/data_collate_functions.md) | Collate functions for data loading |
| [`lib.trainer.torch.utils`](reference/lib/trainer/torch/utils.md) | Trainer utilities |

## Evaluator

Config-driven TorchMetrics evaluation, threshold gating, and report building.

| Module | Description |
|--------|-------------|
| [`lib.evaluator.evaluator`](reference/lib/evaluator/evaluator.md) | `TorchMetricEvaluator` and `create_evaluator` |
| [`lib.evaluator.metrics`](reference/lib/evaluator/metrics.md) | Metric instantiation, dtype resolution, tensor extraction |
| [`lib.evaluator.config`](reference/lib/evaluator/config.md) | Load an `EvaluatorConfig` from a dict or YAML |
| [`lib.evaluator.chart_data`](reference/lib/evaluator/chart_data.md) | ROC/PR curves, regression errors, segment filtering |
| [`lib.evaluator.sanity`](reference/lib/evaluator/sanity.md) | Threshold gating that fails a run on a breached metric |
| [`lib.evaluator.report`](reference/lib/evaluator/report.md) | `EvaluationReport` charts and tables |
| [`lib.evaluator.report_generator`](reference/lib/evaluator/report_generator.md) | Build per-dataset and combined reports |

## Native Transform

TorchScript- and ONNX-exportable transform layers for train/serve parity.

| Module | Description |
|--------|-------------|
| [`lib.native_transform.torch.base_layers`](reference/lib/native_transform/torch/base_layers.md) | Base transform layer classes |
| [`lib.native_transform.torch.id_hash_tokenizer`](reference/lib/native_transform/torch/id_hash_tokenizer.md) | ID hashing tokenizer layer |
| [`lib.native_transform.torch.utils`](reference/lib/native_transform/torch/utils.md) | Transform utilities |

## Workflow Variables

Typed dataset and metadata variables for pipeline IO.

| Module | Description |
|--------|-------------|
| [`workflow.variables.types`](reference/workflow/variables/types.md) | `DatasetVariable` and related types |
| [`workflow.variables.metadata`](reference/workflow/variables/metadata.md) | Variable metadata types |

## Regenerating this reference

This section is generated from source docstrings via `pydoc-markdown`, configured by
`pydoc-markdown.yml` at the repo root. If you're updating docstrings, regenerate the
pages above by running `bun run docs:api` from the `website/` directory.

`pydoc-markdown` is pinned in `website/requirements-docs.txt`. On a fresh checkout,
install it with `pip install -r website/requirements-docs.txt` before running the
script; other versions can produce different output from the same docstrings.
