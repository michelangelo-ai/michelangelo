"""Configuration for sampling the rows of input datasets."""

from __future__ import annotations

from dataclasses import dataclass

__all__ = ["SamplingConfig"]


@dataclass
class SamplingConfig:
    """Fraction of each dataset's rows to keep.

    Attributes:
        fractions: Dataset name to the fraction of its rows to keep. Only a
            fraction strictly between ``0`` and ``1`` samples; any other value,
            or a dataset that is not listed, keeps every row.
        agg_cols: Columns whose values identify a group, such as a session.
            When set, a group is kept or dropped as a whole, chosen by a hash
            of its values, so the same groups are kept on every run. When not
            set or empty, rows are sampled at random with a fixed seed.

    Example:
        >>> SamplingConfig(fractions={"train": 0.1}, agg_cols=["session_id"])
        SamplingConfig(fractions={'train': 0.1}, agg_cols=['session_id'])
    """

    fractions: dict[str, float] | None = None
    agg_cols: list[str] | None = None
