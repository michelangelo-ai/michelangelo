"""Shared, dependency-light constants for the michelangelo library.

These constants are part of the public interface and are safe to import from
any module. They are intentionally not owned by a single feature package so
they can be reused across unrelated consumers.

These are raw values only. Dtype-based selection (e.g. mapping a numpy dtype
to the right sentinel) is handled by callers that already depend on numpy —
see ``michelangelo.lib._internal.utils.numpy_utils.sentinel_for_numpy_dtype``
for the reference implementation.
"""

from .sentinel import (
    BOOL_SENTINEL,
    BYTES_SENTINEL,
    FLOAT_SENTINEL,
    INT32_SENTINEL,
    STRING_SENTINEL,
)

__all__ = [
    "BOOL_SENTINEL",
    "BYTES_SENTINEL",
    "FLOAT_SENTINEL",
    "INT32_SENTINEL",
    "STRING_SENTINEL",
]
