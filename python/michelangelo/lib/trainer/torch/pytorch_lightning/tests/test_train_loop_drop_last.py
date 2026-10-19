"""Tests for ``drop_last`` wiring in ``_train_loop_per_worker``.

The Ray Train session, Lightning trainer and resolver helpers are mocked so the
loop can run without a cluster; the assertions focus on how ``drop_last``
reaches the training ``iter_torch_batches`` call (and not the validation one).
"""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

pytest.importorskip("ray")
pytest.importorskip("torch")
pytest.importorskip("pytorch_lightning")

from michelangelo.lib.trainer.torch.pytorch_lightning._private import util
from michelangelo.lib.trainer.torch.pytorch_lightning.schema import (
    CustomDataloaderParam,
)

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
        The mocked ``ray`` module.
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
    return mock_ray


def _train_and_val_calls(train_loop_config):
    """Return the kwargs of the train and validation ``iter_torch_batches`` calls."""
    shard = _run_loop(train_loop_config).train.get_dataset_shard.return_value
    assert shard.iter_torch_batches.call_count == 2
    train_call, val_call = shard.iter_torch_batches.call_args_list
    return train_call.kwargs, val_call.kwargs


class TestTrainLoopDropLast:
    """``drop_last`` is forwarded to the training iterator only."""

    def test_default_keeps_the_partial_batch(self):
        """Without the option, the training iterator gets ``drop_last=False``."""
        train_kwargs, _ = _train_and_val_calls(_loop_config())
        assert train_kwargs["drop_last"] is False

    def test_enabled_reaches_the_training_iterator(self):
        """``drop_last=True`` is used for the training iterator."""
        train_kwargs, _ = _train_and_val_calls(_loop_config(drop_last=True))
        assert train_kwargs["drop_last"] is True

    @pytest.mark.parametrize("drop_last", [False, True])
    def test_validation_iterator_is_never_dropped(self, drop_last):
        """The validation iterator keeps its partial batch either way."""
        _, val_kwargs = _train_and_val_calls(_loop_config(drop_last=drop_last))
        assert "drop_last" not in val_kwargs


class TestTrainLoopDropLastWithCustomDataloader:
    """A custom dataloader owns its batching and never receives ``drop_last``."""

    def test_custom_dataloader_gets_no_drop_last_and_no_ray_iterators(self):
        """``drop_last=True`` reaches neither the factory nor Ray Data."""
        calls: list = []

        def factory(stage, *, dataset_path, read_kwargs, **kwargs):
            calls.append({"stage": stage, "kwargs": kwargs})
            return f"loader-{stage}"

        custom = CustomDataloaderParam(
            factory=factory,
            train_dataset_path="memory://train",
            validation_dataset_path="memory://val",
            file_backed=True,
        )
        mock_ray = _run_loop(_loop_config(drop_last=True, custom_dataloader=custom))

        assert [c["stage"] for c in calls] == ["train", "validation"]
        assert all("drop_last" not in c["kwargs"] for c in calls)
        shard = mock_ray.train.get_dataset_shard.return_value
        shard.iter_torch_batches.assert_not_called()
