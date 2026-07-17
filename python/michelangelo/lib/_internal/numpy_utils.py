"""Numpy padding and dtype utilities shared across michelangelo.lib."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any

import numpy as np
import pyarrow as pa

if TYPE_CHECKING:
    from collections.abc import Collection

_logger = logging.getLogger(__name__)

INT32_SENTINEL = -(2**31)  # -2147483648, np.iinfo(np.int32).min
FLOAT_SENTINEL = float("nan")
STRING_SENTINEL = ""
BYTES_SENTINEL = b""
BOOL_SENTINEL = False


def sentinel_for_numpy_dtype(dtype: np.dtype) -> float | int | str | bytes | bool:
    """Return the type-native sentinel value for *dtype*.

    Float dtypes use NaN; signed integers int32 and int64 use ``INT32_SENTINEL``.
    int8 and int16 raise ``ValueError`` (``INT32_SENTINEL`` does not fit); pass an
    explicit ``pad_value`` to :func:`pad_ragged_tensor` or cast to a wider dtype first.
    Other integer dtypes (e.g. unsigned) raise ``ValueError``.
    Unicode and object dtypes use :data:`STRING_SENTINEL`; bytes use
    :data:`BYTES_SENTINEL`; bool uses :data:`BOOL_SENTINEL`.
    Raises ``ValueError`` for unsupported dtypes (e.g. void).
    """
    if np.issubdtype(dtype, np.floating):
        return FLOAT_SENTINEL
    if np.issubdtype(dtype, np.integer):
        key = np.dtype(dtype)
        if key in (np.dtype(np.int32), np.dtype(np.int64)):
            return INT32_SENTINEL
        raise ValueError(f"No sentinel defined for dtype: {dtype}")
    if dtype.kind in ("U", "O"):
        return STRING_SENTINEL
    if dtype.kind == "S":
        return BYTES_SENTINEL
    if dtype.kind == "b":
        return BOOL_SENTINEL
    raise ValueError(f"No sentinel defined for dtype: {dtype}")


def infer_dtype(arr: np.ndarray | list) -> type | None:
    """Recursively infer the common leaf dtype of a nested object array."""
    if not isinstance(arr, (np.ndarray, list)):
        if isinstance(arr, np.generic):
            return arr.dtype
        return np.array(arr).dtype

    if isinstance(arr, np.ndarray) and arr.dtype != object:
        return arr.dtype

    for elem in arr:
        if isinstance(elem, list) and len(elem) == 0:
            continue
        return infer_dtype(elem)

    return None


def pad_ragged_tensor(arr: np.ndarray, pad_value: Any | None = None) -> np.ndarray:
    """Recursively pad a ragged tensor to uniform shape at each nesting level."""
    if arr.dtype != object:
        return arr

    if isinstance(arr, np.ndarray) and len(arr) > 0:
        try:
            if all(isinstance(elem, np.ndarray) and elem.ndim == 1 for elem in arr):
                first_non_empty = next((a for a in arr if a.size > 0), None)
                if first_non_empty is not None and first_non_empty.dtype.kind not in (
                    "U",
                    "S",
                    "O",
                ):
                    dtype = next((a.dtype for a in arr if a.size > 0), np.int32)
                    pad_value = (
                        sentinel_for_numpy_dtype(dtype)
                        if pad_value is None
                        else pad_value
                    )
                    return _pad_1d_arrays_fast(arr, pad_value, dtype)
        except (AttributeError, TypeError):
            pass

    max_shape = _get_max_shape_recursive(arr)
    if not max_shape:
        return arr

    dtype = infer_dtype(arr)
    if dtype is None:
        return arr

    pad_value = sentinel_for_numpy_dtype(dtype) if pad_value is None else pad_value

    return _pad_array_recursive(arr, max_shape, pad_value, dtype, level=0)


def _pad_1d_arrays_fast(arr: np.ndarray, pad_value: Any, dtype: type) -> np.ndarray:
    """Fast-path for padding an object array of 1D arrays to a 2D array."""
    max_len = max(a.size for a in arr)
    padded = np.full((len(arr), max_len), pad_value, dtype=dtype)
    for i, a in enumerate(arr):
        if a.size > 0:
            padded[i, : a.size] = a
    return padded


def _get_max_shape_recursive(arr: np.ndarray | list) -> list[int]:
    """Recursively determine the maximum shape at each dimension level."""
    if not isinstance(arr, (np.ndarray, list)):
        return []

    if isinstance(arr, np.ndarray) and arr.dtype != object:
        return list(arr.shape)

    if len(arr) == 0:
        return [0]

    max_len = len(arr)
    child_shapes = []
    for elem in arr:
        if isinstance(elem, (np.ndarray, list)):
            child_shape = _get_max_shape_recursive(elem)
            child_shapes.append(child_shape)

    if not child_shapes:
        return [max_len]

    max_child_shape = []
    max_depth = max(len(s) for s in child_shapes) if child_shapes else 0
    for dim in range(max_depth):
        max_at_dim = max((s[dim] if dim < len(s) else 0) for s in child_shapes)
        max_child_shape.append(max_at_dim)

    return [max_len, *max_child_shape]


def _pad_array_recursive(
    arr: np.ndarray | list,
    target_shape: list[int],
    pad_value: Any,
    dtype: type,
    level: int = 0,
) -> np.ndarray:
    """Recursively pad *arr* to *target_shape*."""
    current_target = target_shape[level]

    if isinstance(arr, np.ndarray):
        arr_len = arr.shape[0] if arr.ndim > 0 else 1
    elif isinstance(arr, list):
        arr_len = len(arr)
    else:
        arr_len = 1

    padded_list = []
    for i in range(current_target):
        if i < arr_len:
            elem = arr[i] if isinstance(arr, (np.ndarray, list)) else arr
            if isinstance(elem, (list, np.ndarray)):
                padded_elem = _pad_array_recursive(
                    elem, target_shape, pad_value, dtype, level + 1
                )
            else:
                padded_elem = elem
            padded_list.append(padded_elem)
        else:
            if level + 1 < len(target_shape):
                child_shape = target_shape[level + 1 :]
                pad_type = dtype if dtype.kind not in {"S", "U"} else None
                padded_elem = np.full(child_shape, pad_value, dtype=pad_type)
            else:
                padded_elem = pad_value
            padded_list.append(padded_elem)

    try:
        result = np.stack(padded_list)
    except ValueError:
        result = np.array(padded_list, dtype=dtype)

    return result


def _try_extract_uniform_nested(
    data: pa.Array,
) -> tuple[tuple[int, ...], np.ndarray] | None:
    """Extract ``(shape, flat_buffer)`` from a uniformly-shaped nested list array.

    Recursively traverses ``list<T>``, ``large_list<T>``, and
    ``fixed_size_list<T, N>`` at any nesting depth. At each level, if all
    elements share the same length, the dimension is recorded and recursion
    continues into the values. The caller reshapes the flat leaf buffer to the
    collected shape.

    Args:
        data: A non-chunked PyArrow ``Array``. Must be contiguous with
            ``offset=0`` — guaranteed when called from :func:`pyarrow_to_numpy`
            after ``combine_chunks()``.

    Returns:
        A ``(shape, flat)`` tuple where ``shape`` is the full N-D shape and
        ``flat`` is the leaf numpy array, or ``None`` if any dimension is
        ragged or the leaf cannot be converted to numpy (letting the caller
        fall back gracefully).
    """
    n = len(data)

    if pa.types.is_fixed_size_list(data.type):
        size = data.type.list_size
        inner = _try_extract_uniform_nested(data.values)
        if inner is None:
            return None
        inner_shape, flat = inner
        return (n, size, *inner_shape[1:]), flat

    if pa.types.is_list(data.type) or pa.types.is_large_list(data.type):
        offsets = data.offsets.to_numpy()
        lengths = np.diff(offsets)
        if len(lengths) == 0 or not np.all(lengths == lengths[0]):
            return None
        size = int(lengths[0])
        if size == 0:
            return None
        # Slice to the exact referenced range; PyArrow values may extend
        # beyond offsets[-1].
        values = data.values.slice(int(offsets[0]), int(offsets[-1]) - int(offsets[0]))
        inner = _try_extract_uniform_nested(values)
        if inner is None:
            return None
        inner_shape, flat = inner
        return (n, size, *inner_shape[1:]), flat

    try:
        flat = data.to_numpy(zero_copy_only=False)
        return (len(flat),), flat
    except Exception:
        return None


def pyarrow_to_numpy(data: pa.ChunkedArray | pa.Array) -> np.ndarray:
    """Convert a PyArrow array to a numpy array.

    For uniformly-shaped nested list columns — ``list<T>``, ``large_list<T>``,
    ``fixed_size_list<T, N>``, or any combination at arbitrary depth — extracts
    the flat values buffer and reshapes directly to an N-D typed array. This
    avoids object-dtype numpy arrays that trigger expensive recursive padding.

    Falls back to PyArrow's ``to_numpy()`` for ragged arrays, scalar columns,
    and any type that cannot be uniformly reshaped.

    Args:
        data: A PyArrow ``ChunkedArray`` or ``Array`` to convert.

    Returns:
        A numpy array representation of the input data.

    Examples:
        >>> import pyarrow as pa
        >>> arr = pa.array([[1, 2], [3, 4]], type=pa.list_(pa.int64(), 2))
        >>> pyarrow_to_numpy(arr).shape
        (2, 2)

        >>> arr = pa.array([[1, 2], [3, 4], [5, 6]])
        >>> pyarrow_to_numpy(arr).shape
        (3, 2)

        >>> arr = pa.array([1, 2, 3, 4, 5])
        >>> pyarrow_to_numpy(arr).shape
        (5,)
    """
    if isinstance(data, pa.ChunkedArray):
        data = data.combine_chunks()

    result = _try_extract_uniform_nested(data)
    if result is not None:
        shape, flat = result
        return flat.reshape(shape)

    return data.to_numpy(zero_copy_only=False)


def numpy_to_pyarrow_array(
    arr: np.ndarray,
    target_type: pa.DataType | None = None,
) -> pa.Array:
    """Convert a single numpy array to a native PyArrow array.

    Encoding:

    - 1-D arrays become flat PyArrow arrays (zero-copy where dtype allows).
    - 2-D arrays of shape ``(B, 1)`` are encoded as ``list<T>`` to preserve
      backward compatibility with callers that introspect the Arrow schema as
      a variable-length list of length 1.
    - General N-D arrays of shape ``(B, d1, d2, ...)`` are encoded as nested
      ``FixedSizeList<T, d_i>`` arrays — built directly from the contiguous
      flat values buffer, with no per-row Python objects. The fixed-size
      encoding carries each trailing dimension in the schema and round-trips
      through :func:`pyarrow_to_numpy` back to the same N-D shape.
    - Object-dtype arrays whose elements are lists or numpy arrays (e.g. ragged
      tensor columns) defer to ``pa.array(arr.tolist())`` so PyArrow can infer
      the nested element type from Python objects.

    Args:
        arr: The numpy array to convert.
        target_type: Optional PyArrow type for the resulting array. When
            provided, prevents silent type promotion — e.g. ragged int32 arrays
            stay int32 instead of being promoted to int64 by PyArrow's default
            int inference. Ignored when the result is built via
            ``FixedSizeListArray.from_arrays`` (the flat buffer dtype already
            determines the leaf type).

    Returns:
        A PyArrow ``Array``.
    """
    try:
        if arr.ndim == 1:
            if (
                arr.dtype == object
                and len(arr) > 0
                and isinstance(arr[0], (list, np.ndarray))
            ):
                result: pa.Array = pa.array(arr.tolist(), type=target_type)
            elif target_type is not None:
                result = pa.array(arr, type=target_type)
            else:
                result = pa.array(arr)
        elif arr.ndim == 2 and arr.shape[1] == 1:
            # Preserve list<T> for (B, 1) — legacy schema contract.
            result = pa.array(arr.tolist(), type=target_type)
        else:
            # General N-D: build nested FixedSizeListArray innermost dim outward.
            flat = pa.array(np.ascontiguousarray(arr).reshape(-1))
            result = flat
            for size in reversed(arr.shape[1:]):
                result = pa.FixedSizeListArray.from_arrays(result, size)
    except Exception as e:
        _logger.debug(
            "Direct PyArrow conversion failed: %s. Using tolist() fallback.", e
        )
        return pa.array(arr.tolist(), type=target_type)
    return result


def assemble_output_table(
    input_table: pa.Table,
    predictions: dict[str, np.ndarray],
    columns_to_keep: Collection[str] | None = None,
    extra_columns: dict[str, pa.Array | pa.ChunkedArray] | None = None,
    raise_on_collision: bool = False,
) -> pa.Table:
    """Assemble a model-output table from passthrough inputs and predictions.

    Input columns are reused verbatim from ``input_table`` as zero-copy Arrow
    chunks — no numpy round-trip. Each prediction array is encoded to a native
    Arrow type via :func:`numpy_to_pyarrow_array`, and any ``extra_columns``
    (e.g. constant metadata columns) are appended as already-built Arrow arrays.

    By default, an output that shares a name with an input column overwrites it
    — the in-place transform semantics the native-transform torch path relies
    on. Set ``raise_on_collision=True`` to instead reject such overlaps with a
    ``ValueError``; this is the guard the inference path uses to avoid silently
    clobbering passthrough data.

    Columns are assembled in the order: input columns, predictions, then
    ``extra_columns``. When ``columns_to_keep`` is a non-empty collection only
    those columns are retained (in assembled order); ``None`` or an empty
    collection keeps every column.

    Args:
        input_table: The batch whose columns pass through unchanged.
        predictions: Model outputs as numpy arrays, keyed by column name.
        columns_to_keep: Optional subset of columns to retain in the result.
        extra_columns: Optional pre-built Arrow columns to append.
        raise_on_collision: When True, raise if a prediction or extra column
            name already exists in ``input_table`` instead of overwriting it.

    Returns:
        A PyArrow ``Table`` of passthrough, prediction, and extra columns.

    Raises:
        ValueError: If ``raise_on_collision`` is True and a prediction or extra
            column name collides with an existing input column.
    """
    extra_columns = extra_columns or {}

    if raise_on_collision:
        collisions = set(input_table.column_names) & (
            set(predictions) | set(extra_columns)
        )
        if collisions:
            raise ValueError(
                f"The output columns {collisions} already exist in the input "
                "dataset. This can cause conflicts. Please check your input data."
            )

    output_arrays: dict[str, pa.Array | pa.ChunkedArray] = {
        col: input_table.column(col) for col in input_table.column_names
    }
    for col, arr in predictions.items():
        output_arrays[col] = numpy_to_pyarrow_array(arr)
    output_arrays.update(extra_columns)

    names = (
        [n for n in output_arrays if n in columns_to_keep]
        if columns_to_keep
        else list(output_arrays)
    )
    return pa.Table.from_arrays([output_arrays[n] for n in names], names=names)
