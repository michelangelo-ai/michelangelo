"""Public XGBoost trainer wrapping Ray Train.

Distributed, data-parallel XGBoost training on a Ray cluster, exposed with the
same shape as the PyTorch Lightning trainer in
``michelangelo.lib.trainer.torch.pytorch_lightning``: a parameter dataclass, a
``train()`` that returns a small result dict, and checkpointing handled by Ray
Train's own report callback.

Typical use::

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
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from typing import Any

import ray.data
import ray.train
from ray.train.xgboost import XGBoostTrainer as RayXGBoostTrainer

_logger = logging.getLogger(__name__)

CHECKPOINT_PATH_KEY = "checkpoint_path"

TRAIN_DATASET_KEY = "train"
VALIDATION_DATASET_KEY = "validation"


@dataclass
class XGBoostTrainerParam:
    """Configuration for :class:`XGBoostTrainer`.

    Attributes:
        label_column: Name of the target column. Must be present in
            ``train_data`` and, when supplied, ``val_data``.
        train_data: Training Ray Dataset.
        val_data: Optional validation Ray Dataset. When omitted, training runs
            with the training set as its only eval set.
        params: XGBoost booster parameters, passed verbatim as the first
            argument to ``xgboost.train`` (for example
            ``{"objective": "reg:squarederror", "eta": 0.1}``).
        num_boost_round: Number of boosting rounds.
        feature_columns: Explicit feature column names. When ``None`` (the
            default), every column except ``label_column`` is used as a
            feature.
        xgboost_train_kwargs: Extra keyword arguments forwarded verbatim to
            ``xgboost.train(...)``. ``callbacks`` is reserved -- the trainer
            supplies Ray Train's report callback, so passing your own here
            raises ``ValueError`` rather than silently losing checkpointing.
    """

    label_column: str
    train_data: ray.data.Dataset
    val_data: ray.data.Dataset | None = None
    params: dict[str, Any] = field(default_factory=dict)
    num_boost_round: int = 10
    feature_columns: list[str] | None = None
    xgboost_train_kwargs: dict[str, Any] = field(default_factory=dict)

    def __post_init__(self) -> None:
        """Reject configurations that would silently drop checkpointing.

        Raises:
            ValueError: If ``label_column`` is empty, ``num_boost_round`` is
                not positive, or ``xgboost_train_kwargs`` carries ``callbacks``
                (which would collide with the report callback the trainer
                installs).
        """
        if not self.label_column:
            raise ValueError("label_column must be a non-empty column name")
        if self.num_boost_round <= 0:
            raise ValueError(
                f"num_boost_round must be positive, got {self.num_boost_round}"
            )
        if "callbacks" in self.xgboost_train_kwargs:
            raise ValueError(
                "xgboost_train_kwargs must not set 'callbacks'; the trainer "
                "installs Ray Train's RayTrainReportCallback, and overriding it "
                "disables checkpoint reporting"
            )


def _train_loop_per_worker(config: dict[str, Any]) -> None:
    """Train one XGBoost worker against its shard of the datasets.

    Runs inside each Ray Train worker. Imports are deferred to worker-side so
    that importing this module does not require ``xgboost`` on the driver.

    Args:
        config: The ``train_loop_config`` assembled by
            :meth:`XGBoostTrainer.__init__`.
    """
    import xgboost
    from ray.train.xgboost import RayTrainReportCallback

    label_column = config["label_column"]
    feature_columns = config["feature_columns"]
    params = config["params"]
    num_boost_round = config["num_boost_round"]
    extra_kwargs = config["xgboost_train_kwargs"]

    train_df = ray.train.get_dataset_shard(TRAIN_DATASET_KEY).materialize().to_pandas()

    if feature_columns is None:
        feature_columns = [c for c in train_df.columns if c != label_column]
    if label_column not in train_df.columns:
        raise KeyError(
            f"label_column {label_column!r} is not present in the training "
            f"shard; available columns: {sorted(train_df.columns)}"
        )

    dtrain = xgboost.DMatrix(train_df[feature_columns], label=train_df[label_column])
    evals = [(dtrain, TRAIN_DATASET_KEY)]

    # Looked up off the flag rather than probing for the shard, so behaviour
    # does not depend on whether get_dataset_shard returns None or raises for
    # an absent dataset.
    if config["has_validation"]:
        val_df = (
            ray.train.get_dataset_shard(VALIDATION_DATASET_KEY)
            .materialize()
            .to_pandas()
        )
        dval = xgboost.DMatrix(val_df[feature_columns], label=val_df[label_column])
        evals.append((dval, VALIDATION_DATASET_KEY))

    xgboost.train(
        params,
        dtrain=dtrain,
        evals=evals,
        num_boost_round=num_boost_round,
        callbacks=[RayTrainReportCallback()],
        **extra_kwargs,
    )


class XGBoostTrainer(RayXGBoostTrainer):
    """Ray ``XGBoostTrainer`` subclass running a data-parallel XGBoost fit."""

    def __init__(
        self,
        trainer_param: XGBoostTrainerParam,
        run_config: ray.train.RunConfig | None = None,
        scaling_config: ray.train.ScalingConfig | None = None,
    ):
        """Initialize the trainer.

        Args:
            trainer_param: Training configuration (label, datasets, booster
                params, ...).
            run_config: Optional Ray ``RunConfig`` (storage path, run name, ...).
            scaling_config: Optional Ray ``ScalingConfig`` (num_workers, GPU/CPU
                requests, ...).
        """
        self.trainer_param = trainer_param
        _logger.info("XGBoostTrainer initialized with trainer_param: %r", trainer_param)

        # Built field-by-field rather than with dataclasses.asdict(), which
        # would deep-copy the Ray Datasets before they are split back out.
        train_loop_config = {
            "label_column": trainer_param.label_column,
            "feature_columns": trainer_param.feature_columns,
            "params": trainer_param.params,
            "num_boost_round": trainer_param.num_boost_round,
            "xgboost_train_kwargs": trainer_param.xgboost_train_kwargs,
            "has_validation": trainer_param.val_data is not None,
        }

        datasets = {TRAIN_DATASET_KEY: trainer_param.train_data}
        if trainer_param.val_data is not None:
            datasets[VALIDATION_DATASET_KEY] = trainer_param.val_data

        super().__init__(
            _train_loop_per_worker,
            train_loop_config=train_loop_config,
            datasets=datasets,
            run_config=run_config,
            scaling_config=scaling_config,
        )

    def train(
        self,
        run_config: ray.train.RunConfig | None = None,
        scaling_config: ray.train.ScalingConfig | None = None,
    ) -> dict:
        """Run training and return a small result dict.

        Args:
            run_config: Optional override applied before ``fit()``.
            scaling_config: Optional override applied before ``fit()``.

        Returns:
            Dict with ``checkpoint_path`` (path to the latest checkpoint, or
            ``None`` if the run reported none), ``path`` (the Ray result path),
            and ``metrics``.

        Raises:
            Exception: Whatever Ray Train reports in ``result.error``.
        """
        if scaling_config is not None:
            self.scaling_config = scaling_config
        if run_config is not None:
            self.run_config = run_config

        result = self.fit()
        if result.error:
            raise result.error

        self.checkpoint = result.checkpoint

        return {
            CHECKPOINT_PATH_KEY: result.checkpoint.path if result.checkpoint else None,
            "path": result.path,
            "metrics": result.metrics,
        }
