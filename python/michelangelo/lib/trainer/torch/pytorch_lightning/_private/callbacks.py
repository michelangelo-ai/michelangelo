"""Ray Train ↔ PyTorch Lightning checkpoint reporting callbacks."""

from __future__ import annotations

import logging
import os
import shutil
from pathlib import Path
from typing import TYPE_CHECKING, Any

import ray
import ray.train.lightning
from ray.train import Checkpoint

try:
    # Native async checkpoint upload; only present in newer Ray releases.
    from ray.train import CheckpointUploadMode

    _NATIVE_ASYNC_UPLOAD_AVAILABLE = True
except ImportError:  # Older Ray: no native async upload.
    CheckpointUploadMode = None  # type: ignore[assignment,misc]
    _NATIVE_ASYNC_UPLOAD_AVAILABLE = False

_logger = logging.getLogger(__name__)

if TYPE_CHECKING:
    from michelangelo.lib.trainer.torch.pytorch_lightning.schema import (
        TrainingObserver,
    )


class RayTrainReportCallback(ray.train.lightning.RayTrainReportCallback):
    """Rank-0-only checkpoint reporting callback.

    Follows the upstream :class:`ray.train.lightning.RayTrainReportCallback`
    implementation but forces only rank zero to report the checkpoint.

    When ``upload_async`` is enabled and the installed Ray supports it, the
    epoch-end checkpoint is reported with
    ``ray.train.CheckpointUploadMode.ASYNC``: Ray Train uploads it on a
    background thread to the run's storage location, deletes the local copy
    afterwards, and flushes pending uploads before the training session ends.
    The upload therefore needs neither a cross-worker barrier nor a local
    cleanup here, and upload failures are raised by Ray Train instead of being
    swallowed. Otherwise the checkpoint is uploaded synchronously.

    Reference:
        https://docs.ray.io/en/latest/_modules/ray/train/lightning/_lightning_utils.html#RayTrainReportCallback.
    """

    # Class-level default so subclasses and test doubles that skip ``__init__``
    # behave synchronously.
    _use_native_async: bool = False

    def __init__(
        self,
        training_observer: TrainingObserver | None = None,
        upload_async: bool = False,
    ) -> None:
        """Initialize the callback.

        Args:
            training_observer: Optional observer notified on checkpoint saves.
            upload_async: Upload the epoch-end checkpoint in the background
                using Ray Train's native async upload. If the installed Ray does
                not provide ``ray.train.CheckpointUploadMode``, a warning is
                logged and the synchronous default is used.
        """
        super().__init__()
        self.world_rank = ray.train.get_context().get_world_rank()
        self.local_rank = ray.train.get_context().get_local_rank()
        self._training_observer = training_observer
        self._use_native_async = bool(upload_async) and _NATIVE_ASYNC_UPLOAD_AVAILABLE
        if upload_async and not _NATIVE_ASYNC_UPLOAD_AVAILABLE:
            _logger.warning(
                "upload_async=True but this Ray version (%s) has no "
                "ray.train.CheckpointUploadMode; falling back to synchronous "
                "checkpoint upload.",
                getattr(ray, "__version__", "unknown"),
            )

    def on_train_epoch_end(self, trainer, pl_module) -> None:
        # Creates a checkpoint dir with fixed name
        tmpdir = Path(self.tmpdir_prefix, str(trainer.current_epoch)).as_posix()
        os.makedirs(tmpdir, exist_ok=True)

        # Fetch metrics
        metrics = trainer.callback_metrics
        metrics = {k: v.item() for k, v in metrics.items()}

        # (Optional) Add customized metrics
        metrics["epoch"] = trainer.current_epoch
        metrics["step"] = trainer.global_step

        # Save checkpoint to local
        ckpt_path = Path(tmpdir, self.CHECKPOINT_NAME).as_posix()
        trainer.save_checkpoint(ckpt_path, weights_only=False)

        # Report to train session
        checkpoint = Checkpoint.from_directory(tmpdir)

        report_kwargs: dict[str, Any] = {}
        if self._use_native_async:
            # Ray uploads in the background and removes the local directory
            # once the upload completes.
            report_kwargs = {
                "checkpoint_upload_mode": CheckpointUploadMode.ASYNC,
                "delete_local_checkpoint_after_upload": True,
            }

        if self.world_rank == 0:
            ray.train.report(metrics=metrics, checkpoint=checkpoint, **report_kwargs)
        else:
            ray.train.report(metrics=metrics, checkpoint=None, **report_kwargs)

        if self._training_observer is not None:
            self._training_observer.on_checkpoint_saved(
                epoch=trainer.current_epoch,
                step=trainer.global_step,
                metrics=metrics,
                checkpoint_path=ckpt_path,
            )

        if self._use_native_async:
            # Ray flushes pending uploads itself and deletes rank 0's local
            # directory after upload, so no barrier or rmtree is needed there.
            # Other ranks attach no checkpoint, so drop their (empty) directory.
            if self.world_rank != 0 and self.local_rank == 0:
                shutil.rmtree(tmpdir, ignore_errors=True)
            return

        # Add a barrier to ensure all workers finished reporting here
        trainer.strategy.barrier()

        if self.local_rank == 0:
            shutil.rmtree(tmpdir)


class RayTrainReportPerNodeCallback(RayTrainReportCallback):
    """Per-node checkpoint reporting callback.

    Derives from :class:`RayTrainReportCallback` but reports the checkpoint per
    node (local rank 0) instead of only on the head rank. Per-node reporting is
    necessary for model parallelism with DeepSpeed ZeRO and FSDP, where each
    node holds a shard of the model state. Also supports step-wise checkpointing
    in addition to epoch-based checkpointing.
    """

    def __init__(
        self,
        step_checkpoint_frequency: int = 0,
        training_observer: TrainingObserver | None = None,
    ) -> None:
        """Initialize the callback.

        Args:
            step_checkpoint_frequency: How often to create checkpoints during
                training steps. Set to 0 to disable step-wise checkpointing.
            training_observer: Optional observer notified on checkpoint saves.
        """
        super().__init__(training_observer=training_observer)
        self.step_checkpoint_frequency = step_checkpoint_frequency
        self.last_step_checkpoint = 0

    def on_train_batch_end(self, trainer, *args, **kwargs) -> None:
        """Called when the train batch ends."""
        if self.step_checkpoint_frequency > 0:
            current_step = trainer.global_step
            if (
                current_step - self.last_step_checkpoint
                >= self.step_checkpoint_frequency
            ):
                checkpoint_id = f"step_{trainer.global_step}"
                self._create_and_report_checkpoint(
                    trainer, checkpoint_id, is_step_checkpoint=True
                )
                self.last_step_checkpoint = current_step

    def on_train_epoch_end(self, trainer, pl_module) -> None:
        """Called when the train epoch ends."""
        checkpoint_id = f"epoch_{trainer.current_epoch}"
        self._create_and_report_checkpoint(
            trainer, checkpoint_id, is_step_checkpoint=False
        )

    def _create_and_report_checkpoint(
        self, trainer, checkpoint_id: str, is_step_checkpoint: bool
    ) -> None:
        """Creates a checkpoint and reports it to Ray Train.

        Args:
            trainer: The PyTorch Lightning trainer instance
            checkpoint_id: Unique identifier for the checkpoint (e.g., epoch number or step number)
            is_step_checkpoint: Whether this is a step-wise checkpoint (True) or epoch checkpoint (False)
        """
        # Create checkpoint directory and prepare metrics
        tmpdir = Path(self.tmpdir_prefix, checkpoint_id).as_posix()
        os.makedirs(tmpdir, exist_ok=True)

        metrics = trainer.callback_metrics
        metrics = {k: v.item() for k, v in metrics.items()}
        metrics.update(
            {
                "epoch": trainer.current_epoch,
                "step": trainer.global_step,
                "is_step_checkpoint": is_step_checkpoint,
            }
        )

        # Save checkpoint and report to Ray Train
        ckpt_path = Path(tmpdir, self.CHECKPOINT_NAME).as_posix()
        trainer.save_checkpoint(ckpt_path, weights_only=False)
        checkpoint = Checkpoint.from_directory(tmpdir)

        if self.local_rank == 0:
            ray.train.report(metrics=metrics, checkpoint=checkpoint)
        else:
            ray.train.report(metrics=metrics, checkpoint=None)

        if self._training_observer is not None:
            self._training_observer.on_checkpoint_saved(
                epoch=trainer.current_epoch,
                step=trainer.global_step,
                metrics=metrics,
                checkpoint_path=ckpt_path,
            )

        # Ensure all workers finished reporting and cleanup
        trainer.strategy.barrier()
        if self.local_rank == 0:
            shutil.rmtree(tmpdir)
