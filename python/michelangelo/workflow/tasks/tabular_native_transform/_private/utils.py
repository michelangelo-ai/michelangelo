"""Sampling, numpy-conversion, and path-resolution helpers for native transform.

``_private/`` convention: this file lives in ``_private/`` — do not import
directly from this path. Import from
``michelangelo.workflow.tasks.tabular_native_transform`` instead.
"""

from __future__ import annotations

import logging
import os
from typing import TYPE_CHECKING, Any

import numpy as np

from michelangelo.lib.model_manager.schema import DataType, ModelSchemaItem

if TYPE_CHECKING:
    from collections.abc import Sequence

    from michelangelo.workflow.variables import DatasetVariable

_logger = logging.getLogger(__name__)

__all__ = [
    "PREFERRED_DATASET_ORDER",
    "col_to_numpy",
    "convert_to_numpy_sample",
    "data_type_to_dtype",
    "get_sample_data_from_datasets",
    "resolve_data_file_path",
]

_DATA_ROOT_ENV_VAR = "MICHELANGELO_DATA_ROOT"

PREFERRED_DATASET_ORDER = ["train", "training", "validation", "test"]
"""Dataset-name ordering used to prefer training data for stat computation."""


def resolve_data_file_path(path: str) -> str:
    """Resolve a possibly-relative data file path to an absolute path.

    Absolute paths are returned unchanged. A relative path is resolved
    against the ``MICHELANGELO_DATA_ROOT`` environment variable when it is
    set, or the current working directory otherwise — this task typically
    runs inside a container image that bundles spec/data files alongside the
    task code, so callers set ``MICHELANGELO_DATA_ROOT`` to that image's data
    root in their deployment environment.

    Args:
        path: A file path, absolute or relative.

    Returns:
        The resolved absolute path.
    """
    if os.path.isabs(path):
        return path
    root = os.environ.get(_DATA_ROOT_ENV_VAR, os.getcwd())
    return os.path.join(root, path)


def get_sample_data_from_datasets(
    datasets: dict[str, DatasetVariable],
) -> dict | None:
    """Get a sample row from the datasets for shape derivation.

    Samples BEFORE transformation to ensure data is available (Ray datasets
    may be consumed/invalidated after transformation, and input columns may
    be dropped). Prefers train/training dataset, falls back to any available
    dataset.

    Args:
        datasets: Dictionary of dataset variables.

    Returns:
        Sample row as dict (column -> value), or ``None`` if not available.
    """
    ordered_names = [name for name in PREFERRED_DATASET_ORDER if name in datasets]
    ordered_names.extend(name for name in datasets if name not in ordered_names)

    for name in ordered_names:
        dataset_var = datasets[name]
        if dataset_var.value is None:
            continue
        try:
            sample_rows = dataset_var.value.take(1)
            if sample_rows:
                return sample_rows[0]
        except Exception as e:
            _logger.warning("Failed to sample from dataset %r: %s", name, e)

    _logger.warning("No sample data available from any dataset")
    return None


def data_type_to_dtype(data_type: DataType) -> np.dtype:
    """Numpy dtype for empty placeholder arrays when a cell is missing or null.

    Used with :func:`convert_to_numpy_sample` when an ``input_schema`` is
    provided, so missing values match the declared feature type.

    Args:
        data_type: The declared schema data type.

    Returns:
        The corresponding numpy dtype, or ``np.object_`` for unmapped types.
    """
    dtype_mapping: dict[DataType, np.dtype] = {
        DataType.FLOAT: np.float32,
        DataType.DOUBLE: np.float64,
        DataType.INT: np.int32,
        DataType.LONG: np.int64,
        DataType.SHORT: np.int16,
        DataType.BYTE: np.int8,
        DataType.BOOLEAN: np.bool_,
        DataType.STRING: np.object_,
    }
    return dtype_mapping.get(data_type, np.object_)


def convert_to_numpy_sample(
    row: dict[str, Any] | None,
    input_schema: Sequence[ModelSchemaItem] | None = None,
) -> list[dict[str, np.ndarray]] | None:
    """Convert one dataset row to model metadata ``sample_data`` format.

    ``ModelMetadata.sample_data`` expects ``list[dict[str, np.ndarray]]`` (a
    batch of one row). Ray ``Dataset.take(1)[0]`` rows use scalars, lists,
    numpy arrays, or Arrow-backed types; this normalizes every cell to an
    ``ndarray``.

    When ``input_schema`` is set (typically ``derived_schema.input_schema``
    from
    :func:`~michelangelo.lib.native_transform.torch.schema.derive_native_transform_schema`),
    only those columns are kept, in schema order. Keys absent from ``row``
    and ``None`` cells use an empty array whose dtype matches each item's
    :class:`~michelangelo.lib.model_manager.schema.DataType`.

    Args:
        row: A single record dict, or ``None``.
        input_schema: Optional transform input schema; extra keys in ``row``
            are dropped.

    Returns:
        A one-element list ``[{ col: ndarray, ... }]``, or ``None`` if
        ``row`` is ``None``.
    """
    if row is None:
        return None
    if input_schema is not None:
        return [
            {
                item.name: col_to_numpy(
                    row.get(item.name),
                    dtype=data_type_to_dtype(item.data_type),
                )
                for item in input_schema
            }
        ]
    return [{key: col_to_numpy(value) for key, value in row.items()}]


def col_to_numpy(value: Any, dtype: np.dtype | None = None) -> np.ndarray:
    """Normalize a single dataset cell value into a numpy ``ndarray``.

    Args:
        value: The cell value — ``None``, a numpy array, a torch tensor, a
            scalar, ``bytes``, a ``str``, or an array-like sequence.
        dtype: Optional dtype to coerce the result to. Used for empty
            placeholders (``value is None``) and ragged sequences containing
            ``None`` entries.

    Returns:
        The value as a numpy ``ndarray``.
    """
    if value is None:
        return np.array([], dtype=dtype or np.object_)
    if isinstance(value, np.ndarray):
        return (
            np.asarray(value, dtype=dtype) if dtype is not None else np.asarray(value)
        )
    if hasattr(value, "detach") and callable(getattr(value, "detach", None)):
        try:
            return np.asarray(value.detach().cpu().numpy())
        except Exception:
            pass
    if isinstance(value, (bytes, bytearray)):
        return np.array([bytes(value)], dtype=dtype or np.object_)
    if isinstance(value, str):
        return np.array([value], dtype=dtype or np.object_)
    if isinstance(value, (bool, int, float, np.integer, np.floating, np.bool_)):
        return (
            np.array([value], dtype=dtype) if dtype is not None else np.array([value])
        )
    if (
        dtype is not None
        and isinstance(value, (list, tuple))
        and any(v is None for v in value)
    ):
        default = np.dtype(dtype).type(0)
        return np.array([default if v is None else v for v in value], dtype=dtype)
    return np.asarray(value, dtype=dtype) if dtype is not None else np.asarray(value)
