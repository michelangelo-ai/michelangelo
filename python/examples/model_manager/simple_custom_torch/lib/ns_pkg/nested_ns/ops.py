"""Nested namespace package module (no __init__.py in nested_ns/)."""

from __future__ import annotations

import numpy as np


def as_float32(x: np.ndarray) -> np.ndarray:
    return x.astype(np.float32)


