"""Tests for the Ray Train ↔ PyTorch Lightning report callbacks.

Covers ``michelangelo.lib.trainer.torch.pytorch_lightning._private.callbacks``:
``RayTrainReportCallback.on_train_epoch_end`` and
``RayTrainReportPerNodeCallback`` (epoch-end reporting, step-wise checkpointing
via ``on_train_batch_end``, and ``_create_and_report_checkpoint``).

The callbacks subclass ``ray.train.lightning.RayTrainReportCallback`` whose
``__init__`` requires an active Ray Train session. Instances are therefore
built via ``object.__new__`` and the base-class attributes the methods rely on
(``tmpdir_prefix`` / ``CHECKPOINT_NAME`` / ``local_rank``) are set explicitly.
``ray.train`` calls and the Lightning trainer are mocked.
"""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

# Importing the callbacks module requires Ray to be installed. Skip cleanly in
# lightweight environments without it.
pytest.importorskip("ray")
pytest.importorskip("torch")
pytest.importorskip("pytorch_lightning")

from michelangelo.lib.trainer.torch.pytorch_lightning._private.callbacks import (
    RayTrainReportCallback,
    RayTrainReportPerNodeCallback,
)

_CALLBACKS_MODULE = (
    "michelangelo.lib.trainer.torch.pytorch_lightning._private.callbacks"
)


def _make_metric(value):
    """Build a fake metric tensor exposing ``.item()`` like a torch scalar."""
    metric = MagicMock()
    metric.item.return_value = value
    return metric


def _make_trainer(current_epoch=0, global_step=0, metrics=None):
    """Build a mock Lightning trainer with the attributes the callbacks read."""
    trainer = MagicMock()
    trainer.current_epoch = current_epoch
    trainer.global_step = global_step
    trainer.callback_metrics = metrics or {}
    return trainer


def _new_default_callback(local_rank=0, world_rank=0, tmpdir_prefix="/tmp/ckpt"):
    """Construct a ``RayTrainReportCallback`` without running base ``__init__``."""
    cb = object.__new__(RayTrainReportCallback)
    cb.tmpdir_prefix = tmpdir_prefix
    cb.CHECKPOINT_NAME = "checkpoint.ckpt"
    cb.local_rank = local_rank
    cb.world_rank = world_rank
    return cb


def _new_per_node_callback(
    local_rank=0,
    step_checkpoint_frequency=0,
    tmpdir_prefix="/tmp/ckpt",
):
    """Construct a ``RayTrainReportPerNodeCallback`` without base ``__init__``."""
    cb = object.__new__(RayTrainReportPerNodeCallback)
    cb.tmpdir_prefix = tmpdir_prefix
    cb.CHECKPOINT_NAME = "checkpoint.ckpt"
    cb.local_rank = local_rank
    cb.step_checkpoint_frequency = step_checkpoint_frequency
    cb.last_step_checkpoint = 0
    return cb


@pytest.fixture
def patched_io():
    """Patch ``os.makedirs`` / ``shutil.rmtree`` / ``Checkpoint`` / ``ray`` in the module."""
    with (
        patch(f"{_CALLBACKS_MODULE}.os.makedirs") as mk,
        patch(f"{_CALLBACKS_MODULE}.shutil.rmtree") as rm,
        patch(f"{_CALLBACKS_MODULE}.Checkpoint") as ckpt_cls,
        patch(f"{_CALLBACKS_MODULE}.ray") as ray_mod,
    ):
        yield {
            "makedirs": mk,
            "rmtree": rm,
            "Checkpoint": ckpt_cls,
            "ray": ray_mod,
        }


# -----------------------------------------------------------------------------
# RayTrainReportCallback.__init__
# -----------------------------------------------------------------------------


class TestRayTrainReportCallbackInit:
    """``RayTrainReportCallback.__init__`` captures the world rank."""

    def test_init_sets_world_rank_from_context(self):
        """``__init__`` records the world rank from the Ray Train context."""
        with (
            patch.object(
                RayTrainReportCallback.__bases__[0], "__init__", return_value=None
            ),
            patch(f"{_CALLBACKS_MODULE}.ray") as ray_mod,
        ):
            ray_mod.train.get_context.return_value.get_world_rank.return_value = 3
            cb = RayTrainReportCallback()
        assert cb.world_rank == 3


# -----------------------------------------------------------------------------
# RayTrainReportCallback.on_train_epoch_end
# -----------------------------------------------------------------------------


class TestRayTrainReportCallbackOnTrainEpochEnd:
    """Epoch-end reporting for the rank-0-only callback."""

    def test_rank0_reports_with_checkpoint(self, patched_io):
        """On world rank 0 the checkpoint is reported alongside the metrics."""
        cb = _new_default_callback(local_rank=0, world_rank=0)
        trainer = _make_trainer(
            current_epoch=2,
            global_step=20,
            metrics={"loss": _make_metric(0.5)},
        )
        ckpt_obj = patched_io["Checkpoint"].from_directory.return_value

        cb.on_train_epoch_end(trainer, MagicMock())

        patched_io["ray"].train.report.assert_called_once()
        _, kwargs = patched_io["ray"].train.report.call_args
        assert kwargs["checkpoint"] is ckpt_obj
        assert kwargs["metrics"]["loss"] == 0.5
        assert kwargs["metrics"]["epoch"] == 2
        assert kwargs["metrics"]["step"] == 20

    def test_non_rank0_reports_without_checkpoint(self, patched_io):
        """On non-zero world rank the checkpoint is ``None``."""
        cb = _new_default_callback(local_rank=1, world_rank=1)
        trainer = _make_trainer(current_epoch=0, global_step=0)

        cb.on_train_epoch_end(trainer, MagicMock())

        _, kwargs = patched_io["ray"].train.report.call_args
        assert kwargs["checkpoint"] is None

    def test_saves_checkpoint_and_barriers(self, patched_io):
        """The trainer saves the checkpoint and a barrier is issued."""
        cb = _new_default_callback(local_rank=0, world_rank=0)
        trainer = _make_trainer()

        cb.on_train_epoch_end(trainer, MagicMock())

        trainer.save_checkpoint.assert_called_once()
        _, save_kwargs = trainer.save_checkpoint.call_args
        assert save_kwargs["weights_only"] is False
        trainer.strategy.barrier.assert_called_once()

    def test_local_rank0_cleans_up_tmpdir(self, patched_io):
        """Local rank 0 removes the temporary checkpoint directory."""
        cb = _new_default_callback(local_rank=0, world_rank=0)
        cb.on_train_epoch_end(_make_trainer(), MagicMock())
        patched_io["rmtree"].assert_called_once()

    def test_non_local_rank0_does_not_clean_up(self, patched_io):
        """Non-local-rank-0 workers do not remove the directory."""
        cb = _new_default_callback(local_rank=2, world_rank=2)
        cb.on_train_epoch_end(_make_trainer(), MagicMock())
        patched_io["rmtree"].assert_not_called()


# -----------------------------------------------------------------------------
# RayTrainReportPerNodeCallback.__init__
# -----------------------------------------------------------------------------


class TestRayTrainReportPerNodeCallbackInit:
    """``RayTrainReportPerNodeCallback.__init__`` step-frequency wiring."""

    def test_init_defaults(self):
        """Default frequency disables step checkpointing and zeroes the cursor."""
        with (
            patch.object(RayTrainReportCallback, "__init__", return_value=None),
        ):
            cb = RayTrainReportPerNodeCallback()
        assert cb.step_checkpoint_frequency == 0
        assert cb.last_step_checkpoint == 0

    def test_init_custom_frequency(self):
        """A custom step frequency is stored."""
        with patch.object(RayTrainReportCallback, "__init__", return_value=None):
            cb = RayTrainReportPerNodeCallback(step_checkpoint_frequency=10)
        assert cb.step_checkpoint_frequency == 10


# -----------------------------------------------------------------------------
# RayTrainReportPerNodeCallback.on_train_batch_end
# -----------------------------------------------------------------------------


class TestPerNodeOnTrainBatchEnd:
    """Step-wise checkpointing driven by ``on_train_batch_end``."""

    def test_disabled_when_frequency_zero(self, patched_io):
        """A frequency of 0 never creates a step checkpoint."""
        cb = _new_per_node_callback(step_checkpoint_frequency=0)
        with patch.object(cb, "_create_and_report_checkpoint") as crc:
            cb.on_train_batch_end(_make_trainer(global_step=100))
        crc.assert_not_called()

    def test_checkpoint_created_when_threshold_reached(self, patched_io):
        """A step checkpoint fires once the step delta meets the frequency."""
        cb = _new_per_node_callback(step_checkpoint_frequency=5)
        trainer = _make_trainer(global_step=5)
        with patch.object(cb, "_create_and_report_checkpoint") as crc:
            cb.on_train_batch_end(trainer)
        crc.assert_called_once()
        _, kwargs = crc.call_args
        assert kwargs["is_step_checkpoint"] is True
        assert cb.last_step_checkpoint == 5

    def test_no_checkpoint_below_threshold(self, patched_io):
        """No checkpoint is created before the frequency threshold is met."""
        cb = _new_per_node_callback(step_checkpoint_frequency=5)
        with patch.object(cb, "_create_and_report_checkpoint") as crc:
            cb.on_train_batch_end(_make_trainer(global_step=3))
        crc.assert_not_called()
        assert cb.last_step_checkpoint == 0

    def test_checkpoint_id_uses_global_step(self, patched_io):
        """The step checkpoint id is derived from the global step."""
        cb = _new_per_node_callback(step_checkpoint_frequency=2)
        with patch.object(cb, "_create_and_report_checkpoint") as crc:
            cb.on_train_batch_end(_make_trainer(global_step=8))
        args, _ = crc.call_args
        assert args[1] == "step_8"


# -----------------------------------------------------------------------------
# RayTrainReportPerNodeCallback.on_train_epoch_end
# -----------------------------------------------------------------------------


class TestPerNodeOnTrainEpochEnd:
    """Epoch-end reporting for the per-node callback."""

    def test_epoch_checkpoint_id_and_flag(self, patched_io):
        """The epoch path uses an ``epoch_<n>`` id and an epoch (non-step) flag."""
        cb = _new_per_node_callback()
        with patch.object(cb, "_create_and_report_checkpoint") as crc:
            cb.on_train_epoch_end(_make_trainer(current_epoch=4), MagicMock())
        args, kwargs = crc.call_args
        assert args[1] == "epoch_4"
        assert kwargs["is_step_checkpoint"] is False


# -----------------------------------------------------------------------------
# RayTrainReportPerNodeCallback._create_and_report_checkpoint
# -----------------------------------------------------------------------------


class TestPerNodeCreateAndReportCheckpoint:
    """The shared checkpoint creation / reporting helper."""

    def test_local_rank0_reports_with_checkpoint(self, patched_io):
        """Local rank 0 reports the checkpoint and the augmented metrics."""
        cb = _new_per_node_callback(local_rank=0)
        trainer = _make_trainer(
            current_epoch=1,
            global_step=10,
            metrics={"acc": _make_metric(0.9)},
        )
        ckpt_obj = patched_io["Checkpoint"].from_directory.return_value

        cb._create_and_report_checkpoint(trainer, "epoch_1", is_step_checkpoint=False)

        _, kwargs = patched_io["ray"].train.report.call_args
        assert kwargs["checkpoint"] is ckpt_obj
        assert kwargs["metrics"]["acc"] == 0.9
        assert kwargs["metrics"]["epoch"] == 1
        assert kwargs["metrics"]["step"] == 10
        assert kwargs["metrics"]["is_step_checkpoint"] is False

    def test_non_local_rank0_reports_without_checkpoint(self, patched_io):
        """Non-local-rank-0 reports ``None`` for the checkpoint."""
        cb = _new_per_node_callback(local_rank=1)
        cb._create_and_report_checkpoint(
            _make_trainer(), "step_4", is_step_checkpoint=True
        )
        _, kwargs = patched_io["ray"].train.report.call_args
        assert kwargs["checkpoint"] is None
        assert kwargs["metrics"]["is_step_checkpoint"] is True

    def test_saves_checkpoint_barriers_and_cleans_up(self, patched_io):
        """Checkpoint is saved, a barrier is issued, and local rank 0 cleans up."""
        cb = _new_per_node_callback(local_rank=0)
        trainer = _make_trainer()
        cb._create_and_report_checkpoint(trainer, "epoch_0", is_step_checkpoint=False)
        trainer.save_checkpoint.assert_called_once()
        trainer.strategy.barrier.assert_called_once()
        patched_io["rmtree"].assert_called_once()

    def test_non_local_rank0_does_not_clean_up(self, patched_io):
        """Non-local-rank-0 workers leave the directory in place."""
        cb = _new_per_node_callback(local_rank=3)
        cb._create_and_report_checkpoint(
            _make_trainer(), "epoch_0", is_step_checkpoint=False
        )
        patched_io["rmtree"].assert_not_called()
