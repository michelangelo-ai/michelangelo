"""Tests for michelangelo.workflow.schema.sampling."""

from __future__ import annotations

import dataclasses

import pytest

from michelangelo.uniflow.core.codec import DataclassCodec
from michelangelo.workflow.schema.sampling import SamplingConfig


def _roundtrip(value):
    codec = DataclassCodec()
    return codec.decode(codec.encode(value))


def test_field_names_match_wire_format():
    """Field names appear in serialized pipeline configs, so they are fixed."""
    assert [f.name for f in dataclasses.fields(SamplingConfig)] == [
        "fractions",
        "agg_cols",
    ]


@pytest.mark.parametrize(
    "value",
    [
        SamplingConfig(),
        SamplingConfig(fractions={"train": 0.1, "validation": 1.0}),
        SamplingConfig(fractions={"train": 0.5}, agg_cols=["user", "session"]),
        # Out-of-range fractions and an empty agg_cols are accepted; they turn
        # sampling off rather than failing.
        SamplingConfig(fractions={"train": 0, "test": 1.5}, agg_cols=[]),
    ],
)
def test_codec_roundtrip(value):
    """Each config, including defaults, round-trips through the codec."""
    assert _roundtrip(value) == value


def test_unknown_key_fails_loudly():
    """An option this schema does not have is a TypeError, not ignored."""
    with pytest.raises(TypeError):
        SamplingConfig(seed=1)
