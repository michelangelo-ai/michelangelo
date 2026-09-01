---
sidebar_position: 1.5
sidebar_label: "Distributed Training"
---

# Distributed Training with LightningTrainer

`LightningTrainer` runs a PyTorch Lightning training loop across a Ray cluster. It is a subclass of Ray's `TorchTrainer` that handles worker setup, dataset sharding, distributed strategy wiring, and checkpointing, so your code only supplies a `LightningModule` factory and two Ray Datasets.

This guide is for ML engineers and data scientists who have trained a model with the SDK and now need to scale it across multiple workers or GPUs. If you haven't run a training task yet, start with the [Model Training Guide](./train-and-register-a-model.md) — this page picks up where its Lightning section leaves off.

## What you'll learn

* How `RayTask`, `ScalingConfig`, and `LightningTrainerParam` divide responsibility for a distributed run
* Every `LightningTrainerParam` field, and the Lightning defaults the trainer changes out from under you
* How to pick between DDP, FSDP, FSDP2, and DeepSpeed
* How to load trained weights back into a plain `torch.nn.Module`
* How to warm-start from existing weights and auto-resume an interrupted run
* How to observe a run with loggers, observers, and profiler sinks

## Prerequisites

- **A prepared dataset** — training and validation data as Ray Datasets. See [Data Preparation](../getting-started/prepare-your-data.md).
- **The trainer extra installed** — `cd python && poetry install -E trainer` from the repo root. Some features need additional extras: `trainer-deepspeed` for the DeepSpeed strategy, and `trainer-mlflow` or `trainer-comet` for the matching profiler sinks.
- **A running sandbox** for remote runs. See [Sandbox Setup](../../getting-started/sandbox-setup.md).

## How the pieces fit together

Three layers of configuration control a distributed run, and they are easy to confuse:

| Layer | Type | Controls |
| --- | --- | --- |
| `RayTask` | `@uniflow.task(config=...)` | The **Ray cluster** the task runs on — head/worker pod sizes and count |
| `ScalingConfig` | `ray.train.ScalingConfig` | The **Ray Train workers** scheduled onto that cluster — how many training processes, and their per-worker resources |
| `LightningTrainerParam` | `michelangelo.lib.trainer.torch.pytorch_lightning` | The **training run itself** — model factory, data, batch size, Lightning arguments |

`RayTask` provisions the cluster; `ScalingConfig` requests a slice of it for training. If `ScalingConfig` asks for more workers or resources than `RayTask` provisioned, training will hang waiting for resources that never arrive.

:::warning
`ScalingConfig(num_workers=N)` must fit inside the cluster `RayTask` creates. A `RayTask(worker_instances=4, worker_cpu=4)` cluster cannot satisfy `ScalingConfig(num_workers=8, resources_per_worker={"CPU": 4})`.
:::

## A minimal training task

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.lib.trainer.torch.pytorch_lightning import (
    LightningTrainer,
    LightningTrainerParam,
)
from michelangelo.uniflow.plugins.ray import RayTask, create_run_config
from michelangelo.workflow.variables import DatasetVariable
from ray.train import CheckpointConfig, ScalingConfig


def create_model(hidden_dim: int, learning_rate: float):
    """Runs on each worker — returns a pytorch_lightning.LightningModule."""
    from my_project.models import MyLightningModule

    return MyLightningModule(hidden_dim=hidden_dim, learning_rate=learning_rate)


@uniflow.task(
    config=RayTask(
        head_cpu=2,
        head_memory="8Gi",
        worker_cpu=4,
        worker_memory="16Gi",
        worker_instances=4,
    )
)
def train(train_dv: DatasetVariable, val_dv: DatasetVariable):
    train_dv.load_ray_dataset()
    val_dv.load_ray_dataset()

    trainer = LightningTrainer(
        trainer_param=LightningTrainerParam(
            create_model_fn=create_model,
            create_model_fn_kwargs={"hidden_dim": 256, "learning_rate": 1e-4},
            train_data=train_dv.value,
            val_data=val_dv.value,
            batch_size=32,
            lightning_trainer_kwargs={
                "max_epochs": 5,
                "precision": "16-mixed",
                "log_every_n_steps": 10,
            },
        ),
        run_config=create_run_config(
            name="my-training-run",
            checkpoint_config=CheckpointConfig(
                num_to_keep=1,
                checkpoint_score_attribute="val_loss",
                checkpoint_score_order="min",
            ),
        ),
        scaling_config=ScalingConfig(
            num_workers=4,
            use_gpu=True,
            resources_per_worker={"CPU": 4},
        ),
    )

    result = trainer.train()
    return result["checkpoint_path"]
```

`train()` returns a dict with three keys:

| Key | Description |
| --- | --- |
| `checkpoint_path` | Path to the latest checkpoint |
| `path` | The Ray result path for the run |
| `metrics` | Final metrics reported by the run |

:::note
`create_model_fn` is invoked **on each worker** as `create_model_fn(**create_model_fn_kwargs)`. The model is never pickled across process boundaries, so import heavy model dependencies inside the factory rather than at module scope.
:::

## LightningTrainerParam reference

**Required:**

| Parameter | Type | Description |
| --- | --- | --- |
| `create_model_fn` | `Callable` | Factory returning a `LightningModule`, called on each worker |
| `create_model_fn_kwargs` | `dict` | Keyword arguments passed to the factory |
| `train_data` | `ray.data.Dataset` | Training dataset |
| `val_data` | `ray.data.Dataset` | Validation dataset |

**Optional:**

| Parameter | Default | Description |
| --- | --- | --- |
| `batch_size` | `8` | Per-worker training batch size |
| `num_shuffle_batches` | `10` | Batches held in the Ray Data local shuffle buffer. `0` disables shuffling |
| `data_collate_fn` | `None` | Custom collate function; defaults to Ray Data's column-tensor output |
| `lightning_trainer_kwargs` | `{}` | Forwarded to `pytorch_lightning.Trainer(...)` — see below |
| `initial_weights_path` | `None` | State dict to warm-start from (local, `s3://`, `gs://`, …) |
| `transfer_learning_spec` | `None` | Layer inheritance and freezing — see [Warm starts](#warm-starts) |
| `incremental_training_spec` | `None` | Continue training from a registered baseline model |
| `training_observer` | `None` | Callbacks for training events — see [Observing a run](#observing-a-run) |
| `experiment_store` | `None` | Enables opt-in auto-resume — see [Auto-resume](#auto-resume-across-runs) |
| `profiler_sink` | `None` | Ships profiler output to an experiment tracker |

:::warning
`num_epochs` is deprecated and logs a warning when set. Use `lightning_trainer_kwargs={"max_epochs": N}` instead.
:::

### Defaults that differ from Lightning

The trainer applies three defaults before constructing `pytorch_lightning.Trainer`. Each is a normal `lightning_trainer_kwargs` key you can override:

| Key | Trainer default | Lightning default |
| --- | --- | --- |
| `max_epochs` | `1` | `None`, which Lightning resolves to 1000 |
| `num_sanity_val_steps` | `0` | `2` |
| `enable_progress_bar` | `False` | `True` |

:::warning
`max_epochs` defaults to **1**, not Lightning's usual 1000. A run that looks like it stopped early after one epoch is almost always this default rather than a bug — set `lightning_trainer_kwargs={"max_epochs": N}` explicitly.
:::

`enable_checkpointing` is the exception — it is **derived** from whether a `ModelCheckpoint` callback is present, and setting it directly in `lightning_trainer_kwargs` is ignored with a warning.

## Choosing a distributed strategy

Pass `strategy` inside `lightning_trainer_kwargs`, as either a string or a Lightning `Strategy` instance. Strings resolve to Ray-aware implementations:

| `strategy` | Resolves to | Use when |
| --- | --- | --- |
| `None` or `"ddp"` | `RayDDPStrategy` | The model fits on one device; the default |
| `"fsdp"` | `RayFSDPStrategy` | The model is too large for one device |
| `"fsdp2"` | `RayModelParallelStrategy` | As above, on PyTorch Lightning 2.3+ |
| `"deepspeed"` | `RayDeepSpeedStrategy` | You want ZeRO optimizer/gradient sharding (needs the `trainer-deepspeed` extra) |

Anything else raises a `ValueError`. Strategy constructor arguments go in a sibling `strategy_kwargs` dict:

```python
lightning_trainer_kwargs={
    "strategy": "fsdp",
    "strategy_kwargs": {"sharding_strategy": "SHARD_GRAD_OP"},
    "precision": "16-mixed",
}
```

### FSDP2 constraints

`RayModelParallelStrategy` (`"fsdp2"`) fills a gap — Ray Train ships no `ModelParallelStrategy` equivalent — and deliberately rejects settings that Ray Train already owns. These `strategy_kwargs` raise `ValueError`:

- `tensor_parallel_size` — tensor parallelism is not currently supported
- `data_parallel_size` — FSDP2 always shards across the full world size
- `process_group_backend` and `timeout` — set by Ray Train

`save_distributed_checkpoint=False` is also unsupported; FSDP2 always writes sharded checkpoints and the trainer forces it back to `True` with a warning.

## Loading trained weights into a model

`train()` gives you a checkpoint path, but checkpoint *format* varies by strategy: DDP writes a single file, DeepSpeed writes a sharded ZeRO directory, and FSDP2 writes a distributed checkpoint. `LightningTrainerWithStateDict` absorbs that difference:

```python
from michelangelo.lib.trainer.torch.pytorch_lightning import LightningTrainerWithStateDict

trainer = LightningTrainerWithStateDict(
    trainer_param=trainer_param,
    run_config=run_config,
    scaling_config=scaling_config,
)
trainer.train()

model = MyTorchModel()
trainer.update_model_state_dict(model)  # populated in place
```

The method reads the strategy off `lightning_trainer_kwargs` and picks the right loader — including converting a DeepSpeed ZeRO checkpoint to an fp32 state dict. It raises `ValueError` if called before `train()`.

:::note
Weights load with `strict=False`, so a checkpoint that does not cover every layer in your model will silently leave the remainder at their initialized values.
:::

## Warm starts

Three mechanisms start a run from existing weights, in increasing order of structure.

**`initial_weights_path`** — the simplest. Rank 0 downloads the state dict and broadcasts it to the other workers:

```python
LightningTrainerParam(..., initial_weights_path="s3://my-bucket/baseline/model.pt")
```

**`TransferLearningSpec`** — inherit a subset of layers from a registered baseline and optionally freeze them:

```python
from michelangelo.lib.trainer.torch.pytorch_lightning import (
    LearningMode,
    ModelSpec,
    TransferLearningSpec,
)
from michelangelo.lib.trainer.torch.pytorch_lightning.schema import TransferLearningMetadata

spec = TransferLearningSpec(
    metadata=TransferLearningMetadata(
        learning_mode=LearningMode.TRANSFER_LEARNING,
        baseline_model=ModelSpec(project_name="my-project", model_name="base-encoder"),
    ),
    layer_names_to_inherit_regex=[r"^encoder\..*"],
    layer_names_to_freeze_regex=[r"^encoder\.embeddings\..*"],
)
```

Both exact-name (`layer_names_to_inherit`, `layer_names_to_freeze`) and regex variants are available, and `model_loader_function` accepts a dotted path to override the default baseline loader.

**`IncrementalTrainingSpec`** — continue training a baseline model, optionally restoring optimizer state via `load_optimizer_weights=True` and overriding the starting epoch with `override_incremental_training_epoch`.

:::info
Both specs carry a `fused_model_submodule` field. It is reserved for future use and has no effect today — nothing currently reads it. Leave it unset.
:::

## Auto-resume across runs

Ray Train V2 resumes natively when a run reuses its `storage_path/name` directory. An `ExperimentStore` adds a pluggable fallback for when Ray's native checkpoint state is unavailable:

```python
from michelangelo.lib.trainer.torch.pytorch_lightning import FsspecExperimentStore

trainer = LightningTrainer(
    trainer_param=LightningTrainerParam(..., experiment_store=FsspecExperimentStore()),
    run_config=create_run_config(name="my-training-run", storage_path="s3://my-bucket/runs"),
    scaling_config=scaling_config,
)
```

The store records this run's experiment directory on rank 0, keyed by `(storage_path, run_name)`. A later run with the same identity resolves that directory and seeds from its latest checkpoint — but only if Ray has no native checkpoint to restore, which always takes priority.

Auto-resume is skipped when `RunConfig` lacks either `name` or `storage_path` — the run proceeds normally and the reason is recorded in the logs at `INFO` level, so check there if a resume you expected did not happen. Neither store method may raise; a failed lookup means "nothing to resume" rather than a failed run.

:::warning
Auto-resume resolves at **construction** time, because Ray Train V2 freezes the run context when the trainer is built. Passing `run_config` to `train()` overrides the config but will not re-trigger resumption — pass it to the constructor instead. The trainer logs a warning if you do this with a store configured.
:::

To use a different backend, implement the `ExperimentStore` protocol's `track()` and `locate_resumable()` methods. Implementations must be picklable, since Ray serializes the store to workers.

## Observing a run

**Loggers** pass through `lightning_trainer_kwargs`, with an optional `logger_kwargs` sibling:

```python
from pytorch_lightning.loggers import MLFlowLogger

lightning_trainer_kwargs={"logger": MLFlowLogger(experiment_name="my-experiment")}
```

**`TrainingObserver`** receives structured events. Implement two methods:

```python
class MyObserver:
    def on_result(self, metrics: dict, checkpoint_path: str | None) -> None:
        """Called once on the driver after training completes."""

    def on_checkpoint_saved(
        self, epoch: int, step: int, metrics: dict, checkpoint_path: str
    ) -> None:
        """Called on every worker each time a checkpoint is saved."""
```

:::warning
`on_checkpoint_saved` fires on **all** ranks, not just rank 0. Guard on rank internally or make the implementation idempotent, or side effects like DB writes will be duplicated across workers. The observer must also be picklable.
:::

**`profiler_sink`** ships PyTorch profiler output to a tracker after `fit()` returns, on each node-local rank 0. Two implementations ship with the SDK:

```python
from michelangelo.lib.trainer.torch.pytorch_lightning import (
    comet_profiler_sink,
    mlflow_profiler_sink,
)

LightningTrainerParam(..., profiler_sink=mlflow_profiler_sink)
```

The sink is ignored when no profiler is configured, and exceptions it raises are logged and swallowed rather than failing the run.

## Troubleshooting

| Symptom | Likely cause |
| --- | --- |
| Training hangs before the first epoch | `ScalingConfig` requests more resources than `RayTask` provisioned |
| `ValueError` on an FSDP2 `strategy_kwargs` key | Setting a parallelism or process-group option Ray Train owns — see [FSDP2 constraints](#fsdp2-constraints) |
| Out of memory | Lower `batch_size`, raise `worker_memory`, or switch to `"fsdp"` / `"deepspeed"` |
| Loaded model has untrained layers | `update_model_state_dict` uses `strict=False`; check that the checkpoint's layer names match |
| Resume did not happen | `RunConfig` is missing `name` or `storage_path`, or `run_config` was passed to `train()` instead of the constructor |
| Duplicate side effects per epoch | `on_checkpoint_saved` runs on every worker |

## What's next?

- [**Model Registry**](./model-registry-guide.md) — version and store the trained model
- [**Deploy a Model**](./deploy-a-model.md) — serve it for inference
- [**Examples**](../examples/index.md) — working distributed runs, including GPT fine-tuning with LoRA and Nomic embedding training
- [**Python SDK Reference**](../../api-reference/python-sdk/index.md) — generated signatures for the trainer and the wider SDK surface
