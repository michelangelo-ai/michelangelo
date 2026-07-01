"""Shared helpers for tabular Lightning training.

Provides dtype maps, model-schema builders, sample-data normalisation, and
Ray-read kwargs construction. These are pure functions with no OSS-infra
dependencies (no Ray clusters, no storage backends) so they can be tested
without a Ray session.

All public functions are imported by ``trainer_task.py`` (PR 7c). Tests live
in ``tests/dataset_test.py``.
"""

from __future__ import annotations

import ast
import contextlib
import logging
import warnings
from typing import TYPE_CHECKING, Callable

import numpy as np

from michelangelo.lib.model_manager.schema.data_type import DataType
from michelangelo.lib.model_manager.schema.model_schema import ModelSchema
from michelangelo.lib.model_manager.schema.model_schema_item import ModelSchemaItem
from michelangelo.lib.trainer.torch.data_collate_functions import pad_ragged_lists

if TYPE_CHECKING:
    from michelangelo.workflow.schema.tabular_trainer import (
        ColumnConfig,
        LightningTrainerConfig,
    )

_logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Dtype maps
# ---------------------------------------------------------------------------

_TORCH_DTYPE_TO_NUMPY: dict[str, type] = {
    "torch.float32": np.float32,
    "torch.float64": np.float64,
    "torch.float": np.float32,
    "torch.int32": np.int32,
    "torch.int64": np.int64,
    "torch.int": np.int32,
    "torch.long": np.int64,
    "torch.int16": np.int16,
    "torch.short": np.int16,
    "torch.int8": np.int8,
    "torch.uint8": np.uint8,
    "torch.bool": np.bool_,
}

_TORCH_DTYPE_TO_DATATYPE: dict[str, DataType] = {
    "torch.float32": DataType.FLOAT,
    "torch.float64": DataType.DOUBLE,
    "torch.float": DataType.FLOAT,
    "torch.int32": DataType.INT,
    "torch.int64": DataType.LONG,
    "torch.int": DataType.INT,
    "torch.long": DataType.LONG,
    "torch.int16": DataType.SHORT,
    "torch.short": DataType.SHORT,
    "torch.int8": DataType.BYTE,
    "torch.uint8": DataType.BYTE,
    "torch.bool": DataType.BOOLEAN,
}


def _map_torch_dtype_to_numpy(torch_dtype_str: str) -> type:
    """Map a PyTorch dtype string to the corresponding NumPy dtype.

    Args:
        torch_dtype_str: PyTorch dtype string, e.g. ``"torch.float32"``.

    Returns:
        NumPy dtype class. Falls back to ``np.float32`` for unknown strings.

    Example:
        >>> _map_torch_dtype_to_numpy("torch.long")
        <class 'numpy.int64'>
    """
    return _TORCH_DTYPE_TO_NUMPY.get(torch_dtype_str, np.float32)


def _map_torch_dtype_to_datatype(torch_dtype_str: str) -> DataType:
    """Map a PyTorch dtype string to a Michelangelo ``DataType`` enum value.

    Args:
        torch_dtype_str: PyTorch dtype string, e.g. ``"torch.float32"``.

    Returns:
        ``DataType`` enum member. Falls back to ``DataType.FLOAT`` for unknowns.

    Example:
        >>> _map_torch_dtype_to_datatype("torch.int64")
        <DataType.LONG: 19>
    """
    return _TORCH_DTYPE_TO_DATATYPE.get(torch_dtype_str, DataType.FLOAT)


# ---------------------------------------------------------------------------
# Schema builder
# ---------------------------------------------------------------------------


def get_model_schema(
    input_columns: dict[str, ColumnConfig],
    output_columns: dict[str, ColumnConfig],
) -> ModelSchema:
    """Build a ``ModelSchema`` from column config dicts.

    Args:
        input_columns: Mapping of feature name → ``ColumnConfig`` for model
            inputs.
        output_columns: Mapping of output name → ``ColumnConfig`` for model
            outputs.

    Returns:
        ``ModelSchema`` with ``input_schema`` and ``output_schema`` populated.

    Example:
        >>> from michelangelo.workflow.schema.tabular_trainer import ColumnConfig
        >>> schema = get_model_schema(
        ...     input_columns={"x": ColumnConfig("torch.float32", [4])},
        ...     output_columns={"y": ColumnConfig("torch.float32")},
        ... )
        >>> len(schema.input_schema)
        1
    """
    _logger.info("Generating model schema")
    input_schema: list[ModelSchemaItem] = [
        ModelSchemaItem(
            name=name,
            data_type=_map_torch_dtype_to_datatype(cfg.data_type),
            shape=cfg.shape,
        )
        for name, cfg in input_columns.items()
    ]
    output_schema: list[ModelSchemaItem] = [
        ModelSchemaItem(
            name=name,
            data_type=_map_torch_dtype_to_datatype(cfg.data_type),
            shape=cfg.shape,
        )
        for name, cfg in output_columns.items()
    ]
    return ModelSchema(input_schema=input_schema, output_schema=output_schema)


# ---------------------------------------------------------------------------
# Sample-row normalisation
# ---------------------------------------------------------------------------


def _pad_row(row: dict) -> dict[str, np.ndarray]:
    """Normalise a raw single-sample dict to dense numpy arrays.

    Replaces the internal ``shared.utils.numpy_utils.pad.pad_batch`` on the
    OSS collate library:

    - ``list`` values are padded via :func:`pad_ragged_lists`.
    - ``np.ndarray`` with ``object`` dtype: string cells are parsed with
      ``ast.literal_eval`` then padded via :func:`pad_ragged_lists`.
    - Plain ``np.ndarray``: returned unchanged.
    - Scalar ``str``: parsed with ``ast.literal_eval`` if possible.
    - All other scalars: wrapped in a 0-D ``np.ndarray`` (use ``np.atleast_1d``
      downstream if a 1-D array is required).

    Args:
        row: Dict from ``train_data.take(1)[0]`` with metadata columns
            already removed.

    Returns:
        Dict mapping column names to dense ``np.ndarray`` values.
    """
    result: dict[str, np.ndarray] = {}
    for key, value in row.items():
        if isinstance(value, np.ndarray):
            if value.dtype == np.object_:
                parsed = [
                    ast.literal_eval(item) if isinstance(item, str) else item
                    for item in value.flat
                ]
                result[key] = pad_ragged_lists(parsed)
            else:
                result[key] = value
        elif isinstance(value, list):
            result[key] = pad_ragged_lists(value)
        elif isinstance(value, str):
            try:
                parsed_val = ast.literal_eval(value)
                result[key] = np.asarray(parsed_val, dtype=np.float32)
            except (ValueError, SyntaxError):
                result[key] = np.array([value], dtype=object)
        else:
            result[key] = np.array(value)
    return result


def collate_sample_row(
    sample_row: dict,
    data_collate_fn: Callable | None = None,
    metadata_columns: list[str] | None = None,
) -> dict[str, np.ndarray]:
    """Normalise a single raw sample row to numpy arrays for ``sample_data``.

    When *data_collate_fn* is provided the row is wrapped in a batch of 1 and
    routed through the same collate function used during training so that
    ``sample_data`` undergoes identical transformations (literal_eval, padding)
    as the data the model actually sees.

    ``metadata_columns`` are fed to the collate function (same as training) but
    removed from the result. Each remaining value must be a ``torch.Tensor``
    with a leading batch dimension of 1; values are squeezed and converted to
    numpy.

    Falls back to :func:`_pad_row` when no collate function is provided.

    Args:
        sample_row: A single raw row dict from ``train_data.take(1)[0]``.
        data_collate_fn: Collate function configured for training. Must accept
            a ``dict[str, np.ndarray]`` (batch of 1) and return a
            ``dict[str, torch.Tensor]`` with a leading batch dimension of 1.
            When provided the sample is routed through it to match training-time
            behaviour.
        metadata_columns: Column names aligned with
            ``LightningTrainerConfig.metadata_columns``. Removed from the
            collate result but not from the final ``sample_data``.

    Returns:
        Dict mapping column names to numpy arrays with the batch dimension
        removed.

    Raises:
        AttributeError: If *data_collate_fn* returns values that are not
            ``torch.Tensor`` objects (e.g. plain numpy arrays or scalars).
    """
    if data_collate_fn is not None:
        _logger.info(
            "Normalising sample row via data_collate_fn %s (batch-of-1 wrapper).",
            data_collate_fn,
        )
        batch: dict[str, np.ndarray] = {}
        for k, v in sample_row.items():
            if isinstance(v, np.ndarray):
                batch[k] = np.expand_dims(v, axis=0)
            elif isinstance(v, str):
                batch[k] = np.array([v], dtype=object)
            else:
                batch[k] = np.array([v])

        collated = data_collate_fn(batch)
        if metadata_columns:
            for name in metadata_columns:
                collated.pop(name, None)

        return {k: t.squeeze(0).detach().cpu().numpy() for k, t in collated.items()}
    else:
        _logger.info("No data_collate_fn configured; using _pad_row for sample data.")
        row = dict(sample_row)
        if metadata_columns:
            for name in metadata_columns:
                row.pop(name, None)
        return _pad_row(row)


# ---------------------------------------------------------------------------
# Sample-data extraction
# ---------------------------------------------------------------------------


def get_sample_data(
    sample_data_dict: dict[str, np.ndarray],
    input_columns: dict[str, ColumnConfig],
) -> list[dict[str, np.ndarray]]:
    """Extract and type-cast sample data for model inference testing.

    Filters ``sample_data_dict`` to only the features listed in
    ``input_columns``, converts each to the configured numpy dtype, and
    attempts to reshape to the expected shape when element counts match.

    Args:
        sample_data_dict: Normalised sample dict from :func:`collate_sample_row`.
        input_columns: Mapping of feature name → ``ColumnConfig``.

    Returns:
        A single-element list containing a dict of feature name → numpy array,
        suitable for passing to ``ModelMetadata._sample_data``.

        If a feature is absent from *sample_data_dict* it is skipped (warning
        logged). If the data shape does not match ``ColumnConfig.shape`` and
        cannot be reshaped, the data is passed through as-is (warning logged).

    Example:
        >>> import numpy as np
        >>> data = get_sample_data(
        ...     {"x": np.array(1.0)},
        ...     {"x": ColumnConfig("torch.float32")},
        ... )
        >>> len(data)
        1
    """
    _logger.info("Generating sample data")
    filtered: dict[str, np.ndarray] = {}

    for feature_name, cfg in input_columns.items():
        if feature_name not in sample_data_dict:
            _logger.warning(
                "Feature '%s' not found in sample data, skipping.", feature_name
            )
            continue

        raw = sample_data_dict[feature_name]

        # Parse stringified arrays produced by object-dtype Ray columns.
        if isinstance(raw, str) and raw.startswith("[") and raw.endswith("]"):
            with contextlib.suppress(ValueError, SyntaxError):
                raw = ast.literal_eval(raw)
        elif (
            isinstance(raw, np.ndarray)
            and raw.dtype == np.object_
            and raw.size > 0
            and isinstance(next(iter(raw.flat)), str)
        ):
            parsed_items = [
                ast.literal_eval(item)
                if isinstance(item, str) and item.startswith("[")
                else item
                for item in raw.flat
            ]
            # Use pad_ragged_lists to handle variable-length (ragged) rows.
            raw = pad_ragged_lists(parsed_items)

        target_dtype = _map_torch_dtype_to_numpy(cfg.data_type)
        data = np.asarray(raw, dtype=target_dtype)

        expected = tuple(cfg.shape)
        if expected and data.shape != expected:
            if data.size == np.prod(expected):
                data = data.reshape(expected)
                _logger.debug("Reshaped '%s' to %s.", feature_name, expected)
            else:
                _logger.warning(
                    "Feature '%s' shape mismatch: got %s, expected %s. Using as-is.",
                    feature_name,
                    data.shape,
                    expected,
                )

        filtered[feature_name] = data

    return [filtered]


# ---------------------------------------------------------------------------
# Deprecation warnings
# ---------------------------------------------------------------------------


def _raise_hyperparameter_deprecation_warnings(
    lightning_trainer_config: LightningTrainerConfig,
) -> None:
    """Warn for hyperparameter keys that now have first-class config fields.

    Args:
        lightning_trainer_config: The ``LightningTrainerConfig`` to inspect.
    """
    deprecated: dict[str, tuple[str, str]] = {
        "num_epochs": ("max_epochs", "lightning_trainer_kwargs"),
        "precision": ("precision", "lightning_trainer_kwargs"),
        "batch_size": ("batch_size", "dataloading_config.batch_iter_config"),
        "num_shuffle_batches": (
            "num_shuffle_batches",
            "dataloading_config.batch_iter_config",
        ),
    }

    if lightning_trainer_config.hyperparameters is None:
        return

    for key in lightning_trainer_config.hyperparameters:
        if key in deprecated:
            new_key, dest = deprecated[key]
            warnings.warn(
                f"Providing '{key}' in hyperparameters is deprecated. "
                f"Set '{new_key}' in '{dest}' instead.",
                DeprecationWarning,
                stacklevel=3,
            )
        else:
            warnings.warn(
                f"Providing '{key}' in hyperparameters is not supported by "
                "the tabular_trainer task and will have no effect.",
                UserWarning,
                stacklevel=3,
            )


def _raise_trainer_config_deprecation_warnings(
    lightning_trainer_config: LightningTrainerConfig,
) -> None:
    """Warn for any deprecated top-level ``LightningTrainerConfig`` fields.

    ``data_collate_fn``, ``project_name``, and ``experiment_name`` were
    dropped from the OSS schema so there is nothing to warn about here.
    The function is kept as an extension point — add entries to
    ``deprecated_config_fields`` when a new field is deprecated.

    Args:
        lightning_trainer_config: The ``LightningTrainerConfig`` to inspect.
    """
    deprecated_config_fields: dict[str, tuple[str, str]] = {
        # Example entry format:
        # "old_field": ("new_field", "config.lightning.<destination>"),
    }

    for key, (new_key, dest) in deprecated_config_fields.items():
        if getattr(lightning_trainer_config, key, None) is not None:
            warnings.warn(
                f"Providing '{key}' in config.lightning is deprecated. "
                f"Set '{new_key}' in '{dest}' instead.",
                DeprecationWarning,
                stacklevel=3,
            )


def raise_lightning_trainer_config_deprecation_warnings(
    lightning_trainer_config: LightningTrainerConfig,
) -> None:
    """Run all deprecation-warning checks for a ``LightningTrainerConfig``.

    Combines :func:`_raise_hyperparameter_deprecation_warnings` and
    :func:`_raise_trainer_config_deprecation_warnings`.

    Args:
        lightning_trainer_config: The ``LightningTrainerConfig`` to inspect.
    """
    _raise_hyperparameter_deprecation_warnings(lightning_trainer_config)
    _raise_trainer_config_deprecation_warnings(lightning_trainer_config)


# ---------------------------------------------------------------------------
# Ray read kwargs
# ---------------------------------------------------------------------------


def construct_read_kwargs(config: LightningTrainerConfig) -> dict:
    """Build ``ray.data.read_parquet`` kwargs from trainer config.

    Derives the ``columns`` projection from ``input_columns``, ``labels``,
    and ``metadata_columns`` (``output_columns`` excluded — they are not
    present in the raw dataset). Adds any ``ParquetReadConfig`` resource
    knobs that are explicitly set.

    Args:
        config: The ``LightningTrainerConfig`` to read from.

    Returns:
        Dict of kwargs ready to unpack into ``ray.data.read_parquet(path,
        **kwargs)``.

    Example:
        >>> from michelangelo.workflow.schema.tabular_trainer import (
        ...     LightningTrainerConfig, ColumnConfig
        ... )
        >>> cfg = LightningTrainerConfig(
        ...     model_class="m", metadata_columns=["id"],
        ...     input_columns={"x": ColumnConfig("torch.float32")},
        ...     output_columns={"y": ColumnConfig("torch.float32")},
        ...     labels={"label": ColumnConfig("torch.long")},
        ... )
        >>> construct_read_kwargs(cfg)
        {'columns': ['id', 'label', 'x']}
    """
    read_kwargs: dict = {}

    # Resource knobs from ParquetReadConfig.
    prc = (
        config.dataloading_config.parquet_read_config
        if config.dataloading_config
        else None
    )
    if prc is not None:
        _parquet_read_fields = (
            "num_cpus",
            "num_gpus",
            "memory",
            "concurrency",
            "override_num_blocks",
            "shuffle",
            "tensor_column_schema",
            "arrow_parquet_args",
        )
        for attr in _parquet_read_fields:
            val = getattr(prc, attr, None)
            if val is not None:
                read_kwargs[attr] = val

    # Column projection: inputs | labels | metadata (output_columns excluded).
    metadata = list(config.metadata_columns) if config.metadata_columns else []
    columns = sorted(
        set(config.input_columns.keys()) | set(config.labels.keys()) | set(metadata)
    )
    if columns:
        read_kwargs["columns"] = columns

    return read_kwargs
