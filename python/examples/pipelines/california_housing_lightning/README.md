# California Housing Lightning

End-to-end ML pipeline for California Housing price prediction using PyTorch
Lightning via `tabular_trainer`'s `train_tabular()`. Counterpart to the
sibling [`california_housing_xgb`](../california_housing_xgb) example, which
uses a bespoke XGBoost training loop instead of `tabular_trainer`.

## Pipeline

```
feature_prep  →  preprocess  →  train  →  push_step
   (Ray)           (Spark)      (Ray)      (Spark)
```

| Step | File | Runtime | Description |
|---|---|---|---|
| `feature_prep` | `feature_prep.py` | Ray | Load dataset, train/test split, Ray Datasets |
| `preprocess` | `preprocess.py` | Spark | Cast columns to float |
| `train` | `train.py` | Ray | Distributed Lightning training via `tabular_trainer.train_tabular()` |
| `push_step` | `push.py` | Spark | Push model and preprocessed datasets to storage/registry |

`feature_prep.py` and `preprocess.py` are duplicated (not imported) from the
sibling `california_housing_xgb` example, to keep each example directory
fully self-contained per this repo's example convention.

## Prerequisites

Same as `california_housing_xgb` — see [its README](../california_housing_xgb/README.md#prerequisites)
for sandbox setup, Java/Spark requirements, and the `ma-examples` project.

## How It Works

### `train_tabular()` instead of a bespoke training loop

Unlike xgb's `train.py`, which builds its own `ScalingConfig`/`RunConfig` and
calls `xgboost.train()` directly, this example's `train.py` is a thin
`@uniflow.task` wrapper around
`michelangelo.workflow.tasks.tabular_trainer.task.train_tabular()` — the
shared Lightning + Ray Train dispatcher also covered by
`workflow/tasks/tabular_trainer/tests/`. `train_tabular()` returns a
`ModelArtifact` directly (already the type `push_step` expects), rather than
a raw checkpoint path.

### `TorchRegressionModel` — the model to plug in

`train_tabular()` requires a `LightningModule` subclass, referenced by dotted
import path via `LightningTrainerConfig.model_class`. This example defines a
minimal MLP regressor in [`model.py`](model.py): two hidden `nn.Linear`
layers (64 → 32, ReLU) feeding a single scalar output, trained with
`MSELoss` and `Adam`.

Ray Data batches passed to `training_step`/`validation_step` are dicts of
column-name → tensor (the default `iter_torch_batches` output when no custom
collate function is configured). `TorchRegressionModel` is constructed with
`feature_columns`/`label_column` (via `LightningTrainerConfig.model_kwargs`)
so it knows which batch keys to stack into its input tensor and which key
holds the regression target.

### CPU-only precision

`train.py` explicitly forces `precision="32"` via `LightningTrainerKwargs`
rather than relying on `train_tabular()`'s dispatcher default
(`"bf16-mixed"`). Verified locally that `bf16-mixed` does not error on a
CPU-only accelerator — Lightning runs real `torch.autocast('cpu',
dtype=torch.bfloat16)` AMP, not a silent fallback — but on x86 CPUs without
`AVX512_BF16` this runs via slower software emulation. `precision="32"`
keeps this tutorial-oriented example's runs fast and deterministic on the
k3d sandbox's CPU-only nodes.

### No eval report

Unlike xgb's `push_step`, this example's pusher does not push an
`eval_report` artifact: `train_tabular()` returns a `ModelArtifact` without a
training-metrics dict (unlike xgb's custom `TrainResult.metrics`), so there
is nothing meaningful to report.

## Local Run

```bash
cd michelangelo-ai/michelangelo/python
JAVA_HOME=/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home \
PYTHONPATH=. poetry run python examples/pipelines/california_housing_lightning/california_housing_lightning.py
```

Without `AWS_ENDPOINT_URL`, `train` and `push_step` both use
`LocalStorageBackend`, writing the model checkpoint and datasets to local
temp directories (no external services required).

## Remote Run / k3d sandbox

Same pattern as `california_housing_xgb` — see
[its README](../california_housing_xgb/README.md#remote-run) for the full
`remote-run` command and environment variable reference. Substitute
`california_housing_lightning` for `california_housing_xgb` in the image
build and `--file` paths.
