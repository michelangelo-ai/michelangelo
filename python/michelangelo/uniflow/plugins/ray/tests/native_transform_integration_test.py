"""Real (non-mocked) Ray integration tests for the native_transform adapter.

These exercise ``transform()`` end-to-end against a real local Ray cluster
(``ray.data``, ``Dataset.map_batches``) to guard against the class of bug
where a fully-mocked unit suite stays green while the actual Ray/torch
integration is broken (mismatched ``map_batches`` kwargs, batch-format
assumptions, etc).

They spin up a real (local, single-process) Ray runtime, which is slow and
memory-heavier than the mocked unit suite. On memory-constrained CI runners
this can be tight, so — matching the precedent set by
``lib/trainer/torch/pytorch_lightning/tests/test_auto_resume_integration.py``
— these are **skipped in CI by default**. Run them locally (the default
off-CI), or in a dedicated Ray-enabled CI job, by setting
``MICHELANGELO_RUN_RAY_INTEGRATION_TESTS=1``.
"""

from __future__ import annotations

import os

import pytest

if (os.environ.get("CI") or os.environ.get("GITHUB_ACTIONS")) and (
    os.environ.get("MICHELANGELO_RUN_RAY_INTEGRATION_TESTS") != "1"
):
    pytest.skip(
        "Skipping real-Ray integration tests in CI (they spin up a Ray cluster "
        "and are memory-heavy on constrained runners). Set "
        "MICHELANGELO_RUN_RAY_INTEGRATION_TESTS=1 to run them.",
        allow_module_level=True,
    )

ray = pytest.importorskip("ray")
pytest.importorskip("ray.data")
torch = pytest.importorskip("torch")

from michelangelo.lib.native_transform.torch import TransformSpec  # noqa: E402
from michelangelo.uniflow.plugins.ray.native_transform import transform  # noqa: E402


@pytest.fixture(scope="module", autouse=True)
def _ray_cluster():
    ray.init(num_cpus=2, include_dashboard=False, ignore_reinit_error=True)
    yield
    ray.shutdown()


def _make_concatenate_spec() -> TransformSpec:
    """Build a single-layer spec: Concatenate(x) -> x_out (identity for one input)."""
    raw_spec = {
        "transform_specs": [
            {
                "transform_name": "Concatenate",
                "input_cols": ["x"],
                "output_cols": ["x_out"],
            }
        ],
    }
    return TransformSpec(raw_transform_specs=raw_spec)


class TestTransformOnRealRayCluster:
    """End-to-end ``transform()`` against a real local Ray cluster."""

    def test_transform_produces_expected_output_column(self):
        """A single-level Concatenate spec runs and produces the output column."""
        ds = ray.data.from_items([{"x": 1.0}, {"x": 2.0}, {"x": 3.0}])
        spec = _make_concatenate_spec()

        result_ds, result_spec, result_stats = transform(ds, spec, feature_stats={})

        rows = sorted(result_ds.take_all(), key=lambda r: r["x"])
        assert [r["x_out"] for r in rows] == pytest.approx([1.0, 2.0, 3.0])
        assert result_spec is spec
        assert isinstance(result_stats, dict)

    def test_transform_passes_through_non_numeric_columns(self):
        """A string column not referenced by the spec passes through unchanged."""
        ds = ray.data.from_items(
            [{"x": 1.0, "label": "a"}, {"x": 2.0, "label": "b"}],
        )
        spec = _make_concatenate_spec()

        result_ds, _, _ = transform(ds, spec, feature_stats={})

        rows = sorted(result_ds.take_all(), key=lambda r: r["x"])
        assert [r["label"] for r in rows] == ["a", "b"]
