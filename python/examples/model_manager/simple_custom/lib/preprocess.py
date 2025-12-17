"""Preprocessing helpers for the simple_custom example."""

from __future__ import annotations

import numpy as np


def ensure_int32(x: np.ndarray) -> np.ndarray:
    """Convert `x` to int32."""
    return x.astype(np.int32)
