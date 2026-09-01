---
sidebar_label: lightning_trainer
title: michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer
---

Public PyTorch Lightning trainer wrapping Ray Train.

This package is a one-time snapshot of an internal trainer used for distributed
PyTorch Lightning training on Ray. Bugs may be patched in OSS, but new features
are not automatically backported from the source. See `CONTRIBUTING.md` for
the support policy.

Typical use:

```python
from michelangelo.lib.trainer.torch.pytorch_lightning import (
    LightningTrainer,
    LightningTrainerParam,
)

trainer = LightningTrainer(
    trainer_param=LightningTrainerParam(
        create_model_fn=my_model_factory,
        create_model_fn_kwargs={"hidden_dim": 64},
        train_data=train_ds,
        val_data=val_ds,
        batch_size=256,
    ),
    run_config=ray.train.RunConfig(name="my_run", storage_path="/tmp/runs"),
    scaling_config=ray.train.ScalingConfig(num_workers=1, use_gpu=False),
)
result = trainer.train()
```
## LightningTrainerParam Objects

```python
@dataclass
class LightningTrainerParam()
```

Configuration for `LightningTrainer`.

All callables (`create_model_fn`, `data_collate_fn`) are invoked inside the
Ray Train worker. The model is constructed on each worker via
`create_model_fn(**create_model_fn_kwargs)` rather than being pickled across
process boundaries.

**Attributes**:

- `create_model_fn` - Factory returning a `pytorch_lightning.LightningModule`. Invoked on each
  worker with `**create_model_fn_kwargs`.
- `create_model_fn_kwargs` - Keyword arguments passed to `create_model_fn`.
- `train_data` - Training Ray Dataset.
- `val_data` - Validation Ray Dataset.
- `batch_size` - Per-worker training batch size.
- `num_shuffle_batches` - Number of batches kept in the Ray Data local shuffle buffer. `0` disables
  shuffling.
- `num_epochs` - Deprecated; prefer `lightning_trainer_kwargs={"max_epochs": N}`.
- `data_collate_fn` - Optional custom collate function passed to `Dataset.iter_torch_batches`;
  defaults to Ray Data's column-tensor output.
- `lightning_trainer_kwargs` - Extra keyword arguments forwarded verbatim to
  `pytorch_lightning.Trainer(...)`.
- `transfer_learning_spec` - Optional warm-start spec describing layer freezing patterns.
- `incremental_training_spec` - Optional spec for continuing from an existing run.
- `initial_weights_path` - Optional path to a state dict file (local, `s3://`, `gs://`, etc.); loaded on
  rank 0 and broadcast to other workers.
- `training_observer` - Optional `schema.TrainingObserver` that receives `on_result` (driver-side,
  after training) and `on_checkpoint_saved` (worker-side, per epoch/step). Must
  be picklable if per-epoch observation is needed.
- `experiment_store` - Optional `schema.ExperimentStore` enabling opt-in auto-resume. When set (and
  `run_config` carries both a `name` and a `storage_path`), the trainer records
  this run's experiment directory on rank 0 and, on a re-run with the same
  identity, seeds training from the previously recorded directory if it holds a
  restorable checkpoint. Note that Ray Train V2 also resumes natively from a
  reused run directory (`storage_path/name`); the store additionally covers the
  case where Ray's native checkpoint state is unavailable and provides a
  pluggable, backend-agnostic record of resumable runs. Defaults to `None` (no
  tracking, no store-driven resume). Use
  `experiment_store.FsspecExperimentStore` for the filesystem default. Must be
  picklable (serialized to workers for tracking).
- `profiler_sink` - Optional callable invoked on each node-local rank 0 after `fit()` returns, as
  `profiler_sink(profiler, profiler_logs_path, logger)`, where `profiler` is the
  profiler built from `lightning_trainer_kwargs["profiler"]`,
  `profiler_logs_path` is the directory it wrote to, and `logger` is the
  resolved Lightning logger. Use it to ship profiler output to an experiment
  tracker; `_private.util.comet_profiler_sink` does this for Comet and
  `_private.util.mlflow_profiler_sink` does this for MLflow. Ignored when no
  profiler is configured or when the profiler config sets
  `upload_profiler_results: False`. Exceptions raised by the sink are logged and
  swallowed. Must be picklable (serialized to workers).

#### num\_epochs

type: ignore[assignment]  # sentinel replaced in __post_init__

#### \_\_post\_init\_\_

```python
def __post_init__()
```

Apply default `num_epochs` and warn on the deprecated field usage.

## LightningTrainer Objects

```python
class LightningTrainer(TorchTrainer)
```

Ray `TorchTrainer` subclass that runs a PyTorch Lightning training loop.

#### \_\_init\_\_

```python
def __init__(trainer_param: LightningTrainerParam,
             run_config: ray.train.RunConfig | None = None,
             scaling_config: ray.train.ScalingConfig | None = None)
```

Initialize the trainer.

**Arguments**:

- `trainer_param` - Training configuration (model factory, datasets, etc.).
- `run_config` - Optional Ray `RunConfig` (storage path, run name, ...).
- `scaling_config` - Optional Ray `ScalingConfig` (num_workers, GPU/CPU
  requests, ...).

#### train

```python
def train(run_config: ray.train.RunConfig | None = None,
          scaling_config: ray.train.ScalingConfig | None = None) -> dict
```

Run training and return a small result dict.

**Arguments**:

- `run_config` - Optional override applied before `fit()`.
- `scaling_config` - Optional override applied before `fit()`.
  

**Returns**:

Dict with `checkpoint_path` (path to the latest checkpoint), `path` (the Ray
result path), and `metrics`.
  

**Raises**:

Exception: Whatever Ray Train reports in `result.error`.

## LightningTrainerWithStateDict Objects

```python
class LightningTrainerWithStateDict(LightningTrainer)
```

LightningTrainer that loads the trained checkpoint into a torch model.

After `train()` completes, callers can pass an initialized `torch.nn.Module`
to `update_model_state_dict` and have it populated from the latest
checkpoint. Supports DDP single-file checkpoints, DeepSpeed ZeRO sharded
directories, and FSDP2 distributed checkpoints.

#### update\_model\_state\_dict

```python
def update_model_state_dict(torch_model: torch.nn.Module) -> None
```

Populate `torch_model` in-place from the latest training checkpoint.

**Arguments**:

- `torch_model` - Model whose `state_dict` will be replaced.
  

**Raises**:

ValueError: If `train()` has not been called yet.

