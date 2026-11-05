"""Tests for Ray dataset row sampling."""

from __future__ import annotations

import pyarrow as pa
import pytest

from michelangelo.workflow.schema.sampling import SamplingConfig
from michelangelo.workflow.tasks.functions.data.ray.sampling import (
    apply_sampling,
    hash_sample_batch,
)

_ROWS = [{"session": i // 4, "user": i % 3, "x": i} for i in range(400)]


@pytest.fixture(scope="module")
def ray_session():
    """One shared local Ray session for the module."""
    import ray

    ray.init(num_cpus=2, include_dashboard=False, ignore_reinit_error=True)
    yield
    ray.shutdown()


def _dataset():
    import ray.data

    return ray.data.from_items(_ROWS)


@pytest.mark.parametrize(
    "config",
    [
        None,
        SamplingConfig(),
        SamplingConfig(fractions={"validation": 0.1}),
        SamplingConfig(fractions={"train": 1.0}),
        SamplingConfig(fractions={"train": 0}),
        SamplingConfig(fractions={"train": -0.1}),
        SamplingConfig(fractions={"train": 1.5}),
    ],
    ids=[
        "no-config",
        "no-fractions",
        "other-dataset",
        "fraction-one",
        "fraction-zero",
        "negative",
        "above-one",
    ],
)
def test_unsampled_datasets_are_returned_as_they_are(config):
    """Nothing is added to the plan when the dataset keeps every row."""
    dataset = object()
    assert apply_sampling(dataset, config, "train") is dataset


def test_random_sampling_is_seeded(ray_session):
    """The same rows are kept on every run."""
    config = SamplingConfig(fractions={"train": 0.25})

    first = sorted(
        r["x"] for r in apply_sampling(_dataset(), config, "train").take_all()
    )
    second = sorted(
        r["x"] for r in apply_sampling(_dataset(), config, "train").take_all()
    )

    assert first == second
    assert 40 < len(first) < 160


def test_group_sampling_keeps_whole_groups(ray_session):
    """Every session is either fully kept or fully dropped."""
    config = SamplingConfig(fractions={"train": 0.5}, agg_cols=["session"])

    kept = apply_sampling(_dataset(), config, "train").take_all()

    sessions = {r["session"] for r in kept}
    assert 0 < len(sessions) < 100
    assert len(kept) == 4 * len(sessions)


def test_group_sampling_is_independent_of_batching():
    """A group's fate depends only on its key, so batches agree with each other."""
    table = pa.Table.from_pylist(_ROWS)

    whole = hash_sample_batch(table, 0.5, ["session", "user"])
    halves = [
        hash_sample_batch(table.slice(0, 200), 0.5, ["session", "user"]),
        hash_sample_batch(table.slice(200), 0.5, ["session", "user"]),
    ]

    assert whole.column("x").to_pylist() == [
        x for half in halves for x in half.column("x").to_pylist()
    ]


def test_group_sampling_by_one_column():
    """A single grouping column is hashed directly, without joining."""
    table = pa.Table.from_pylist(_ROWS)

    kept = hash_sample_batch(table, 0.5, ["session"]).column("session").to_pylist()

    assert 0 < len(set(kept)) < 100
    assert all(kept.count(s) == 4 for s in set(kept))


def test_empty_agg_cols_samples_at_random(ray_session):
    """An empty grouping list is the same as none: seeded random sampling."""
    grouped = SamplingConfig(fractions={"train": 0.25}, agg_cols=[])
    plain = SamplingConfig(fractions={"train": 0.25})

    def kept(config):
        return sorted(
            r["x"] for r in apply_sampling(_dataset(), config, "train").take_all()
        )

    assert kept(grouped) == kept(plain)
