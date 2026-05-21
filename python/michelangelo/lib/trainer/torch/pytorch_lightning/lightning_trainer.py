"""Public PyTorch Lightning trainer wrapping Ray Train.

This package is a one-time snapshot of an internal trainer used for distributed
PyTorch Lightning training on Ray. Bugs may be patched in OSS, but new features
are not automatically backported from the source. See ``CONTRIBUTING.md`` for
the support policy.

Typical use::

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
"""

from __future__ import annotations

import logging
import os
import uuid
from contextlib import contextmanager
from dataclasses import asdict, dataclass, field
from typing import TYPE_CHECKING, Callable

import ray
import torch
from pytorch_lightning.utilities.deepspeed import (
    convert_zero_checkpoint_to_fp32_state_dict,
)
from ray.train.torch import TorchTrainer

from michelangelo.lib.trainer.torch.pytorch_lightning._private.util import (
    _train_loop_per_worker,
)

if TYPE_CHECKING:
    from michelangelo.lib.trainer.torch.pytorch_lightning.schema import (
        IncrementalTrainingSpec,
        TransferLearningSpec,
    )

_logger = logging.getLogger(__name__)
CHECKPOINT_NAME = ray.train.lightning.RayTrainReportCallback.CHECKPOINT_NAME
CHECKPOINT_PATH_KEY = "checkpoint_path"
_UNSET = object()


@dataclass
class CometParam:
    """Configuration for the Comet ML logger.

    Credentials are forwarded as-is to ``comet_ml``; ``api_key`` should be treated
    as a secret. ``comet_ml`` is imported lazily inside the worker, so this
    dataclass can be constructed even when ``comet_ml`` is not installed (the
    import only fires when the trainer actually attaches the logger).

    Attributes:
        api_key: Comet API key for the target workspace.
        project_name: Comet project name to log under.
        experiment_name: Display name for the experiment in the Comet UI.
        workspace: Comet workspace owning the project.
        tags: Optional list of tags to attach to the experiment.
    """

    api_key: str
    project_name: str
    experiment_name: str
    workspace: str
    tags: list[str] | None = field(default_factory=list)


@dataclass
class LightningTrainerParam:
    """Configuration for :class:`LightningTrainer`.

    All callables (``create_model_fn``, ``data_collate_fn``) are invoked inside the
    Ray Train worker. The model is constructed on each worker via
    ``create_model_fn(**create_model_fn_kwargs)`` rather than being pickled across
    process boundaries.

    Attributes:
        create_model_fn: Factory returning a ``pytorch_lightning.LightningModule``.
            Invoked on each worker with ``**create_model_fn_kwargs``.
        create_model_fn_kwargs: Keyword arguments passed to ``create_model_fn``.
        train_data: Training Ray Dataset.
        val_data: Validation Ray Dataset.
        batch_size: Per-worker training batch size.
        num_shuffle_batches: Number of batches kept in the Ray Data local shuffle
            buffer. ``0`` disables shuffling.
        num_epochs: Deprecated; prefer ``lightning_trainer_kwargs={"max_epochs": N}``.
        data_collate_fn: Optional custom collate function passed to
            ``Dataset.iter_torch_batches``; defaults to Ray Data's column-tensor
            output.
        comet_param: Optional :class:`CometParam`; when set, a CometLogger is
            attached on each worker.
        lightning_trainer_kwargs: Extra keyword arguments forwarded verbatim to
            ``pytorch_lightning.Trainer(...)``.
        transfer_learning_spec: Optional warm-start spec describing layer freezing
            patterns.
        incremental_training_spec: Optional spec for continuing from an existing
            run.
        initial_weights_path: Optional path to a state dict file (local, ``s3://``,
            ``gs://``, etc.); loaded on rank 0 and broadcast to other workers.
    """

    create_model_fn: Callable
    create_model_fn_kwargs: dict
    train_data: ray.data.Dataset
    val_data: ray.data.Dataset
    batch_size: int = 8
    num_shuffle_batches: int = (
        10  # By default we reserve 10 batches in ray data shuffle buffer.
    )
    num_epochs: int | None = field(default=_UNSET)  # type: ignore[assignment]  # sentinel replaced in __post_init__
    data_collate_fn: Callable | None = None
    comet_param: CometParam | None = None
    lightning_trainer_kwargs: dict = field(default_factory=dict)

    transfer_learning_spec: TransferLearningSpec | None = None
    incremental_training_spec: IncrementalTrainingSpec | None = None
    initial_weights_path: str | None = None

    def __post_init__(self):
        """Apply default ``num_epochs`` and warn on the deprecated field usage."""
        if self.num_epochs is _UNSET:
            self.num_epochs = 1
        else:
            _logger.warning(
                "LightningTrainerParam.num_epochs is deprecated. "
                "Use LightningTrainerParam.lightning_trainer_kwargs={'max_epochs': N} instead."
            )


class LightningTrainer(TorchTrainer):
    """Ray ``TorchTrainer`` subclass that runs a PyTorch Lightning training loop."""

    def __init__(
        self,
        trainer_param: LightningTrainerParam,
        run_config: ray.train.RunConfig | None = None,
        scaling_config: ray.train.ScalingConfig | None = None,
    ):
        """Initialize the trainer.

        Args:
            trainer_param: Training configuration (model factory, datasets, etc.).
            run_config: Optional Ray ``RunConfig`` (storage path, run name, ...).
            scaling_config: Optional Ray ``ScalingConfig`` (num_workers, GPU/CPU
                requests, ...).
        """
        self.trainer_param = trainer_param
        _logger.info(
            "LightningTrainer initialized with trainer_param: %r", trainer_param
        )
        train_loop_config = asdict(trainer_param)
        # Unique run id for Comet experiment
        train_loop_config["run_id"] = str(uuid.uuid4())
        # Pop out train and val data since we have to pass them into datasets parameter of TorchTrainer.
        train_data = train_loop_config.pop("train_data")
        val_data = train_loop_config.pop("val_data")

        super().__init__(
            train_loop_per_worker=_train_loop_per_worker,
            train_loop_config=train_loop_config,
            scaling_config=scaling_config,
            run_config=run_config,
            datasets={"train": train_data, "val": val_data},
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
            Dict with ``checkpoint_path`` (path to the latest checkpoint),
            ``path`` (the Ray result path), and ``metrics``.

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

        # The user-supplied LightningModule is captured in result.metrics["config"]
        # and is generally not serializable across worker boundaries. Drop it.
        result.metrics.pop("config", None)
        # Keep the checkpoint object for subclasses that need it (e.g., LightningTrainerWithStateDict)
        self.checkpoint = result.checkpoint
        return {
            CHECKPOINT_PATH_KEY: result.checkpoint.path,
            "path": result.path,
            "metrics": result.metrics,
        }


class LightningTrainerWithStateDict(LightningTrainer):
    """LightningTrainer that loads the trained checkpoint into a torch model.

    After ``train()`` completes, callers can pass an initialized ``torch.nn.Module``
    to :meth:`update_model_state_dict` and have it populated from the latest
    checkpoint. Supports both DDP single-file checkpoints and DeepSpeed ZeRO
    sharded directories.
    """

    def _is_deepspeed_strategy(self) -> bool:
        """Return ``True`` if the configured strategy is DeepSpeed."""
        strategy = self.trainer_param.lightning_trainer_kwargs.get("strategy")
        if strategy is None:
            return False

        # DeepSpeed was used if the strategy is "deepspeed" or a RayDeepSpeedStrategy instance
        if isinstance(strategy, str):
            return strategy.lower() == "deepspeed"

        try:
            from ray.train.lightning import RayDeepSpeedStrategy

            return isinstance(strategy, RayDeepSpeedStrategy)
        except ImportError:
            return False

    def update_model_state_dict(self, torch_model: torch.nn.Module) -> None:
        """Populate ``torch_model`` in-place from the latest training checkpoint.

        Args:
            torch_model: Model whose ``state_dict`` will be replaced.

        Raises:
            ValueError: If ``train()`` has not been called yet.
        """
        if not hasattr(self, "checkpoint") or self.checkpoint is None:
            raise ValueError(
                "No checkpoint available. Please call train() first to generate a checkpoint."
            )
        used_deepspeed = self._is_deepspeed_strategy()
        # use the ray checkpoint as_directory() to get the local temp checkpoint directory
        with self.checkpoint.as_directory() as d:
            _logger.info(
                "Saving Ray Checkpoint to local temp Checkpoint directory: %s", d
            )
            data_dir_contents = os.listdir(d)
            _logger.info("Data directory contents: %s", data_dir_contents)
            lightning_ckpt_path = os.path.join(d, CHECKPOINT_NAME)
            if used_deepspeed:
                local_model_path = os.path.join(lightning_ckpt_path, "model.pt")
                # PyTorch 2.6+ defaults weights_only=True, which rejects arbitrary Python classes
                # (LossScaler, DynamicLossScaler, optimizer states, etc.) embedded in DeepSpeed ZeRO
                # checkpoints. The env var reverts the default for any torch.load call that doesn't
                # explicitly pass weights_only, covering both pytorch_lightning and deepspeed internals.
                # TODO: Remove this once we upgrade to Lightning 2.6+ https://github.com/Lightning-AI/pytorch-lightning/pull/21194
                with _torch_weights_only_disabled():
                    model_state_dict = convert_zero_checkpoint_to_fp32_state_dict(
                        lightning_ckpt_path, local_model_path
                    )
                _logger.info(
                    "Loaded DeepSpeed checkpoint from %s to %s",
                    lightning_ckpt_path,
                    local_model_path,
                )
            else:
                # DDP checkpoint
                checkpoint = torch.load(lightning_ckpt_path, map_location="cpu")
                model_state_dict = checkpoint["state_dict"]
                _logger.info("Loaded DDP checkpoint from %s", lightning_ckpt_path)
            torch_model.load_state_dict(model_state_dict, strict=False)
            _logger.info("Updated the state dict of the torch model.")


@contextmanager
def _torch_weights_only_disabled():
    """Force ``torch.load()`` to use ``weights_only=False`` for callers that don't pass it explicitly."""
    key = "TORCH_FORCE_NO_WEIGHTS_ONLY_LOAD"
    old = os.environ.pop(key, None)
    os.environ[key] = "1"
    try:
        yield
    finally:
        if old is not None:
            os.environ[key] = old
        else:
            os.environ.pop(key, None)
