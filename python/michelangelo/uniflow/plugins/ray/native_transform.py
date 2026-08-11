"""Ray execution adapter for the ``native_transform`` torch compute core.

Runs a fitted :class:`~michelangelo.lib.native_transform.torch.TransformSpec`
DAG over a ``ray.data.Dataset``: hydrates any missing numerical statistics
(percentiles, min/max/mean/std) level by level, materializes each level as a
:class:`~michelangelo.lib.native_transform.torch.TorchTransformModule`, and
runs it as a Ray ``map_batches`` stage via :class:`TorchBatchPredictor`. The
same ``TransformSpec`` DAG is TorchScript-exportable, so training-time
transforms here match serving-time transforms exactly.

Filesystem access for dataset I/O goes through the
:func:`~michelangelo.uniflow.plugins.ray.io._fs_path` seam already used by
:class:`~michelangelo.uniflow.plugins.ray.io.RayDatasetIO`, so this module has
no filesystem-registration side effects of its own.
"""

from __future__ import annotations

import logging
import time
from typing import Any

import numpy as np
import pyarrow as pa
import ray
import torch

from michelangelo.lib._internal.utils.numpy_utils import (
    assemble_output_table,
    infer_dtype,
    pad_ragged_tensor,
    pyarrow_to_numpy,
)
from michelangelo.lib.native_transform.torch import (
    TorchTransformBaseLayer,
    TransformSpec,
    get_transform_module,
)
from michelangelo.lib.native_transform.torch.constants import (
    DEFAULT_NUMERICAL_OUTPUT_DTYPE,
)
from michelangelo.uniflow.plugins.ray.io import _fs_path

_logger = logging.getLogger(__name__)

DEFAULT_MAP_BATCH_CONCURRENCY = 100
DEFAULT_NUM_STATS_BATCH_FN_SIZE = 500
DEFAULT_MAP_BATCH_SIZE = 20000

# Only numeric pyarrow types are supported by the torch transform layers.
PYARROW_TYPE_TO_TORCH_DATA_TYPE_MAP: dict[pa.DataType, torch.dtype] = {
    pa.int32(): torch.int32,
    pa.int64(): torch.int64,
    pa.float32(): torch.float32,
    pa.float64(): torch.float64,
}

__all__ = [
    "DataProcessor",
    "DefaultDataProcessor",
    "TorchBatchPredictor",
    "check_stats_exist",
    "compute_numerical_statistics",
    "filter_columns",
    "get_numerical_stats_names",
    "get_torch_dtype",
    "get_transform_torch_inference",
    "numerical_statistics_preparation",
    "read_dataset",
    "transform",
    "write_dataset",
]


def read_dataset(path: str) -> ray.data.Dataset:
    """Read a Parquet dataset from *path* as a Ray ``Dataset``.

    Args:
        path: Directory path or URL containing Parquet files (local,
            ``s3://``, etc). Resolved to a filesystem via
            :func:`~michelangelo.uniflow.plugins.ray.io._fs_path`.

    Returns:
        The loaded Ray ``Dataset``.
    """
    fs, resolved_path = _fs_path(path)
    return ray.data.read_parquet(resolved_path, filesystem=fs)


def write_dataset(ds: ray.data.Dataset, path: str, mode: str = "append") -> None:
    """Write a Ray ``Dataset`` to *path* as Parquet.

    Args:
        ds: The dataset to write.
        path: Destination directory path or URL. Resolved to a filesystem via
            :func:`~michelangelo.uniflow.plugins.ray.io._fs_path`.
        mode: Ray ``write_parquet`` write mode (``"append"``, ``"overwrite"``,
            or ``"error_if_exists"``).
    """
    fs, resolved_path = _fs_path(path)
    ds.write_parquet(resolved_path, filesystem=fs, mode=mode)


def filter_columns(
    ds: ray.data.Dataset, columns_to_keep: list[str]
) -> ray.data.Dataset:
    """Filter a Ray dataset down to the columns present in *columns_to_keep*.

    Args:
        ds: The dataset to filter.
        columns_to_keep: Column names to retain. Names not present in the
            dataset's schema are silently ignored.

    Returns:
        A dataset containing only the columns in both *columns_to_keep* and
        the dataset's schema.
    """
    return ds.select_columns(
        [col for col in columns_to_keep if col in ds.schema().names]
    )


def compute_numerical_statistics(
    ray_data_df: ray.data.Dataset,
    existing_numerical_stats: dict[str, float],
    numerical_statistics_computation_specs: dict[str, dict[str, Any]],
    numerical_statistics_batch_fn_size: int = DEFAULT_NUM_STATS_BATCH_FN_SIZE,
) -> dict[str, float]:
    """Compute numerical statistics (percentiles, max, min, mean, std) for columns.

    Statistics already present in *existing_numerical_stats* are skipped, and
    aggregate functions are batched to bound how many run in a single Ray
    ``aggregate`` call.

    Args:
        ray_data_df: Ray dataset to compute statistics on.
        existing_numerical_stats: Already computed statistics to skip
            (keyed ``"{column}_{stat}"``).
        numerical_statistics_computation_specs: Per-column statistics to
            compute, e.g. ``{"col": {"percentiles": ["0.5"], "max": True,
            "min": True, "mean": True, "std": True}}``.
        numerical_statistics_batch_fn_size: Max aggregate functions to run in
            a single ``Dataset.aggregate`` call.

    Returns:
        Newly computed statistics, keyed ``"{column}_{stat}"`` (e.g.
        ``"age_50"`` for the 50th percentile).
    """
    # Local import: pinning ray.data.aggregate at module import time can pick
    # up an inconsistent Ray version on the driver vs. workers.
    from ray.data.aggregate import Max, Mean, Min, Quantile, Std

    columns_needed = list(numerical_statistics_computation_specs.keys())
    if columns_needed:
        _logger.info(
            "Projecting dataset to %d columns for aggregation: %s",
            len(columns_needed),
            columns_needed,
        )
        ray_data_df = ray_data_df.select_columns(columns_needed)

    aggregate_fns = []
    for input_col, spec in numerical_statistics_computation_specs.items():
        aggregate_fns.extend(
            [
                Quantile(
                    input_col,
                    q=float(p),
                    alias_name=f"{input_col}_{int(100 * float(p))!s}",
                )
                for p in spec["percentiles"]
                if f"{input_col}_{int(100 * float(p))!s}"
                not in existing_numerical_stats
            ]
        )
        if spec["max"] and f"{input_col}_max" not in existing_numerical_stats:
            aggregate_fns.append(Max(input_col, alias_name=f"{input_col}_max"))
        if spec["min"] and f"{input_col}_min" not in existing_numerical_stats:
            aggregate_fns.append(Min(input_col, alias_name=f"{input_col}_min"))
        if spec["std"] and f"{input_col}_std" not in existing_numerical_stats:
            aggregate_fns.append(Std(input_col, alias_name=f"{input_col}_std"))
        if spec["mean"] and f"{input_col}_mean" not in existing_numerical_stats:
            aggregate_fns.append(Mean(input_col, alias_name=f"{input_col}_mean"))

    numerical_stats: dict[str, float] = {}
    if not aggregate_fns:
        return numerical_stats

    batch_count = (
        len(numerical_statistics_computation_specs)
        // numerical_statistics_batch_fn_size
        + 1
    )
    for i in range(batch_count):
        batch = aggregate_fns[
            i * numerical_statistics_batch_fn_size : (i + 1)
            * numerical_statistics_batch_fn_size
        ]
        numerical_stats.update(ray_data_df.aggregate(*batch))
    return numerical_stats


def get_torch_dtype(pa_type: pa.DataType) -> torch.dtype:
    """Return the torch dtype corresponding to a numeric pyarrow type.

    Recursively unwraps nested ``list``/``fixed_size_list`` types (e.g. for
    2D arrays like ``list<element: list<element: double>>``) down to their
    innermost value type.

    Args:
        pa_type: A pyarrow data type.

    Returns:
        The corresponding ``torch.dtype``.

    Raises:
        ValueError: If *pa_type* (after unwrapping any nesting) is not one of
            the supported numeric types.
    """
    while isinstance(pa_type, (pa.ListType, pa.FixedSizeListType)):
        pa_type = pa_type.value_type

    torch_dtype = PYARROW_TYPE_TO_TORCH_DATA_TYPE_MAP.get(pa_type)
    if torch_dtype is None:
        raise ValueError(
            f"Unsupported pyarrow type for torch dtype conversion: {pa_type}"
        )
    return torch_dtype


def transform(
    df: ray.data.Dataset,
    transform_spec: TransformSpec,
    feature_stats: dict[str, float],
    batch_size: int = DEFAULT_MAP_BATCH_SIZE,
    map_batch_concurrency: int = DEFAULT_MAP_BATCH_CONCURRENCY,
    num_gpus: int = 0,
) -> tuple[ray.data.Dataset, TransformSpec, dict[str, float]]:
    """Run a ``TransformSpec`` DAG over a Ray dataset, level by level.

    Non-numeric columns (strings, etc.) pass through unchanged; only numeric
    columns participate in the transform DAG. Each level hydrates any missing
    numerical statistics from the dataset before materializing and running
    that level's layers as a ``map_batches`` stage.

    Args:
        df: The dataset to transform.
        transform_spec: The transform DAG to run. Mutated in place: dtypes
            and fitted statistics are hydrated onto it as levels complete.
        feature_stats: Previously computed statistics to reuse, keyed
            ``"{column}_{stat}"``. Updated in place with any newly computed
            statistics.
        batch_size: Row batch size passed to ``Dataset.map_batches``.
        map_batch_concurrency: Concurrent ``map_batches`` workers.
        num_gpus: GPUs to request per worker; only honored when a CUDA device
            is available, otherwise runs on CPU.

    Returns:
        A ``(transformed_dataset, transform_spec, feature_stats)`` tuple.
    """
    data_dtype_map: dict[str, torch.dtype] = {}
    skipped_columns: dict[str, str] = {}
    for name, pa_type in zip(df.schema().names, df.schema().types):
        try:
            data_dtype_map[name] = get_torch_dtype(pa_type)
        except ValueError:
            skipped_columns[name] = str(pa_type)

    if skipped_columns:
        _logger.info(
            "Skipping %d non-numeric columns: %s", len(skipped_columns), skipped_columns
        )

    # Register output columns for cross-level dtype tracking. Use the layer's
    # explicit output_dtype when set, else fall back to float32 so later
    # levels' update_input_dtype can resolve this column's dtype.
    for layer_spec in transform_spec.transform_specs.values():
        for output_col in layer_spec.output_cols:
            data_dtype_map[output_col] = (
                layer_spec.output_dtype or DEFAULT_NUMERICAL_OUTPUT_DTYPE
            )

    max_level = transform_spec.get_max_transform_level()
    for level in range(max_level + 1):
        transform_spec.update_input_dtype(data_dtype_map, level)

    finished_levels: list[int] = []
    for level in range(max_level + 1):
        start = time.time()
        if not check_stats_exist(feature_stats, transform_spec, level):
            torch_inference = get_transform_torch_inference(
                transform_spec, finished_levels, level - 1
            )
            if torch_inference:
                df = df.map_batches(
                    torch_inference,
                    batch_size=batch_size,
                    num_gpus=num_gpus if torch.cuda.is_available() else 0,
                    zero_copy_batch=True,
                    batch_format="pyarrow",  # keeps passthrough columns zero-copy
                    concurrency=map_batch_concurrency,
                    num_cpus=1,  # GIL-bound torch inference; 1 Python thread per task
                )
            finished_levels.append(level - 1)
            df = df.materialize()
        numerical_statistics_preparation(df, feature_stats, transform_spec, level)
        _logger.info(
            "native transform level %d finished in %.3f seconds",
            level,
            time.time() - start,
        )

    torch_inference = get_transform_torch_inference(
        transform_spec, finished_levels, max_level
    )
    if torch_inference:
        df = df.map_batches(
            torch_inference,
            batch_size=batch_size,
            num_gpus=num_gpus if torch.cuda.is_available() else 0,
            zero_copy_batch=True,
            batch_format="pyarrow",
            concurrency=map_batch_concurrency,
            num_cpus=1,
        )

    return df, transform_spec, feature_stats


def get_numerical_stats_names(specs: dict[str, dict[str, Any]]) -> list[str]:
    """Return the flattened statistic names a computation spec dict requests.

    Args:
        specs: Per-column statistics specs, as passed to
            :func:`compute_numerical_statistics`.

    Returns:
        Names in the form ``"{column}_{stat}"`` (e.g. ``"age_mean"``,
        ``"age_50"`` for the 50th percentile).
    """
    names = []
    for col, spec in specs.items():
        if spec.get("min", False):
            names.append(f"{col}_min")
        if spec.get("max", False):
            names.append(f"{col}_max")
        if spec.get("std", False):
            names.append(f"{col}_std")
        if spec.get("mean", False):
            names.append(f"{col}_mean")
        names.extend(
            f"{col}_{int(100 * float(p))!s}" for p in spec.get("percentiles", [])
        )
    return names


def check_stats_exist(
    feature_stats: dict[str, float], transform_spec: TransformSpec, level: int
) -> bool:
    """Return whether every statistic *level* needs is already in *feature_stats*.

    Args:
        feature_stats: Already computed statistics, keyed
            ``"{column}_{stat}"``.
        transform_spec: The transform DAG being hydrated.
        level: The transform level to check.

    Returns:
        ``True`` if *level* requires no additional statistics computation.
    """
    required = get_numerical_stats_names(
        transform_spec.get_numerical_statistics_computation_specs(level)
    )
    return all(name in feature_stats for name in required)


def numerical_statistics_preparation(
    df: ray.data.Dataset,
    existing_feature_stats: dict[str, float],
    transform_spec: TransformSpec,
    level: int,
) -> None:
    """Compute and hydrate any numerical statistics *level* still needs.

    Computes missing statistics via :func:`compute_numerical_statistics`,
    merges them into *existing_feature_stats*, and hydrates the resulting
    values onto *transform_spec*'s standard-scaler, min-max-scaler, and
    bucketization layers for *level*.

    Args:
        df: The dataset to compute statistics from.
        existing_feature_stats: Previously computed statistics; updated in
            place with any newly computed values.
        transform_spec: The transform DAG being hydrated; mutated in place.
        level: The transform level to prepare statistics for.
    """
    specs = transform_spec.get_numerical_statistics_computation_specs(level)
    start = time.time()
    if specs:
        _logger.info("start computing numerical_standard_transform_parameters")
        new_stats = compute_numerical_statistics(df, existing_feature_stats, specs)
        existing_feature_stats.update(new_stats)
        transform_spec.update_numerical_standard_transform_parameters(
            existing_feature_stats, level
        )
        transform_spec.update_standard_scaler_specs(existing_feature_stats, level)
        _logger.info("standard_scaler_parameters computation finished.")
        transform_spec.update_min_max_scaler_specs(existing_feature_stats, level)
        _logger.info("min_max_scaler_parameters computation finished.")
        transform_spec.update_bucketization_specs(existing_feature_stats, level)
        _logger.info("bucketization_parameters computation finished.")
    _logger.info(
        "numerical_stats computation finished in %.3f seconds", time.time() - start
    )


def get_transform_torch_inference(
    transform_spec: TransformSpec,
    finished_levels: list[int],
    current_level: int,
) -> TorchBatchPredictor | None:
    """Materialize the torch layers for levels after the last finished level.

    Args:
        transform_spec: The transform DAG to materialize from.
        finished_levels: Levels already run; the next range starts right
            after the highest of these (or at ``0`` if empty).
        current_level: The last level to include in the materialized range.

    Returns:
        A predictor wrapping the materialized level range, or ``None`` if
        that range contains no layers.
    """
    start_level = 0 if not finished_levels else max(finished_levels) + 1
    transform_module = get_transform_module(transform_spec, start_level, current_level)
    if transform_module is None:
        return None

    if transform_spec.columns_to_keep is None:
        columns_to_keep = None
    else:
        # Keep both the user-requested columns and any input columns future
        # levels still need, since this predictor's output feeds them.
        max_level = transform_spec.get_max_transform_level()
        future_input_cols: set[str] = set()
        for level in range(current_level + 1, max_level + 1):
            future_input_cols.update(transform_spec.get_transform_input_cols(level))
        columns_to_keep = set(transform_spec.columns_to_keep) | future_input_cols

    return TorchBatchPredictor(
        model=transform_module,
        input_columns=transform_module.input_cols,
        output_columns=transform_module.output_cols,
        model_kwargs=None,
        columns_to_keep=columns_to_keep,
    )


def _pad_ragged_arrays(
    arrays: np.ndarray,
    ragged_fill_value: int | float | None = None,
) -> np.ndarray:
    """Pad an object-dtype array of variable-length numeric sub-arrays.

    Supports arbitrarily nested ragged structures (e.g. ``list<list<float32>>``
    from parquet). When *ragged_fill_value* is ``None``, the type-native
    sentinel from
    :func:`~michelangelo.lib._internal.utils.numpy_utils.sentinel_for_numpy_dtype`
    is used.

    Args:
        arrays: Object-dtype array whose elements are numeric ``np.ndarray``s.
        ragged_fill_value: Value to pad with. Defaults to the type-native
            sentinel when ``None``.

    Returns:
        A uniform numeric array with the original elements' dtype preserved.

    Raises:
        ValueError: If *arrays* is empty, contains non-``np.ndarray``
            elements, or its inferred dtype is non-numeric.
    """
    if len(arrays) == 0:
        raise ValueError("Cannot pad empty array")

    if not all(isinstance(a, np.ndarray) for a in arrays):
        raise ValueError("Not all elements are numpy arrays")

    dtype = infer_dtype(arrays)
    if dtype is None:
        raise ValueError("Cannot infer element dtype: all sub-arrays are empty")
    if dtype.kind in ("U", "S", "O"):
        raise ValueError(f"Non-numeric dtype: {dtype}")

    padded = pad_ragged_tensor(arrays, pad_value=ragged_fill_value)
    if padded.dtype != dtype:
        padded = padded.astype(dtype)
    return padded


class DataProcessor:
    """Abstract interface for pre/post-processing torch model inference batches."""

    def prepare_inputs(self, batch: dict[str, np.ndarray]) -> dict[str, torch.Tensor]:
        """Convert a raw numpy batch into model input tensors."""
        raise NotImplementedError

    def postprocess_outputs(
        self, outputs: dict[str, torch.Tensor]
    ) -> dict[str, np.ndarray]:
        """Convert model output tensors into the expected numpy format."""
        raise NotImplementedError


class DefaultDataProcessor(DataProcessor):
    """Numeric-only ``DataProcessor`` used by :class:`TorchBatchPredictor`.

    Args:
        input_columns: Column names to treat as model inputs. ``None`` means
            every column in the batch.
        output_columns: Column names to extract from model outputs. ``None``
            means every key the model returns.
    """

    def __init__(
        self,
        input_columns: list[str] | None = None,
        output_columns: list[str] | None = None,
    ) -> None:
        """Initialize the data processor.

        Args:
            input_columns: Column names to treat as model inputs. ``None``
                means every column in the batch.
            output_columns: Column names to extract from model outputs.
                ``None`` means every key the model returns.
        """
        self.input_columns = input_columns
        self.output_columns = output_columns

    def prepare_inputs(self, batch: dict[str, np.ndarray]) -> dict[str, torch.Tensor]:
        """Convert numeric batch columns to torch tensors, preserving dtype and shape.

        Only numeric types are supported. Object-dtype columns holding nested
        arrays (as parquet list columns commonly deserialize to) are stacked;
        ragged nested arrays are padded with a type-native sentinel via
        :func:`_pad_ragged_arrays`.

        Args:
            batch: Input batch as a dict of numpy arrays.

        Returns:
            A dict mapping each input column to its input tensor.

        Raises:
            ValueError: If a column has a non-numeric dtype (string, bytes,
                or bool), or an object-dtype column cannot be converted to a
                numeric array.
        """
        inputs: dict[str, torch.Tensor] = {}

        for col in self.input_columns or list(batch.keys()):
            input_data = batch[col]

            is_bool_array = input_data.dtype in (bool, np.bool_)
            if input_data.dtype.kind in ("U", "S") or is_bool_array:
                raise ValueError(
                    f"Column '{col}' has unsupported type {input_data.dtype}. "
                    "Only numeric types are allowed for torch model inference."
                )

            if input_data.dtype.kind == "O":
                try:
                    input_data = np.stack(input_data)
                except (ValueError, TypeError):
                    # np.stack failed: sub-arrays likely have different
                    # lengths (ragged). Pad to the batch max with the
                    # sentinel so downstream transforms can tell real data
                    # from padding.
                    try:
                        input_data = _pad_ragged_arrays(input_data)
                    except (ValueError, TypeError) as pad_err:
                        raise ValueError(
                            f"Column '{col}' has object dtype but cannot be "
                            f"converted to numeric array: {pad_err}"
                        ) from pad_err

                if input_data.dtype.kind == "O":
                    try:
                        # Recursively expand nested structures, e.g. a
                        # [B, 1, d] column where each element is itself a
                        # nested array.
                        input_data = np.array(input_data.tolist(), dtype=np.float64)
                    except (ValueError, TypeError) as e:
                        raise ValueError(
                            f"Column '{col}' contains non-numeric or ragged "
                            f"array data: {e}"
                        ) from e

            # np.ascontiguousarray keeps the memory layout compatible with
            # torch.from_numpy; the original dtype is preserved (rather than
            # casting to float32) to avoid precision loss for large integers.
            tensor = torch.from_numpy(np.ascontiguousarray(input_data))
            inputs[col] = tensor
        return inputs

    def postprocess_outputs(
        self, outputs: dict[str, torch.Tensor]
    ) -> dict[str, np.ndarray]:
        """Convert model output tensors/values to numpy arrays.

        Args:
            outputs: Model outputs keyed by column name.

        Returns:
            A dict mapping each output column to its numpy array.

        Raises:
            ValueError: If an expected output column is missing from
                *outputs*.
        """
        result: dict[str, np.ndarray] = {}
        for col in self.output_columns or list(outputs.keys()):
            if col not in outputs:
                raise ValueError(
                    f"Expected output column '{col}' not found in model outputs"
                )
            output_value = outputs[col]
            if isinstance(output_value, torch.Tensor):
                result[col] = output_value.detach().cpu().numpy()
            else:
                result[col] = np.array(output_value)
        return result


class TorchBatchPredictor:
    """Callable batch-inference adapter for running torch models over Ray datasets.

    Follows the Ray ``Dataset.map_batches`` callable-class pattern: an
    instance is passed directly to ``map_batches`` and invoked once per
    batch. Only the configured input columns are materialized to numpy;
    every other column passes through as native Arrow data untouched.

    Example:
        >>> predictor = TorchBatchPredictor(
        ...     model=my_torch_model,
        ...     input_columns=["features"],
        ...     output_columns=["predictions"],
        ... )
        >>> results = dataset.map_batches(
        ...     predictor,
        ...     batch_size=32,
        ...     num_gpus=1 if torch.cuda.is_available() else 0,
        ...     zero_copy_batch=True,
        ...     batch_format="pyarrow",
        ... )
    """

    def __init__(
        self,
        model: TorchTransformBaseLayer,
        input_columns: list[str] | None = None,
        output_columns: list[str] | None = None,
        model_kwargs: dict[str, Any] | None = None,
        columns_to_keep: list[str] | set[str] | None = None,
    ) -> None:
        """Initialize the batch predictor.

        Args:
            model: Torch model to run. A ``TorchTransformBaseLayer`` subclass
                taking and returning ``dict[str, torch.Tensor]``.
            input_columns: Columns to use as model inputs. ``None`` uses all
                batch columns.
            output_columns: Columns to extract from model outputs. ``None``
                uses every key the model returns.
            model_kwargs: Extra keyword arguments forwarded to the model's
                ``forward()``.
            columns_to_keep: Columns to retain in the output. ``None`` or
                empty keeps every column.

        Raises:
            ValueError: If *model* is ``None``.
        """
        if model is None:
            raise ValueError("model must be provided")

        self._model = model
        self.input_columns = input_columns
        self.output_columns = output_columns
        self.columns_to_keep = columns_to_keep
        self.model_kwargs = model_kwargs or {}
        self._data_processor: DefaultDataProcessor | None = None

    @property
    def data_processor(self) -> DefaultDataProcessor:
        """The lazily-instantiated data processor for this predictor."""
        if self._data_processor is None:
            self._data_processor = DefaultDataProcessor(
                input_columns=self.input_columns,
                output_columns=self.output_columns,
            )
        return self._data_processor

    def _predict(self, batch: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        """Run inference on a numpy batch and postprocess the model outputs."""
        model_inputs = self.data_processor.prepare_inputs(batch)
        if self.model_kwargs:
            model_outputs = self._model(model_inputs, **self.model_kwargs)
        else:
            model_outputs = self._model(model_inputs)
        return self.data_processor.postprocess_outputs(model_outputs)

    def __call__(self, batch: pa.Table) -> pa.Table:
        """Run inference on a PyArrow ``Table`` batch.

        Args:
            batch: A batch of input rows. Each model-input column must be
                convertible to numpy via
                :func:`~michelangelo.lib._internal.utils.numpy_utils.pyarrow_to_numpy`.

        Returns:
            A ``Table`` of model predictions plus pass-through input columns.
            Multi-dim outputs are encoded as native nested
            ``FixedSizeList<T, K>`` columns so the result writes directly to
            Parquet. When ``columns_to_keep`` is a non-empty collection, only
            those columns are retained.

        Raises:
            ValueError: If inference fails.
        """
        try:
            input_cols = (
                self.input_columns
                if self.input_columns is not None
                else list(batch.column_names)
            )
            input_batch = {
                col: pyarrow_to_numpy(batch.column(col)) for col in input_cols
            }

            with torch.inference_mode():
                processed_outputs = self._predict(input_batch)

            return assemble_output_table(
                batch, processed_outputs, columns_to_keep=self.columns_to_keep
            )
        except Exception as e:
            _logger.error("Batch inference failed: %s", e, exc_info=True)
            raise ValueError(f"Error during batch inference: {e}") from e
