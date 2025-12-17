"""Nested namespace package module (no __init__.py in nested_ns/)."""

from __future__ import annotations

import numpy as np


def add_zero(x: np.ndarray) -> np.ndarray:
    return x + np.int32(0)
