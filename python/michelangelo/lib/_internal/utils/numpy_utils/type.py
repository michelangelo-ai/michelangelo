"""Recursive dtype inference for nested/ragged numpy arrays."""

from __future__ import annotations

import numpy as np


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
