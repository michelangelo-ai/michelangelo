# ruff: noqa: I001
import logging
import os
import ray
import torch
import uuid

from dataclasses import (
    asdict,
    dataclass,
    field,
)
from ray.train.torch import TorchTrainer
from typing import Callable, Optional

from michelangelo.lib.trainer.torch.pytorch_lightning.schema import (
    IncrementalTrainingSpec,
    TransferLearningSpec,
)
from michelangelo.lib.trainer.torch.pytorch_lightning._private.util import (
    _train_loop_per_worker,
)
from pytorch_lightning.utilities.deepspeed import convert_zero_checkpoint_to_fp32_state_dict
from contextlib import contextmanager

_logger = logging.getLogger(__name__)
CHECKPOINT_NAME = ray.train.lightning.RayTrainReportCallback.CHECKPOINT_NAME
CHECKPOINT_PATH_KEY = "checkpoint_path"
_UNSET = object()


@dataclass
class CometParam:
    api_key: str
    project_name: str
    experiment_name: str
    workspace: str
    tags: list[str] = None


@dataclass
class LightningTrainerParam:
    create_model_fn: Callable
    create_model_fn_kwargs: dict
    train_data: ray.data.Dataset
    val_data: ray.data.Dataset
    batch_size: int = 8
    num_shuffle_batches: int = 10  # By default we reserve 10 batches in ray data shuffle buffer.
    num_epochs: Optional[int] = field(default=_UNSET)  # type: ignore[assignment]  # sentinel replaced in __post_init__
    data_collate_fn: Callable = None
    comet_param: CometParam = None
    lightning_trainer_kwargs: dict = field(default_factory=dict)

    transfer_learning_spec: Optional[TransferLearningSpec] = None
    incremental_training_spec: Optional[IncrementalTrainingSpec] = None
    initial_weights_path: Optional[str] = None

    # Raise warning if the deprecated num_epochs field is set. We default to 1 epoch for backwards compatibility.
    def __post_init__(self):
        if self.num_epochs is _UNSET:
            self.num_epochs = 1
        else:
            _logger.warning(
                "LightningTrainerParam.num_epochs is deprecated. Use LightningTrainerParam.lightning_trainer_kwargs={'max_epochs': N} instead."
            )


class LightningTrainer(TorchTrainer):
    def __init__(
        self,
        trainer_param: LightningTrainerParam,
        run_config: Optional[ray.train.RunConfig] = None,
        scaling_config: Optional[ray.train.ScalingConfig] = None,
    ):
        self.trainer_param = trainer_param
        _logger.info("LightningTrainer initialized with trainer_param: %r", trainer_param)
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
        run_config: Optional[ray.train.RunConfig] = None,
        scaling_config: Optional[ray.train.ScalingConfig] = None,
    ) -> dict:
        if scaling_config is not None:
            self.scaling_config = scaling_config
        if run_config is not None:
            self.run_config = run_config

        result = self.fit()
        if result.error:
            raise result.error

        # User-specified LightningModule is saved in config field and cannot be serialized on uniflow for now.
        # We take the config out.
        result.metrics.pop("config", None)
        # Keep the checkpoint object for subclasses that need it (e.g., LightningTrainerWithStateDict)
        self.checkpoint = result.checkpoint
        return {
            CHECKPOINT_PATH_KEY: result.checkpoint.path,
            "path": result.path,
            "metrics": result.metrics,
        }


class LightningTrainerWithStateDict(LightningTrainer):
    """
    LightningTrainer that provides functions to update model state dict from checkpoint.
    """

    def _is_deepspeed_strategy(self) -> bool:
        """Check if DeepSpeed was used in the training configuration."""
        strategy = self.trainer_param.lightning_trainer_kwargs.get("strategy")
        if strategy is None:
            return False

        # DeepSpeed was used if the strategy is "deepspeed" or a RayDeepSpeedStrategy instance
        if isinstance(strategy, str):
            return strategy.lower() == "deepspeed"

        try:
            from ray.train.lightning import RayDeepSpeedStrategy  # noqa: PLC0415

            return isinstance(strategy, RayDeepSpeedStrategy)
        except ImportError:
            return False

    def update_model_state_dict(self, torch_model: torch.nn.Module):
        """
        Update the model state dict with the local checkpoint.
        """
        if not hasattr(self, "checkpoint") or self.checkpoint is None:
            raise ValueError("No checkpoint available. Please call train() first to generate a checkpoint.")
        used_deepspeed = self._is_deepspeed_strategy()
        # use the ray checkpoint as_directory() to get the local temp checkpoint directory
        with self.checkpoint.as_directory() as d:
            _logger.info(f"Saving Ray Checkpoint to local temp Checkpoint directory: {d}")
            data_dir_contents = os.listdir(d)
            _logger.info(f"Data directory contents: {data_dir_contents}")
            lightning_ckpt_path = os.path.join(d, CHECKPOINT_NAME)
            if used_deepspeed:
                local_model_path = os.path.join(lightning_ckpt_path, "model.pt")
                # PyTorch 2.6+ defaults weights_only=True, which rejects arbitrary Python classes
                # (LossScaler, DynamicLossScaler, optimizer states, etc.) embedded in DeepSpeed ZeRO
                # checkpoints. The env var reverts the default for any torch.load call that doesn't
                # explicitly pass weights_only, covering both pytorch_lightning and deepspeed internals.
                # TODO: Remove this once we upgrade to Lightning 2.6+ https://github.com/Lightning-AI/pytorch-lightning/pull/21194
                with _torch_weights_only_disabled():
                    model_state_dict = convert_zero_checkpoint_to_fp32_state_dict(lightning_ckpt_path, local_model_path)
                _logger.info(f"Loaded DeepSpeed checkpoint from {lightning_ckpt_path} to {local_model_path}")
            else:
                # DDP checkpoint
                checkpoint = torch.load(lightning_ckpt_path, map_location="cpu")
                model_state_dict = checkpoint["state_dict"]
                _logger.info(f"Loaded DDP checkpoint from {lightning_ckpt_path}")
            torch_model.load_state_dict(model_state_dict, strict=False)
            _logger.info("Updated the state dict of the torch model in the ModelVariable")


@contextmanager
def _torch_weights_only_disabled():
    """Force torch.load() to use weights_only=False for call sites that don't pass it explicitly."""
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
