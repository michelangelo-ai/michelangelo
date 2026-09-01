---
sidebar_label: xgboost_trainer
title: michelangelo.lib.trainer.xgboost.xgboost_trainer
---

Public XGBoost trainer wrapping Ray Train.

Distributed, data-parallel XGBoost training on a Ray cluster, exposed with the
same shape as the PyTorch Lightning trainer in
`michelangelo.lib.trainer.torch.pytorch_lightning`: a parameter dataclass, a
`train()` that returns a small result dict, and checkpointing handled by Ray
Train's own report callback.

Typical use:

```python
from michelangelo.lib.trainer.xgboost import (
    XGBoostTrainer,
    XGBoostTrainerParam,
)

trainer = XGBoostTrainer(
    trainer_param=XGBoostTrainerParam(
        label_column="target",
        train_data=train_ds,
        val_data=val_ds,
        params={"objective": "reg:squarederror", "eta": 0.1},
        num_boost_round=50,
    ),
    run_config=ray.train.RunConfig(name="my_run", storage_path="/tmp/runs"),
    scaling_config=ray.train.ScalingConfig(num_workers=2, use_gpu=False),
)
result = trainer.train()
print(result["checkpoint_path"])
```
## XGBoostTrainerParam Objects

```python
@dataclass
class XGBoostTrainerParam()
```

Configuration for `XGBoostTrainer`.

**Attributes**:

- `label_column` - Name of the target column. Must be present in `train_data` and, when supplied,
  `val_data`.
- `train_data` - Training Ray Dataset.
- `val_data` - Optional validation Ray Dataset. When omitted, training runs with the training
  set as its only eval set.
- `params` - XGBoost booster parameters, passed verbatim as the first argument to
  `xgboost.train` (for example `{"objective": "reg:squarederror", "eta": 0.1}`).
- `num_boost_round` - Number of boosting rounds.
- `feature_columns` - Explicit feature column names. When `None` (the default), every column except
  `label_column` is used as a feature.
- `xgboost_train_kwargs` - Extra keyword arguments forwarded verbatim to `xgboost.train(...)`.
  `callbacks` is reserved -- the trainer supplies Ray Train's report callback,
  so passing your own here raises `ValueError` rather than silently losing
  checkpointing.

#### \_\_post\_init\_\_

```python
def __post_init__() -> None
```

Reject configurations that would silently drop checkpointing.

**Raises**:

- `ValueError` - If `label_column` is empty, `num_boost_round` is
  not positive, or `xgboost_train_kwargs` carries `callbacks`
  (which would collide with the report callback the trainer
  installs).

## XGBoostTrainer Objects

```python
class XGBoostTrainer(RayXGBoostTrainer)
```

Ray `XGBoostTrainer` subclass running a data-parallel XGBoost fit.

#### \_\_init\_\_

```python
def __init__(trainer_param: XGBoostTrainerParam,
             run_config: ray.train.RunConfig | None = None,
             scaling_config: ray.train.ScalingConfig | None = None)
```

Initialize the trainer.

**Arguments**:

- `trainer_param` - Training configuration (label, datasets, booster
  params, ...).
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

Dict with `checkpoint_path` (path to the latest checkpoint, or `None` if the run
reported none), `path` (the Ray result path), and `metrics`.
  

**Raises**:

Exception: Whatever Ray Train reports in `result.error`.

