"""Tests for ``michelangelo.lib._internal.numpy_utils``."""

from __future__ import annotations

import numpy as np
import pytest

from michelangelo.lib._internal.numpy_utils import (
    BOOL_SENTINEL,
    BYTES_SENTINEL,
    FLOAT_SENTINEL,
    INT32_SENTINEL,
    STRING_SENTINEL,
    infer_dtype,
    pad_ragged_tensor,
    sentinel_for_numpy_dtype,
)

# -----------------------------------------------------------------------------
# sentinel_for_numpy_dtype
# -----------------------------------------------------------------------------


class TestSentinelForNumpyDtype:
    def test_float32(self):
        assert np.isnan(sentinel_for_numpy_dtype(np.dtype(np.float32)))

    def test_float64(self):
        assert np.isnan(sentinel_for_numpy_dtype(np.dtype(np.float64)))

    def test_int32(self):
        assert sentinel_for_numpy_dtype(np.dtype(np.int32)) == INT32_SENTINEL

    def test_int64(self):
        assert sentinel_for_numpy_dtype(np.dtype(np.int64)) == INT32_SENTINEL

    def test_int8_raises(self):
        with pytest.raises(ValueError):
            sentinel_for_numpy_dtype(np.dtype(np.int8))

    def test_unicode(self):
        assert sentinel_for_numpy_dtype(np.dtype("U10")) == STRING_SENTINEL

    def test_object(self):
        assert sentinel_for_numpy_dtype(np.dtype(object)) == STRING_SENTINEL

    def test_bytes(self):
        assert sentinel_for_numpy_dtype(np.dtype("S10")) == BYTES_SENTINEL

    def test_bool(self):
        assert sentinel_for_numpy_dtype(np.dtype(bool)) == BOOL_SENTINEL

    def test_float_sentinel_is_nan(self):
        assert np.isnan(FLOAT_SENTINEL)

    def test_int32_sentinel_value(self):
        assert INT32_SENTINEL == -(2**31)


# -----------------------------------------------------------------------------
# infer_dtype
# -----------------------------------------------------------------------------


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


# -----------------------------------------------------------------------------
# pad_ragged_tensor
# -----------------------------------------------------------------------------


class TestPadRaggedTensor:
    def test_uniform_array_unchanged(self):
        arr = np.array([1.0, 2.0, 3.0])
        result = pad_ragged_tensor(arr)
        np.testing.assert_array_equal(result, arr)

    def test_pads_1d_ragged_to_2d(self):
        arr = np.array([np.array([1.0, 2.0]), np.array([3.0])], dtype=object)
        result = pad_ragged_tensor(arr)
        assert result.shape == (2, 2)
        assert result[0, 0] == 1.0
        assert result[0, 1] == 2.0
        assert result[1, 0] == 3.0
        assert np.isnan(result[1, 1])

    def test_custom_pad_value(self):
        arr = np.array(
            [np.array([1, 2], dtype=np.int32), np.array([3], dtype=np.int32)],
            dtype=object,
        )
        result = pad_ragged_tensor(arr, pad_value=-1)
        assert result[1, 1] == -1

    def test_int_array_uses_int32_sentinel(self):
        arr = np.array(
            [np.array([1], dtype=np.int32), np.array([2, 3], dtype=np.int32)],
            dtype=object,
        )
        result = pad_ragged_tensor(arr)
        assert result[0, 1] == INT32_SENTINEL

    def test_empty_object_array(self):
        arr = np.array([], dtype=object)
        result = pad_ragged_tensor(arr)
        assert len(result) == 0

    def test_nested_2d_ragged(self):
        inner1 = np.array([[1.0, 2.0], [3.0, 4.0]])
        inner2 = np.array([[5.0]])
        arr = np.array([inner1, inner2], dtype=object)
        result = pad_ragged_tensor(arr)
        assert result.shape == (2, 2, 2)
        assert result[0, 0, 0] == 1.0
        assert result[1, 0, 0] == 5.0
