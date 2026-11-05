"""Row sampling for Ray datasets."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

import pyarrow as pa
import pyarrow.compute as pc

if TYPE_CHECKING:
    import ray.data

    from michelangelo.workflow.schema.sampling import SamplingConfig

_logger = logging.getLogger(__name__)

__all__ = ["apply_sampling", "hash_sample_batch"]

#: Seed for random sampling, fixed so a rerun keeps the same rows.
_SEED = 1


def hash_sample_batch(
    batch: pa.Table, fraction: float, agg_cols: list[str]
) -> pa.Table:
    """Keep the rows whose group hashes below ``fraction`` of the hash range.

    Rows with the same values in ``agg_cols`` hash the same, so a group is kept
    or dropped as a whole, in every batch and on every run.

    Args:
        batch: Rows to sample.
        fraction: Fraction of groups to keep.
        agg_cols: Columns whose values identify a group.

    Returns:
        The kept rows.
    """
    import pandas as pd

    if len(agg_cols) == 1:
        keys = pc.cast(batch.column(agg_cols[0]), pa.string())
    else:
        keys = pc.binary_join_element_wise(
            *[pc.cast(batch.column(c), pa.string()) for c in agg_cols], "|"
        )
    # PyArrow has no stable hash kernel. pandas' hash is stable across runs and
    # processes and returns uint64 values spread over [0, 2**64).
    hashes = pd.util.hash_pandas_object(keys.to_pandas(), index=False)
    return batch.filter(pa.array(hashes < int(fraction * 2**64)))


def apply_sampling(
    dataset: ray.data.Dataset, config: SamplingConfig | None, dataset_name: str
) -> ray.data.Dataset:
    """Sample a dataset by the fraction configured for its name.

    Args:
        dataset: The dataset to sample. Sampling is lazy, like any other Ray
            Data operation.
        config: The sampling settings. ``None`` keeps every row.
        dataset_name: The dataset's name, looked up in ``config.fractions``.

    Returns:
        The sampled dataset, or ``dataset`` itself when it is not sampled.
    """
    fraction = (config.fractions or {}).get(dataset_name, 1.0) if config else 1.0
    if fraction >= 1:
        return dataset

    if config.agg_cols:
        _logger.info(
            "Sampling %s of the %s groups by %s",
            fraction,
            dataset_name,
            config.agg_cols,
        )
        return dataset.map_batches(
            hash_sample_batch,
            fn_kwargs={"fraction": fraction, "agg_cols": config.agg_cols},
            batch_format="pyarrow",
            zero_copy_batch=True,
        )
    _logger.info("Sampling %s of the %s rows at random", fraction, dataset_name)
    return dataset.random_sample(fraction, seed=_SEED)
