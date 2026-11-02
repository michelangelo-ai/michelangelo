"""Tests for ``random_seed`` wiring in ``_train_loop_per_worker``.

The Ray Train session, Lightning trainer and resolver helpers are mocked so the
loop can run without a cluster. Seeding is asserted both through a mocked
``seed_everything`` call and through the real function, by recording the RNG
output observed inside ``create_model_fn``.
"""

from __future__ import annotations

import os
import random
from unittest.mock import MagicMock, patch

import numpy as np
import pytest

pytest.importorskip("ray")
pytest.importorskip("torch")
pytest.importorskip("pytorch_lightning")

import torch

from michelangelo.lib.trainer.torch.pytorch_lightning._private import util

_UTIL_MODULE = "michelangelo.lib.trainer.torch.pytorch_lightning._private.util"


@pytest.fixture(autouse=True)
def _restore_global_rng_state():
    """Undo the global RNG and env mutations made by the real ``seed_everything``."""
    torch_state = torch.get_rng_state()
    numpy_state = np.random.get_state()
    python_state = random.getstate()
    env_keys = ("PL_GLOBAL_SEED", "PL_SEED_WORKERS")
    saved_env = {k: os.environ.get(k) for k in env_keys}
    yield
    torch.set_rng_state(torch_state)
    np.random.set_state(numpy_state)
    random.setstate(python_state)
    for key, value in saved_env.items():
        if value is None:
            os.environ.pop(key, None)
        else:
            os.environ[key] = value


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


def _run_loop(train_loop_config, *, mock_pl=True):
    """Run ``_train_loop_per_worker`` with heavy collaborators mocked.

    Args:
        train_loop_config: Config passed to the worker loop.
        mock_pl: When ``True`` the whole ``pl`` module is mocked so calls to
            ``seed_everything`` can be asserted. When ``False`` only
            ``pl.Trainer`` is mocked and the real ``seed_everything`` runs.

    Returns:
        The mocked ``pl`` module (``None`` when ``mock_pl`` is ``False``).
    """
    with (
        patch(f"{_UTIL_MODULE}.ray") as mock_ray,
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
        if mock_pl:
            with patch(f"{_UTIL_MODULE}.pl") as pl_mock:
                util._train_loop_per_worker(train_loop_config)
            return pl_mock
        with patch.object(util.pl, "Trainer"):
            util._train_loop_per_worker(train_loop_config)
        return None


def _sample_rngs_in_create_model_fn(**config_overrides):
    """Run the loop for real seeding and return RNG draws seen at model creation."""
    draws = {}

    def create_model_fn():
        draws["torch"] = torch.rand(4).tolist()
        draws["python"] = random.random()
        return MagicMock()

    _run_loop(
        _loop_config(create_model_fn=create_model_fn, **config_overrides),
        mock_pl=False,
    )
    return draws


class TestTrainLoopRandomSeed:
    """``random_seed`` triggers ``seed_everything`` once per worker loop."""

    def test_seed_everything_called_with_workers_true(self):
        """A configured seed is passed to Lightning with ``workers=True``."""
        pl_mock = _run_loop(_loop_config(random_seed=1234))
        pl_mock.seed_everything.assert_called_once_with(1234, workers=True)

    def test_zero_is_a_valid_seed(self):
        """``0`` is a real seed, not treated as unset."""
        pl_mock = _run_loop(_loop_config(random_seed=0))
        pl_mock.seed_everything.assert_called_once_with(0, workers=True)

    def test_not_called_when_unset(self):
        """Without the key the loop never seeds."""
        pl_mock = _run_loop(_loop_config())
        pl_mock.seed_everything.assert_not_called()

    def test_not_called_when_none(self):
        """An explicit ``None`` leaves behaviour unchanged."""
        pl_mock = _run_loop(_loop_config(random_seed=None))
        pl_mock.seed_everything.assert_not_called()

    def test_seeds_before_the_model_is_created(self):
        """Seeding precedes ``create_model_fn`` so weight init is seeded."""
        order = []
        create_model_fn = MagicMock(
            side_effect=lambda **_: order.append("create_model") or MagicMock()
        )
        with patch(f"{_UTIL_MODULE}.pl") as pl_mock:
            pl_mock.seed_everything.side_effect = lambda *a, **k: order.append("seed")
            with (
                patch(f"{_UTIL_MODULE}.ray") as mock_ray,
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
                util._train_loop_per_worker(
                    _loop_config(random_seed=5, create_model_fn=create_model_fn)
                )
        assert order == ["seed", "create_model"]


class TestTrainLoopSeedDeterminism:
    """With the real ``seed_everything``, equal seeds give equal RNG streams."""

    def test_same_seed_gives_identical_draws(self):
        """Two seeded runs observe identical torch and Python RNG output."""
        first = _sample_rngs_in_create_model_fn(random_seed=42)
        second = _sample_rngs_in_create_model_fn(random_seed=42)
        assert first == second

    def test_different_seeds_give_different_draws(self):
        """Distinct seeds produce distinct streams."""
        first = _sample_rngs_in_create_model_fn(random_seed=42)
        second = _sample_rngs_in_create_model_fn(random_seed=43)
        assert first != second

    def test_unseeded_runs_are_not_reseeded(self):
        """Without a seed the loop does not reset the RNG between runs."""
        first = _sample_rngs_in_create_model_fn()
        second = _sample_rngs_in_create_model_fn()
        assert first != second
