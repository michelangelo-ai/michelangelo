"""Shared, dependency-light constants for the michelangelo library.

These constants are part of the public interface and are safe to import from
any module. They are intentionally not owned by a single feature package so
they can be reused across unrelated consumers.
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
