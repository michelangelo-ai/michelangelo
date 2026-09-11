"""Real (non-mocked) integration test for the Ray Tune helper.

Exercises :func:`michelangelo.lib.tuner.ray_tune.tune` end-to-end on a local
Ray cluster -- no mocking -- to guard against the class of bug where the unit
suite stays green while the helper is broken against the installed Ray
runtime (schedulers, ``TuneConfig``, and ``ResultGrid`` have all shifted
across Ray releases).

Like ``lib/trainer``'s ``test_auto_resume_integration.py``, this spins up a
real Ray runtime, which OOM-kills constrained CI runners -- so it is **skipped
in CI by default**. Run it locally (the default off-CI), or in a dedicated
Ray-enabled CI job, by setting ``MICHELANGELO_RUN_RAY_INTEGRATION_TESTS=1``.
"""

from __future__ import annotations

import os

import pytest

# Skip in CI unless explicitly forced (see module docstring): a real Ray
# runtime exceeds the memory of shared coverage runners.
if (os.environ.get("CI") or os.environ.get("GITHUB_ACTIONS")) and (
    os.environ.get("MICHELANGELO_RUN_RAY_INTEGRATION_TESTS") != "1"
):
    pytest.skip(
        "Skipping real-Ray integration tests in CI (they spin up a Ray "
        "runtime and OOM on constrained runners). Set "
        "MICHELANGELO_RUN_RAY_INTEGRATION_TESTS=1 to run them.",
        allow_module_level=True,
    )

pytest.importorskip("ray.tune")

import ray
import ray.train

from michelangelo.lib.tuner.ray_tune import TuneParam, tune

# ray.tune.RunConfig only exists on newer Ray; older supported versions take a
# ray.train.RunConfig for the same Tuner argument.
_RunConfig = getattr(ray.tune, "RunConfig", None) or ray.train.RunConfig


@pytest.fixture(scope="module")
def local_ray():
    """A small local Ray runtime shared by the tests in this module."""
    ray.init(num_cpus=2, include_dashboard=False, ignore_reinit_error=True)
    yield
    ray.shutdown()


def _objective(config: dict) -> None:
    """Deterministic quadratic bowl: best trial is the x closest to 3."""
    score = -((config["x"] - 3) ** 2)
    ray.tune.report({"score": score})


@pytest.mark.usefixtures("local_ray")
def test_tune_finds_best_config_on_local_ray(tmp_path):
    """A real 4-trial grid search completes and picks the best config."""
    result = tune(
        TuneParam(
            trainable=_objective,
            param_space={"x": ray.tune.grid_search([0, 1, 3, 8])},
            metric="score",
            mode="max",
            num_samples=1,  # grid_search expands to one trial per grid value
            scheduler="fifo",
            run_config=_RunConfig(storage_path=str(tmp_path), name="tune_helper_it"),
        )
    )

    assert result["best_config"] == {"x": 3}
    assert result["best_metrics"]["score"] == 0
    assert result["num_trials"] == 4
    assert result["num_errors"] == 0
    # The objective never reports a checkpoint, so the contract maps it to None.
    assert result["checkpoint_path"] is None


@pytest.mark.usefixtures("local_ray")
def test_tune_asha_random_search_completes(tmp_path):
    """The default ASHA path runs real sampled trials to completion."""
    result = tune(
        TuneParam(
            trainable=_objective,
            param_space={"x": ray.tune.uniform(-5, 5)},
            metric="score",
            mode="max",
            num_samples=4,
            run_config=_RunConfig(
                storage_path=str(tmp_path), name="tune_helper_asha_it"
            ),
        )
    )

    assert result["num_trials"] == 4
    assert -5 <= result["best_config"]["x"] <= 5
    assert result["best_metrics"]["score"] <= 0
