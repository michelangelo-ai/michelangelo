"""Tests for ``michelangelo.lib.trainer.torch._numpy_utils``."""

from __future__ import annotations

import numpy as np
import pytest

from michelangelo.lib.trainer.torch._numpy_utils import (
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
    """Mapping from numpy dtype to its type-native pad sentinel."""

    @pytest.mark.parametrize(
        "dtype",
        [np.float16, np.float32, np.float64],
    )
    def test_floating_dtypes_return_nan(self, dtype):
        """All floating dtypes share the ``NaN`` sentinel."""
        sentinel = sentinel_for_numpy_dtype(np.dtype(dtype))
        assert np.isnan(sentinel)
        assert sentinel == FLOAT_SENTINEL or np.isnan(FLOAT_SENTINEL)

    @pytest.mark.parametrize("dtype", [np.int32, np.int64])
    def test_supported_integer_dtypes(self, dtype):
        """``int32`` / ``int64`` use ``INT32_SENTINEL`` (the int32 min)."""
        assert sentinel_for_numpy_dtype(np.dtype(dtype)) == INT32_SENTINEL

    @pytest.mark.parametrize("dtype", [np.int8, np.int16, np.uint8, np.uint16])
    def test_unsupported_integer_dtypes_raise(self, dtype):
        """Narrower integer dtypes have no defined sentinel."""
        with pytest.raises(ValueError, match="No sentinel defined"):
            sentinel_for_numpy_dtype(np.dtype(dtype))

    def test_unicode_string_dtype(self):
        """Unicode strings (``kind='U'``) pad with empty string."""
        assert sentinel_for_numpy_dtype(np.dtype("U5")) == STRING_SENTINEL

    def test_object_dtype(self):
        """Object dtype (``kind='O'``) pads with empty string."""
        assert sentinel_for_numpy_dtype(np.dtype("O")) == STRING_SENTINEL

    def test_byte_string_dtype(self):
        """Byte strings (``kind='S'``) pad with empty bytes."""
        assert sentinel_for_numpy_dtype(np.dtype("S5")) == BYTES_SENTINEL

    def test_boolean_dtype(self):
        """Bool pads with ``False``."""
        assert sentinel_for_numpy_dtype(np.dtype(bool)) == BOOL_SENTINEL

    def test_unsupported_complex_dtype_raises(self):
        """Complex dtype has no defined sentinel."""
        with pytest.raises(ValueError, match="No sentinel defined"):
            sentinel_for_numpy_dtype(np.dtype(np.complex64))


# -----------------------------------------------------------------------------
# infer_dtype
# -----------------------------------------------------------------------------


class TestInferDtype:
    """Recursive leaf-dtype inference for nested object arrays."""

    def test_scalar_returns_array_dtype(self):
        """A bare Python int round-trips through ``np.array(...).dtype``."""
        assert infer_dtype(5) == np.array(5).dtype

    def test_typed_ndarray_returns_its_dtype(self):
        """A non-object ndarray returns its own dtype directly."""
        assert infer_dtype(np.array([1, 2, 3], dtype=np.int64)) == np.int64

    def test_object_array_recurses_into_first_leaf(self):
        """An object array yields the inner concrete dtype."""
        inner = np.array([1.0, 2.0], dtype=np.float32)
        outer = np.empty(2, dtype=object)
        outer[0] = inner
        outer[1] = inner
        assert infer_dtype(outer) == np.float32

    def test_list_of_lists_recurses(self):
        """Nested lists return the inferred leaf dtype."""
        assert infer_dtype([[1, 2], [3, 4]]) == np.int64

    def test_empty_returns_none(self):
        """A fully empty nested list yields ``None`` (no dtype to infer)."""
        assert infer_dtype([]) is None

    def test_numpy_scalar_returns_its_dtype(self):
        """A numpy scalar carries its dtype."""
        x = np.float32(1.0)
        assert infer_dtype(x) == np.float32


# -----------------------------------------------------------------------------
# pad_ragged_tensor
# -----------------------------------------------------------------------------


class TestPadRaggedTensor:
    """End-to-end padding of object arrays of variable-length 1-D arrays."""

    def test_non_object_array_returned_as_is(self):
        """Already-rectangular ndarrays are not touched."""
        a = np.array([[1, 2], [3, 4]], dtype=np.int64)
        out = pad_ragged_tensor(a)
        np.testing.assert_array_equal(out, a)

    def test_ragged_1d_arrays_padded_with_int_sentinel(self):
        """Object array of 1-D int arrays → rectangular with INT32_SENTINEL pad."""
        rows = [
            np.array([1, 2, 3], dtype=np.int64),
            np.array([4], dtype=np.int64),
            np.array([5, 6], dtype=np.int64),
        ]
        arr = np.empty(len(rows), dtype=object)
        for i, r in enumerate(rows):
            arr[i] = r

        out = pad_ragged_tensor(arr)
        assert out.shape == (3, 3)
        np.testing.assert_array_equal(out[0], [1, 2, 3])
        assert out[1, 0] == 4
        assert out[1, 1] == INT32_SENTINEL
        assert out[1, 2] == INT32_SENTINEL
        assert out[2, 0] == 5
        assert out[2, 1] == 6
        assert out[2, 2] == INT32_SENTINEL

    def test_ragged_1d_arrays_padded_with_explicit_value(self):
        """User-supplied pad value is honored."""
        rows = [
            np.array([1, 2], dtype=np.int64),
            np.array([3], dtype=np.int64),
        ]
        arr = np.empty(len(rows), dtype=object)
        for i, r in enumerate(rows):
            arr[i] = r
        out = pad_ragged_tensor(arr, pad_value=-1)
        assert out[1, 1] == -1

    def test_ragged_2d_lists_padded(self):
        """Object array of 2-D rectangular lists pads each dim."""
        arr = np.empty(2, dtype=object)
        arr[0] = [[1, 2], [3, 4]]
        arr[1] = [[5]]
        out = pad_ragged_tensor(arr, pad_value=0)
        assert out.shape == (2, 2, 2)
        assert out[0, 0, 0] == 1 and out[0, 1, 1] == 4
        assert out[1, 0, 0] == 5
        assert out[1, 0, 1] == 0  # padded
        assert out[1, 1, 0] == 0  # padded row

    def test_floating_ragged_1d_uses_nan_sentinel(self):
        """Floats pad with NaN by default."""
        rows = [
            np.array([1.0, 2.0], dtype=np.float32),
            np.array([3.0], dtype=np.float32),
        ]
        arr = np.empty(len(rows), dtype=object)
        for i, r in enumerate(rows):
            arr[i] = r
        out = pad_ragged_tensor(arr)
        assert out.shape == (2, 2)
        assert out.dtype == np.float32
        assert np.isnan(out[1, 1])
