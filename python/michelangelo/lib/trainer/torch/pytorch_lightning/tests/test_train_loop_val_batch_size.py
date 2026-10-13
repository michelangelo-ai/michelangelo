"""Tests for ``val_batch_size`` wiring in ``_train_loop_per_worker``.

The Ray Train session, Lightning trainer and resolver helpers are mocked so the
loop can run without a cluster; the assertions focus on which batch size each
``iter_torch_batches`` call receives.
"""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

pytest.importorskip("ray")
pytest.importorskip("torch")
pytest.importorskip("pytorch_lightning")

from michelangelo.lib.trainer.torch.pytorch_lightning._private import util

_UTIL_MODULE = "michelangelo.lib.trainer.torch.pytorch_lightning._private.util"


def _loop_config(**overrides):
    """Build a minimal ``train_loop_config`` for the worker loop."""
    config = {
        "batch_size": 4,
        "num_epochs": 1,
        "num_shuffle_batches": 2,
        "create_model_fn": MagicMock(),
        "create_model_fn_kwargs": {},
        "data_collate_fn": None,
    }
    config.update(overrides)
    return config


def _run_loop(train_loop_config):
    """Run ``_train_loop_per_worker`` with all heavy collaborators mocked.

    Returns:
        The mocked dataset shard whose ``iter_torch_batches`` calls are recorded.
    """
    with (
        patch(f"{_UTIL_MODULE}.ray") as mock_ray,
        patch(f"{_UTIL_MODULE}.pl"),
        patch(f"{_UTIL_MODULE}._maybe_track_experiment"),
        patch(f"{_UTIL_MODULE}._resolve_strategy"),
        patch(f"{_UTIL_MODULE}._resolve_plugins"),
        patch(f"{_UTIL_MODULE}._resolve_logger"),
        patch(f"{_UTIL_MODULE}._resolve_callbacks", return_value=([], False)),
        patch(
            f"{_UTIL_MODULE}._resolve_profiler",
            return_value=(None, None, False),
        ),
        patch(f"{_UTIL_MODULE}._prepare_trainer_for_ray"),
        patch(f"{_UTIL_MODULE}._maybe_export_profiler_results"),
    ):
        mock_ray.train.get_context.return_value.get_world_rank.return_value = 0
        mock_ray.train.get_context.return_value.get_world_size.return_value = 1
        mock_ray.train.get_checkpoint.return_value = None
        util._train_loop_per_worker(train_loop_config)
    return mock_ray.train.get_dataset_shard.return_value


class TestTrainLoopValBatchSize:
    """``val_batch_size`` reaches only the validation iterator."""

    def test_unset_reuses_batch_size(self):
        """Without the option both iterators use ``batch_size``."""
        shard = _run_loop(_loop_config())
        sizes = [
            c.kwargs["batch_size"] for c in shard.iter_torch_batches.call_args_list
        ]
        assert sizes == [4, 4]

    def test_none_reuses_batch_size(self):
        """An explicit ``None`` (as produced by ``asdict``) reuses ``batch_size``."""
        shard = _run_loop(_loop_config(val_batch_size=None))
        sizes = [
            c.kwargs["batch_size"] for c in shard.iter_torch_batches.call_args_list
        ]
        assert sizes == [4, 4]

    def test_custom_value_applies_to_validation_only(self):
        """Training keeps ``batch_size`` and its shuffle buffer; validation differs."""
        shard = _run_loop(_loop_config(val_batch_size=16))
        train_call, val_call = shard.iter_torch_batches.call_args_list
        assert train_call.kwargs["batch_size"] == 4
        assert train_call.kwargs["local_shuffle_buffer_size"] == 2 * 4
        assert val_call.kwargs["batch_size"] == 16
        assert "local_shuffle_buffer_size" not in val_call.kwargs
