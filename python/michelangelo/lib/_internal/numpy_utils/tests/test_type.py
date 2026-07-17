"""Tests for ``michelangelo.lib._internal.numpy_utils.type``."""

from __future__ import annotations

import numpy as np

from michelangelo.lib._internal.numpy_utils.type import infer_dtype


class TestInferDtype:
    def test_uniform_float_array(self):
        arr = np.array([1.0, 2.0, 3.0])
        assert infer_dtype(arr) == np.float64

    def test_nested_object_array(self):
        arr = np.array([np.array([1.0, 2.0]), np.array([3.0])], dtype=object)
        assert infer_dtype(arr) == np.float64

    def test_all_empty_returns_none(self):
        arr = np.array([[], []], dtype=object)
        assert infer_dtype(arr) is None

    def test_scalar(self):
        assert infer_dtype(np.float32(1.0)) == np.float32
