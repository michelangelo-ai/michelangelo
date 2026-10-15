"""Tests for ``michelangelo.lib._internal.utils.numpy_utils.pad``."""

from __future__ import annotations

import numpy as np
import pytest

from michelangelo.lib._internal.utils.numpy_utils.pad import (
    pad_batch,
    pad_ragged_tensor,
)
from michelangelo.lib.constants.sentinel import INT32_SENTINEL


class TestPadRaggedTensor:
    """Tests for ``pad_ragged_tensor``."""

    def test_uniform_array_unchanged(self):
        """A uniform array is returned unchanged."""
        arr = np.array([1.0, 2.0, 3.0])
        result = pad_ragged_tensor(arr)
        np.testing.assert_array_equal(result, arr)

    def test_pads_1d_ragged_to_2d(self):
        """Ragged 1D rows are padded to a rectangular 2D array."""
        arr = np.array([np.array([1.0, 2.0]), np.array([3.0])], dtype=object)
        result = pad_ragged_tensor(arr)
        assert result.shape == (2, 2)
        assert result[0, 0] == 1.0
        assert result[0, 1] == 2.0
        assert result[1, 0] == 3.0
        assert np.isnan(result[1, 1])

    def test_custom_pad_value(self):
        """A caller-supplied pad value fills the padded slots."""
        arr = np.array(
            [np.array([1, 2], dtype=np.int32), np.array([3], dtype=np.int32)],
            dtype=object,
        )
        result = pad_ragged_tensor(arr, pad_value=-1)
        assert result[1, 1] == -1

    def test_int_array_uses_int32_sentinel(self):
        """Integer arrays pad with the int32 sentinel by default."""
        arr = np.array(
            [np.array([1], dtype=np.int32), np.array([2, 3], dtype=np.int32)],
            dtype=object,
        )
        result = pad_ragged_tensor(arr)
        assert result[0, 1] == INT32_SENTINEL

    def test_empty_object_array(self):
        """An empty object array yields an empty result."""
        arr = np.array([], dtype=object)
        result = pad_ragged_tensor(arr)
        assert len(result) == 0

    def test_nested_2d_ragged(self):
        """Ragged nested 2D rows are padded to a uniform 3D array."""
        inner1 = np.array([[1.0, 2.0], [3.0, 4.0]])
        inner2 = np.array([[5.0]])
        arr = np.array([inner1, inner2], dtype=object)
        result = pad_ragged_tensor(arr)
        assert result.shape == (2, 2, 2)
        assert result[0, 0, 0] == 1.0
        assert result[1, 0, 0] == 5.0

    def test_inhomogeneous_row_raises(self):
        """A row mixing a sequence and a scalar raises ``ValueError``."""
        # A row mixing a sequence with a scalar at the same level cannot be
        # padded to a uniform shape: np.stack fails and the np.array(dtype=...)
        # fallback also rejects the inhomogeneous input. Pins that this raises
        # rather than silently producing a malformed array.
        arr = np.array([np.array([1.0, 2.0]), np.float64(3.0)], dtype=object)
        with pytest.raises(ValueError):
            pad_ragged_tensor(arr)


class TestPadBatch:
    """Tests for ``pad_batch``."""

    def test_pads_only_ragged_columns(self):
        """Ragged columns are padded; uniform ones are returned as they are."""
        uniform = np.array([1.0, 2.0])
        batch = {
            "ragged": np.array([[1, 2], [3]], dtype=object),
            "uniform": uniform,
        }
        result = pad_batch(batch, {"ragged": -1})
        assert result["ragged"].tolist() == [[1, 2], [3, -1]]
        assert result["uniform"] is uniform
        assert list(result) == ["ragged", "uniform"]

    def test_default_pad_is_dtype_sentinel_not_zero(self):
        """Unlisted ragged columns use the sentinel for their dtype."""
        batch = {
            "ids": np.array(
                [np.array([1, 2], dtype=np.int64), np.array([3])], dtype=object
            )
        }
        result = pad_batch(batch)
        assert result["ids"][1, 1] == INT32_SENTINEL

    def test_object_array_of_scalars_passes_through(self):
        """An object column of non-lists is not touched."""
        arr = np.array(["a", None], dtype=object)
        assert pad_batch({"s": arr})["s"] is arr

    def test_empty_object_array_passes_through(self):
        """A zero-row object column is not touched."""
        arr = np.array([], dtype=object)
        assert pad_batch({"s": arr})["s"] is arr

    def test_unpaddable_dtype_error_names_the_column(self):
        """int8 has no default sentinel, so the error says which column to fix."""
        arr = np.array(
            [np.array([1, 2], dtype=np.int8), np.array([3], dtype=np.int8)],
            dtype=object,
        )
        with pytest.raises(ValueError, match=r"column_padding_values\['small'\]"):
            pad_batch({"small": arr})
        assert pad_batch({"small": arr}, {"small": 0})["small"].shape == (2, 2)
