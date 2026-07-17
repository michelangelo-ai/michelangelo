"""Tests for ``michelangelo.lib._internal.utils.numpy_utils.type``."""

from __future__ import annotations

import numpy as np

from michelangelo.lib._internal.utils.numpy_utils.type import infer_dtype


class TestInferDtype:
    """Tests for ``infer_dtype``."""

    def test_uniform_float_array(self):
        """A uniform float array infers ``float64``."""
        arr = np.array([1.0, 2.0, 3.0])
        assert infer_dtype(arr) == np.float64

    def test_nested_object_array(self):
        """A nested object array infers the leaf dtype."""
        arr = np.array([np.array([1.0, 2.0]), np.array([3.0])], dtype=object)
        assert infer_dtype(arr) == np.float64

    def test_all_empty_returns_none(self):
        """An all-empty array infers no dtype."""
        arr = np.array([[], []], dtype=object)
        assert infer_dtype(arr) is None

    def test_scalar(self):
        """A numpy scalar infers its own dtype."""
        assert infer_dtype(np.float32(1.0)) == np.float32
