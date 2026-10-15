"""Helpers that persist Ray-backed datasets for workflow tasks.

Shared by every task that writes ``DatasetVariable`` outputs (native transform,
batch inference), so the Ray-version gating of ``write_parquet`` options lives
in one place.
"""

from __future__ import annotations

import logging
import time
from typing import TYPE_CHECKING

import ray
from packaging import version

from michelangelo.workflow.schema.exceptions import ConfigurationError

if TYPE_CHECKING:
    from michelangelo.workflow.schema.ray_data_io import WriteConfig
    from michelangelo.workflow.variables import DatasetVariable

_logger = logging.getLogger(__name__)

__all__ = ["save_datasets", "write_config_to_kwargs"]

# Ray version floors below which `Dataset.write_parquet` silently swallows the
# kwarg into `**arrow_parquet_args` instead of applying it, later raising a
# confusing `TypeError` from inside a write task rather than failing at
# config time. See `parquet_io.py`'s analogous 2.50 read-API gate.
_MIN_RAY_VERSION_FOR_MAX_ROWS_PER_FILE = version.parse("2.48")
_MIN_RAY_VERSION_FOR_MIN_ROWS_PER_FILE = version.parse("2.43")


def write_config_to_kwargs(write_config: WriteConfig | None) -> dict:
    """Convert a ``WriteConfig`` into ``write_parquet`` kwargs, gated by Ray version.

    ``max_rows_per_file``/``min_rows_per_file`` are only understood by
    ``Dataset.write_parquet`` from Ray 2.48/2.43 onward respectively. On an
    older Ray, the kwarg isn't rejected up front — it flows through to
    PyArrow's writer as an unexpected option and fails deep inside a write
    task, after the whole upstream computation has already run. Fail fast here
    instead.

    Args:
        write_config: The write options, or ``None`` for Ray's defaults.

    Returns:
        Keyword arguments for ``DatasetVariable.save_ray_dataset()``. Only
        options that are set appear in the result.

    Raises:
        ConfigurationError: If ``write_config`` requests a kwarg the
            installed Ray version doesn't support.
    """
    if not write_config:
        return {}

    ray_version = version.parse(ray.__version__)
    write_kwargs = {}

    if write_config.max_rows_per_file is not None:
        if ray_version < _MIN_RAY_VERSION_FOR_MAX_ROWS_PER_FILE:
            raise ConfigurationError(
                f"write_config.max_rows_per_file requires Ray >= "
                f"{_MIN_RAY_VERSION_FOR_MAX_ROWS_PER_FILE}, but installed Ray is "
                f"{ray.__version__}."
            )
        write_kwargs["max_rows_per_file"] = write_config.max_rows_per_file

    if write_config.min_rows_per_file is not None:
        if ray_version < _MIN_RAY_VERSION_FOR_MIN_ROWS_PER_FILE:
            raise ConfigurationError(
                f"write_config.min_rows_per_file requires Ray >= "
                f"{_MIN_RAY_VERSION_FOR_MIN_ROWS_PER_FILE}, but installed Ray is "
                f"{ray.__version__}."
            )
        write_kwargs["min_rows_per_file"] = write_config.min_rows_per_file

    # ``Dataset.write_parquet`` accepts ``concurrency`` in the Ray releases this
    # package has been checked against (2.41-2.48 and 2.51), so no version gate.
    if write_config.concurrency is not None:
        write_kwargs["concurrency"] = write_config.concurrency

    return write_kwargs


def save_datasets(datasets: dict[str, DatasetVariable], **write_kwargs) -> None:
    """Persist each dataset through its ``DatasetVariable``.

    Args:
        datasets: Mapping of dataset name to the ``DatasetVariable`` to persist.
        **write_kwargs: Forwarded to ``DatasetVariable.save_ray_dataset()``
            and ultimately to ``ray.data.Dataset.write_parquet()``.
            Supported keys include ``max_rows_per_file``,
            ``min_rows_per_file`` and ``concurrency``.
    """
    for dataset_name, dataset_var in datasets.items():
        _logger.info("Saving dataset: %s", dataset_name)
        t = time.time()

        if write_kwargs:
            _logger.info("Writing %s with write_kwargs=%s", dataset_name, write_kwargs)
        dataset_var.save_ray_dataset(**write_kwargs)
        _logger.info("Saved %s dataset in %.2f seconds", dataset_name, time.time() - t)
