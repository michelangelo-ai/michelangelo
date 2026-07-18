"""Dtype-driven dispatch to the sentinel values in :mod:`michelangelo.lib.constants`."""

from __future__ import annotations

import numpy as np

from michelangelo.lib.constants.sentinel import (
    BOOL_SENTINEL,
    BYTES_SENTINEL,
    FLOAT_SENTINEL,
    INT32_SENTINEL,
    STRING_SENTINEL,
)


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
