"""Tests for ``michelangelo.lib.constants.sentinel``."""

from __future__ import annotations

import numpy as np
import pytest

from michelangelo.lib.constants.sentinel import (
    BOOL_SENTINEL,
    BYTES_SENTINEL,
    FLOAT_SENTINEL,
    INT32_SENTINEL,
    STRING_SENTINEL,
    sentinel_for_numpy_dtype,
)


class TestSentinelForNumpyDtype:
    """Tests for ``sentinel_for_numpy_dtype`` and the sentinel constants."""

    def test_float32(self):
        """The float32 dtype maps to a NaN sentinel."""
        assert np.isnan(sentinel_for_numpy_dtype(np.dtype(np.float32)))

    def test_float64(self):
        """The float64 dtype maps to a NaN sentinel."""
        assert np.isnan(sentinel_for_numpy_dtype(np.dtype(np.float64)))

    def test_int32(self):
        """The int32 dtype maps to the int32 sentinel."""
        assert sentinel_for_numpy_dtype(np.dtype(np.int32)) == INT32_SENTINEL

    def test_int64(self):
        """The int64 dtype maps to the int32 sentinel."""
        assert sentinel_for_numpy_dtype(np.dtype(np.int64)) == INT32_SENTINEL

    def test_int8_raises(self):
        """An unsupported integer width raises ``ValueError``."""
        with pytest.raises(ValueError):
            sentinel_for_numpy_dtype(np.dtype(np.int8))

    def test_unicode(self):
        """Unicode strings map to the string sentinel."""
        assert sentinel_for_numpy_dtype(np.dtype("U10")) == STRING_SENTINEL

    def test_object(self):
        """The object dtype maps to the string sentinel."""
        assert sentinel_for_numpy_dtype(np.dtype(object)) == STRING_SENTINEL

    def test_bytes(self):
        """Byte strings map to the bytes sentinel."""
        assert sentinel_for_numpy_dtype(np.dtype("S10")) == BYTES_SENTINEL

    def test_bool(self):
        """The bool dtype maps to the bool sentinel."""
        assert sentinel_for_numpy_dtype(np.dtype(bool)) == BOOL_SENTINEL

    def test_float_sentinel_is_nan(self):
        """The float sentinel is NaN."""
        assert np.isnan(FLOAT_SENTINEL)

    def test_int32_sentinel_value(self):
        """The int32 sentinel is the minimum int32 value."""
        assert INT32_SENTINEL == -(2**31)
