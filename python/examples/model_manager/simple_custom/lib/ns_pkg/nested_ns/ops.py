"""Nested namespace package module (no __init__.py in nested_ns/)."""

from __future__ import annotations

import numpy as np


def add_zero(x: np.ndarray) -> np.ndarray:
    """Add 0 to `x` (exercise nested namespace package imports)."""
    return x + np.int32(0)
