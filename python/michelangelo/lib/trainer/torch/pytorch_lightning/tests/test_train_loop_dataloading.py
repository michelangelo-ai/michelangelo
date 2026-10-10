"""Tests for dataloader and checkpoint-upload wiring in ``_train_loop_per_worker``.

The Ray Train session, Lightning trainer and resolver helpers are mocked so the
loop can run without a cluster; the assertions focus on how ``prefetch_batches``
reaches ``iter_torch_batches`` and how ``upload_async`` reaches the report
callback resolver.
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
        The mocked ``ray`` module and the mocked ``_resolve_callbacks``.
    """
    with (
        patch(f"{_UTIL_MODULE}.ray") as mock_ray,
        patch(f"{_UTIL_MODULE}.pl"),
        patch(f"{_UTIL_MODULE}._maybe_track_experiment"),
        patch(f"{_UTIL_MODULE}._resolve_strategy"),
        patch(f"{_UTIL_MODULE}._resolve_plugins"),
        patch(f"{_UTIL_MODULE}._resolve_logger"),
        patch(
            f"{_UTIL_MODULE}._resolve_callbacks", return_value=([], False)
        ) as mock_callbacks,
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
    return mock_ray, mock_callbacks


class TestTrainLoopPrefetch:
    """``prefetch_batches`` is forwarded to both Ray Data iterators."""

    def test_default_prefetch_is_one(self):
        """Without the option, the Ray Data default of 1 is passed explicitly."""
        mock_ray, _ = _run_loop(_loop_config())
        shard = mock_ray.train.get_dataset_shard.return_value
        assert shard.iter_torch_batches.call_count == 2
        for call in shard.iter_torch_batches.call_args_list:
            assert call.kwargs["prefetch_batches"] == 1

    def test_custom_prefetch_reaches_train_and_val(self):
        """A configured value is used for the train and validation iterators."""
        mock_ray, _ = _run_loop(_loop_config(prefetch_batches=7))
        shard = mock_ray.train.get_dataset_shard.return_value
        for call in shard.iter_torch_batches.call_args_list:
            assert call.kwargs["prefetch_batches"] == 7

    def test_zero_prefetch_passed_through(self):
        """``0`` (prefetch disabled) is forwarded rather than treated as unset."""
        mock_ray, _ = _run_loop(_loop_config(prefetch_batches=0))
        shard = mock_ray.train.get_dataset_shard.return_value
        for call in shard.iter_torch_batches.call_args_list:
            assert call.kwargs["prefetch_batches"] == 0


class TestTrainLoopUploadAsync:
    """``upload_async`` is forwarded to the callback resolver."""

    def test_default_is_synchronous(self):
        """The resolver receives ``upload_async=False`` by default."""
        _, mock_callbacks = _run_loop(_loop_config())
        assert mock_callbacks.call_args.kwargs["upload_async"] is False

    def test_enabled_is_forwarded(self):
        """``upload_async=True`` in the loop config reaches the resolver."""
        _, mock_callbacks = _run_loop(_loop_config(upload_async=True))
        assert mock_callbacks.call_args.kwargs["upload_async"] is True
