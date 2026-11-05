"""Configuration for sampling the rows of input datasets."""

from __future__ import annotations

from dataclasses import dataclass

from michelangelo.workflow.schema.exceptions import ConfigurationError

__all__ = ["SamplingConfig"]


@dataclass
class SamplingConfig:
    """Fraction of each dataset's rows to keep.

    Attributes:
        fractions: Dataset name to the fraction of its rows to keep, greater
            than ``0`` and at most ``1``. A dataset that is not listed, or has
            fraction ``1``, is not sampled.
        agg_cols: Columns whose values identify a group, such as a session.
            When set, a group is kept or dropped as a whole, chosen by a hash
            of its values, so the same groups are kept on every run. When not
            set, rows are sampled at random with a fixed seed.

    Raises:
        ConfigurationError: If a fraction is not in ``(0, 1]``, or
            ``agg_cols`` is an empty list.

    Example:
        >>> SamplingConfig(fractions={"train": 0.1}, agg_cols=["session_id"])
        SamplingConfig(fractions={'train': 0.1}, agg_cols=['session_id'])
    """

    fractions: dict[str, float] | None = None
    agg_cols: list[str] | None = None

    def __post_init__(self) -> None:
        """Validate the fractions and grouping columns."""
        for name, fraction in (self.fractions or {}).items():
            if isinstance(fraction, bool) or not 0 < fraction <= 1:
                raise ConfigurationError(
                    f"sampling.fractions[{name!r}] must be > 0 and <= 1, "
                    f"got {fraction!r}."
                )
        if self.agg_cols is not None and not self.agg_cols:
            raise ConfigurationError(
                "sampling.agg_cols must name at least one column, or be omitted."
            )
