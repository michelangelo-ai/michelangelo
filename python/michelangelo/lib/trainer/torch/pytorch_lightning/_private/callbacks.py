# ruff: noqa: I001
import os
import ray
import ray.train.lightning
import shutil

from pathlib import Path
from ray.train import Checkpoint


class RayTrainReportCallback(ray.train.lightning.RayTrainReportCallback):
    """
    We follow existing implementation of RayTrainReportCallback,
    only force rank zero to report checkpoint.
    Reference: https://docs.ray.io/en/latest/_modules/ray/train/lightning/_lightning_utils.html#RayTrainReportCallback
    """

    def __init__(self) -> None:
        super().__init__()
        self.world_rank = ray.train.get_context().get_world_rank()

    def on_train_epoch_end(self, trainer, pl_module) -> None:  # noqa: ARG002
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

        if self.world_rank == 0:
            ray.train.report(metrics=metrics, checkpoint=checkpoint)
        else:
            ray.train.report(metrics=metrics, checkpoint=None)

        # Add a barrier to ensure all workers finished reporting here
        trainer.strategy.barrier()

        if self.local_rank == 0:
            shutil.rmtree(tmpdir)


class RayTrainReportPerNodeCallback(RayTrainReportCallback):
    """
    We derive from RayTrainReportCallback, but report checkpoint per node instead of on head rank.
    Report per node is necessary for model parallelism for deepspeed zeros and FSDP.
    Also supports step-wise checkpointing in addition to epoch-based checkpointing.
    """

    def __init__(self, step_checkpoint_frequency: int = 0) -> None:
        """
        Args:
            step_checkpoint_frequency: How often to create checkpoints during training steps.
                Default is 0 steps. Set to 0 to disable step-wise checkpointing.
        """
        super().__init__()
        self.step_checkpoint_frequency = step_checkpoint_frequency
        self.last_step_checkpoint = 0

    def on_train_batch_end(self, trainer, *args, **kwargs) -> None:  # noqa: ARG002
        """Called when the train batch ends."""
        if self.step_checkpoint_frequency > 0:
            current_step = trainer.global_step
            if current_step - self.last_step_checkpoint >= self.step_checkpoint_frequency:
                checkpoint_id = f"step_{trainer.global_step}"
                self._create_and_report_checkpoint(trainer, checkpoint_id, is_step_checkpoint=True)
                self.last_step_checkpoint = current_step

    def on_train_epoch_end(self, trainer, pl_module) -> None:  # noqa: ARG002
        """Called when the train epoch ends."""
        checkpoint_id = f"epoch_{trainer.current_epoch}"
        self._create_and_report_checkpoint(trainer, checkpoint_id, is_step_checkpoint=False)

    def _create_and_report_checkpoint(self, trainer, checkpoint_id: str, is_step_checkpoint: bool) -> None:
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
        metrics.update({"epoch": trainer.current_epoch, "step": trainer.global_step, "is_step_checkpoint": is_step_checkpoint})

        # Save checkpoint and report to Ray Train
        ckpt_path = Path(tmpdir, self.CHECKPOINT_NAME).as_posix()
        trainer.save_checkpoint(ckpt_path, weights_only=False)
        checkpoint = Checkpoint.from_directory(tmpdir)

        if self.local_rank == 0:
            ray.train.report(metrics=metrics, checkpoint=checkpoint)
        else:
            ray.train.report(metrics=metrics, checkpoint=None)

        # Ensure all workers finished reporting and cleanup
        trainer.strategy.barrier()
        if self.local_rank == 0:
            shutil.rmtree(tmpdir)
