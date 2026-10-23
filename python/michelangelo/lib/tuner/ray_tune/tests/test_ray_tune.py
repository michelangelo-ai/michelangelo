"""Tests for the Ray Tune helper module.

Cover the public surface of ``michelangelo.lib.tuner.ray_tune.tuner``:
dataclass validation, scheduler resolution, Tuner wiring, and the ``tune()``
result contract. ``ray.tune.Tuner`` is patched throughout, so nothing here
needs a Ray cluster; the real-cluster path lives in
``test_ray_tune_integration.py``.
"""

from __future__ import annotations

from types import SimpleNamespace
from unittest.mock import MagicMock, patch

import pytest

pytest.importorskip("ray.tune")

from ray.tune.schedulers import ASHAScheduler, FIFOScheduler

from michelangelo.lib.tuner.ray_tune.tuner import (
    BEST_CONFIG_KEY,
    BEST_METRICS_KEY,
    CHECKPOINT_PATH_KEY,
    TuneParam,
    _resolve_scheduler,
    tune,
)

_TUNER = "michelangelo.lib.tuner.ray_tune.tuner.ray.tune.Tuner"
_WITH_RESOURCES = "michelangelo.lib.tuner.ray_tune.tuner.ray.tune.with_resources"


def _make_param(**overrides) -> TuneParam:
    """Build a minimally-valid ``TuneParam`` for tests."""
    defaults = {
        "trainable": lambda config: None,
        "param_space": {"lr": [0.1, 0.01]},
        "metric": "val_loss",
    }
    defaults.update(overrides)
    return TuneParam(**defaults)


def _make_result_grid(
    *, checkpoint_path: str | None = "/ckpt/best", errors: list | None = None
) -> MagicMock:
    """Build a mocked ``ResultGrid`` with one best result."""
    checkpoint = SimpleNamespace(path=checkpoint_path) if checkpoint_path else None
    best = SimpleNamespace(
        config={"lr": 0.1},
        metrics={"val_loss": 0.25},
        checkpoint=checkpoint,
        path="/results/trial_0",
    )
    grid = MagicMock(name="result_grid")
    grid.get_best_result.return_value = best
    grid.errors = errors or []
    grid.__len__.return_value = 4
    return grid


# -----------------------------------------------------------------------------
# TuneParam
# -----------------------------------------------------------------------------


class TestTuneParam:
    """``TuneParam`` dataclass behavior."""

    def test_defaults(self):
        """It applies the documented default for every optional field."""
        param = _make_param()
        assert param.mode == "min"
        assert param.num_samples == 10
        assert param.scheduler == "asha"
        assert param.resources_per_trial is None
        assert param.max_concurrent_trials is None
        assert param.run_config is None
        assert param.tune_config_kwargs == {}

    def test_mutable_defaults_are_not_shared(self):
        """Each instance gets its own ``tune_config_kwargs`` dict."""
        first = _make_param()
        second = _make_param()
        first.tune_config_kwargs["reuse_actors"] = True
        assert second.tune_config_kwargs == {}

    def test_rejects_non_callable_trainable(self):
        """A non-callable trainable is rejected."""
        with pytest.raises(ValueError, match="trainable must be callable"):
            _make_param(trainable="not-a-function")

    def test_rejects_empty_param_space(self):
        """An empty search space is rejected."""
        with pytest.raises(ValueError, match="non-empty search space"):
            _make_param(param_space={})

    def test_rejects_empty_metric(self):
        """An empty metric name is rejected."""
        with pytest.raises(ValueError, match="non-empty metric name"):
            _make_param(metric="")

    def test_rejects_bad_mode(self):
        """A mode other than min/max is rejected."""
        with pytest.raises(ValueError, match="mode must be 'min' or 'max'"):
            _make_param(mode="maximize")

    def test_rejects_non_positive_num_samples(self):
        """A zero or negative trial budget is rejected."""
        with pytest.raises(ValueError, match="num_samples must be positive"):
            _make_param(num_samples=0)

    def test_rejects_unknown_scheduler_name(self):
        """An unknown scheduler name is rejected."""
        with pytest.raises(ValueError, match="scheduler must be one of"):
            _make_param(scheduler="hyperband-but-misspelled")

    def test_rejects_non_scheduler_instance(self):
        """A non-TrialScheduler object is rejected."""
        with pytest.raises(ValueError, match="TrialScheduler"):
            _make_param(scheduler=object())

    def test_accepts_scheduler_instance(self):
        """Any ``TrialScheduler`` instance passes validation unchanged."""
        scheduler = FIFOScheduler()
        assert _make_param(scheduler=scheduler).scheduler is scheduler

    def test_rejects_reserved_tune_config_keys(self):
        """Keys owned by TuneParam fields are rejected in tune_config_kwargs."""
        with pytest.raises(ValueError, match="must not set"):
            _make_param(tune_config_kwargs={"metric": "other"})


# -----------------------------------------------------------------------------
# _resolve_scheduler
# -----------------------------------------------------------------------------


class TestResolveScheduler:
    """Scheduler name / instance resolution."""

    def test_asha_name(self):
        """The name 'asha' resolves to an ASHAScheduler."""
        assert isinstance(_resolve_scheduler("asha"), ASHAScheduler)

    def test_fifo_name(self):
        """The name 'fifo' resolves to a FIFOScheduler."""
        assert isinstance(_resolve_scheduler("fifo"), FIFOScheduler)

    def test_instance_passthrough(self):
        """A TrialScheduler instance is returned unchanged."""
        scheduler = ASHAScheduler()
        assert _resolve_scheduler(scheduler) is scheduler


# -----------------------------------------------------------------------------
# tune()
# -----------------------------------------------------------------------------


class TestTune:
    """``tune()`` wiring and result contract, with ``Tuner`` patched."""

    def test_passes_search_through_to_tuner(self):
        """Trainable, space, and TuneConfig fields reach the Tuner verbatim."""
        param = _make_param(
            mode="max",
            num_samples=7,
            max_concurrent_trials=3,
            tune_config_kwargs={"reuse_actors": True},
        )
        with patch(_TUNER) as tuner_cls:
            tuner_cls.return_value.fit.return_value = _make_result_grid()
            tune(param)

        (trainable,), kwargs = tuner_cls.call_args
        assert trainable is param.trainable
        assert kwargs["param_space"] == {"lr": [0.1, 0.01]}
        assert kwargs["run_config"] is None
        tune_config = kwargs["tune_config"]
        assert tune_config.metric == "val_loss"
        assert tune_config.mode == "max"
        assert tune_config.num_samples == 7
        assert tune_config.max_concurrent_trials == 3
        assert tune_config.reuse_actors is True
        assert isinstance(tune_config.scheduler, ASHAScheduler)

    def test_run_config_passthrough(self):
        """A caller-supplied run_config is handed to the Tuner unchanged."""
        run_config = MagicMock(name="run_config")
        with patch(_TUNER) as tuner_cls:
            tuner_cls.return_value.fit.return_value = _make_result_grid()
            tune(_make_param(run_config=run_config))
        assert tuner_cls.call_args.kwargs["run_config"] is run_config

    def test_resources_per_trial_wraps_trainable(self):
        """``resources_per_trial`` applies ``with_resources`` to the trainable."""
        param = _make_param(resources_per_trial={"cpu": 2, "gpu": 1})
        wrapped = MagicMock(name="wrapped_trainable")
        with (
            patch(_WITH_RESOURCES, return_value=wrapped) as with_resources,
            patch(_TUNER) as tuner_cls,
        ):
            tuner_cls.return_value.fit.return_value = _make_result_grid()
            tune(param)
        with_resources.assert_called_once_with(param.trainable, {"cpu": 2, "gpu": 1})
        assert tuner_cls.call_args.args[0] is wrapped

    def test_no_resources_leaves_trainable_unwrapped(self):
        """Without resources_per_trial the trainable is passed as-is."""
        with (
            patch(_WITH_RESOURCES) as with_resources,
            patch(_TUNER) as tuner_cls,
        ):
            tuner_cls.return_value.fit.return_value = _make_result_grid()
            tune(_make_param())
        with_resources.assert_not_called()

    def test_result_contract(self):
        """The returned dict carries the documented keys."""
        with patch(_TUNER) as tuner_cls:
            tuner_cls.return_value.fit.return_value = _make_result_grid(
                errors=[RuntimeError("trial 3 died")]
            )
            result = tune(_make_param())

        assert result[BEST_CONFIG_KEY] == {"lr": 0.1}
        assert result[BEST_METRICS_KEY] == {"val_loss": 0.25}
        assert result[CHECKPOINT_PATH_KEY] == "/ckpt/best"
        assert result["path"] == "/results/trial_0"
        assert result["num_trials"] == 4
        assert result["num_errors"] == 1

    def test_checkpointless_best_result_maps_to_none(self):
        """A trainable that never reports a checkpoint yields ``None``."""
        with patch(_TUNER) as tuner_cls:
            tuner_cls.return_value.fit.return_value = _make_result_grid(
                checkpoint_path=None
            )
            result = tune(_make_param())
        assert result[CHECKPOINT_PATH_KEY] is None

    def test_no_successful_trial_propagates(self):
        """Ray's no-completed-trials error is not swallowed."""
        grid = _make_result_grid()
        grid.get_best_result.side_effect = RuntimeError("No best trial found")
        with patch(_TUNER) as tuner_cls:
            tuner_cls.return_value.fit.return_value = grid
            with pytest.raises(RuntimeError, match="No best trial"):
                tune(_make_param())
