"""Helpers for working with Ray datasets."""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    import ray


def merge_ray_datasets(datasets: list[ray.data.Dataset]) -> ray.data.Dataset | None:
    """Merge multiple Ray datasets into one.

    Args:
        datasets: The datasets to merge.

    Returns:
        The union of the datasets, the only dataset when given one, or ``None``
        when given none.
    """
    if not datasets:
        return None
    if len(datasets) == 1:
        return datasets[0]
    ds_1 = datasets[0]
    return ds_1.union(*datasets[1:])
