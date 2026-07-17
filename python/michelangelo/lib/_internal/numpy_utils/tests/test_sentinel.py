"""Tests for ``michelangelo.lib._internal.numpy_utils.sentinel``."""

from __future__ import annotations

import numpy as np
import pytest

from michelangelo.lib._internal.numpy_utils.sentinel import (
    BOOL_SENTINEL,
    BYTES_SENTINEL,
    FLOAT_SENTINEL,
    INT32_SENTINEL,
    STRING_SENTINEL,
    sentinel_for_numpy_dtype,
)


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
