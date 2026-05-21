"""Numpy padding utilities used by the data-collate helpers.

Kept private to the trainer package; callers should use
:mod:`michelangelo.lib.trainer.torch.data_collate_functions`.
"""

from __future__ import annotations

from typing import Any

import numpy as np

INT32_SENTINEL = -(2**31)  # -2147483648, np.iinfo(np.int32).min
FLOAT_SENTINEL = float("nan")
STRING_SENTINEL = ""
BYTES_SENTINEL = b""
BOOL_SENTINEL = False


def sentinel_for_numpy_dtype(dtype: np.dtype) -> float | int | str | bytes | bool:
    """Return the type-native sentinel value for *dtype*."""
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
