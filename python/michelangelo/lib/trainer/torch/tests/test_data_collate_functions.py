"""Tests for ``michelangelo.lib.trainer.torch.data_collate_functions``.

Covers the structural checks (``cell_is_nested_subsequence`` /
``row_is_list_of_nested_cells``), the rectangularization helper
(``pad_ragged_lists``) for 1-D / 2-D / 3-D inputs, the per-column collate
functions (``collate_value_to_float32_numpy`` / ``collate_value_to_float32_tensor``),
and the batch / ``LiteralEvalFloat32Collate`` paths.
"""

from __future__ import annotations

import numpy as np
import pytest
import torch

from michelangelo.lib.constants.sentinel import (
    FLOAT_SENTINEL,
    INT32_SENTINEL,
)
from michelangelo.lib.trainer.torch.data_collate_functions import (
    DEFAULT_COLLATE_NUMPY_DTYPE,
    LiteralEvalFloat32Collate,
    cell_is_nested_subsequence,
    collate_batch_to_float32_tensors,
    collate_value_to_float32_numpy,
    collate_value_to_float32_tensor,
    literal_eval_data_collate_function,
    pad_ragged_lists,
    row_is_list_of_nested_cells,
)

# -----------------------------------------------------------------------------
# Structure checks
# -----------------------------------------------------------------------------


class TestCellIsNestedSubsequence:
    """``cell_is_nested_subsequence`` distinguishes scalars from vectors."""

    @pytest.mark.parametrize("cell", [[1, 2], (1, 2), np.array([1, 2])])
    def test_vectors_are_nested(self, cell):
        """Lists, tuples, and 1-D ndarrays are nested cells."""
        assert cell_is_nested_subsequence(cell) is True

    @pytest.mark.parametrize("cell", [1, 1.5, "x", np.array(7), np.int64(3)])
    def test_scalars_are_leaves(self, cell):
        """Python scalars and 0-D ndarrays are leaves."""
        assert cell_is_nested_subsequence(cell) is False


class TestRowIsListOfNestedCells:
    """``row_is_list_of_nested_cells`` picks the 3-D normalization branch."""

    def test_empty_row_returns_false(self):
        """An empty row has no nested cells."""
        assert row_is_list_of_nested_cells([]) is False

    def test_all_scalars_returns_false(self):
        """A row of scalars is the 2-D path."""
        assert row_is_list_of_nested_cells([1, 2, 3]) is False

    def test_all_lists_returns_true(self):
        """A row of lists is the 3-D path."""
        assert row_is_list_of_nested_cells([[1, 2], [3, 4]]) is True

    def test_mixed_scalar_and_list_returns_true(self):
        """A leading scalar with later list cells still selects the 3-D branch."""
        assert row_is_list_of_nested_cells([1, [2, 3]]) is True


# -----------------------------------------------------------------------------
# pad_ragged_lists
# -----------------------------------------------------------------------------


class TestPadRaggedLists:
    """``pad_ragged_lists`` rectangularizes nested Python lists."""

    def test_empty_input_returns_empty_array(self):
        """Empty input yields an empty array of the target dtype."""
        out = pad_ragged_lists([])
        assert out.shape == (0,)
        assert out.dtype == DEFAULT_COLLATE_NUMPY_DTYPE

    def test_scalar_per_row_returns_1d_array(self):
        """A list of scalars becomes a 1-D float32 array."""
        out = pad_ragged_lists([1, 2, 3])
        assert out.dtype == np.float32
        np.testing.assert_array_equal(out, [1.0, 2.0, 3.0])

    def test_rectangular_2d_lists_stay_rectangular(self):
        """A rectangular list-of-lists yields a 2-D array without padding."""
        out = pad_ragged_lists([[1, 2, 3], [4, 5, 6]])
        assert out.shape == (2, 3)
        np.testing.assert_array_equal(out, [[1, 2, 3], [4, 5, 6]])

    def test_ragged_2d_pads_to_max_length(self):
        """Variable-length rows are padded with the float sentinel (NaN)."""
        out = pad_ragged_lists([[1, 2, 3], [4]])
        assert out.shape == (2, 3)
        assert out[0, 2] == 3.0
        assert np.isnan(out[1, 1]) and np.isnan(out[1, 2])

    def test_ragged_2d_with_explicit_pad_value(self):
        """User-supplied pad_value overrides the dtype sentinel."""
        out = pad_ragged_lists([[1, 2, 3], [4]], pad_value=0.0)
        assert out[1, 1] == 0.0 and out[1, 2] == 0.0

    def test_3d_ragged_normalizes_inner_cells(self):
        """A ragged list-of-list-of-list pads to a dense 3-D tensor."""
        # Row 0 cells: [1,2], [3,4,5]; row 1 cells: [6], [7]. Max inner len = 3.
        out = pad_ragged_lists([[[1, 2], [3, 4, 5]], [[6], [7]]])
        assert out.shape == (2, 2, 3)
        # Inner cell padding: shorter inner sequences pad with NaN.
        np.testing.assert_array_equal(out[0, 1], [3, 4, 5])
        assert out[0, 0, 0] == 1 and out[0, 0, 1] == 2
        assert np.isnan(out[0, 0, 2])  # padded slot
        assert out[1, 0, 0] == 6 and out[1, 1, 0] == 7
        assert np.isnan(out[1, 0, 1]) and np.isnan(out[1, 1, 2])

    def test_int_dtype_pads_with_int_sentinel(self):
        """Integer numpy_dtype pads ragged rows with the int sentinel."""
        out = pad_ragged_lists([[1, 2, 3], [4]], numpy_dtype=np.int64)
        assert out.dtype == np.int64
        assert out[1, 1] == INT32_SENTINEL


# -----------------------------------------------------------------------------
# collate_value_to_float32_numpy
# -----------------------------------------------------------------------------


class TestCollateValueToFloat32Numpy:
    """``collate_value_to_float32_numpy`` per-column conversion."""

    def test_numeric_array_reshaped_to_column(self):
        """1-D numeric input is reshaped to ``(N, 1)`` by default."""
        out = collate_value_to_float32_numpy(np.array([1, 2, 3]))
        assert out.shape == (3, 1)
        assert out.dtype == np.float32

    def test_disable_reshape(self):
        """``reshape_1d_features=False`` returns the bare 1-D array."""
        out = collate_value_to_float32_numpy(
            np.array([1, 2, 3]), reshape_1d_features=False
        )
        assert out.shape == (3,)

    def test_literal_eval_string_input(self):
        """A stringified Python list is parsed via :func:`ast.literal_eval`."""
        out = collate_value_to_float32_numpy("[1, 2, 3]", reshape_1d_features=False)
        np.testing.assert_array_equal(out, [1.0, 2.0, 3.0])

    def test_disable_literal_eval_treats_string_as_unparsed(self):
        """With ``parse_string_with_literal_eval=False`` strings stay strings.

        The string is then wrapped via ``np.array(...)`` and the function falls
        through to the numeric path — non-numeric strings will fail, so we use
        a string that ``np.array(...).astype(float32)`` can coerce.
        """
        out = collate_value_to_float32_numpy(
            "1.5",
            reshape_1d_features=False,
            parse_string_with_literal_eval=False,
        )
        # ``np.array("1.5").astype(np.float32)`` → 1.5 as a 0-D float32 array.
        assert out.dtype == np.float32
        assert float(out) == 1.5

    def test_object_array_of_stringified_lists(self):
        """An object array of ``"[1,2]"`` strings is parsed then padded."""
        cells = np.array(["[1,2,3]", "[4]"], dtype=object)
        out = collate_value_to_float32_numpy(cells, reshape_1d_features=False)
        assert out.shape == (2, 3)
        assert out[0, 2] == 3.0
        assert np.isnan(out[1, 1])


# -----------------------------------------------------------------------------
# collate_value_to_float32_tensor
# -----------------------------------------------------------------------------


class TestCollateValueToFloat32Tensor:
    """``collate_value_to_float32_tensor`` wraps the numpy helper."""

    def test_returns_float32_tensor(self):
        """Numeric input becomes a float32 tensor."""
        out = collate_value_to_float32_tensor(np.array([1, 2, 3]))
        assert isinstance(out, torch.Tensor)
        assert out.dtype == torch.float32
        assert out.shape == (3, 1)

    def test_int_dtype_yields_int_tensor(self):
        """Specifying ``numpy_dtype=int64`` yields a torch.int64 tensor."""
        out = collate_value_to_float32_tensor(np.array([1, 2, 3]), numpy_dtype=np.int64)
        assert out.dtype == torch.int64


# -----------------------------------------------------------------------------
# collate_batch_to_float32_tensors
# -----------------------------------------------------------------------------


class TestCollateBatchToFloat32Tensors:
    """Full batch-dict path."""

    def test_simple_batch_round_trip(self):
        """Each column becomes a float32 tensor; dict structure is preserved."""
        batch = {
            "user_idx": np.array([1, 2, 3]),
            "rating": np.array([0.5, 0.7, 0.9]),
        }
        out = collate_batch_to_float32_tensors(batch)
        assert set(out.keys()) == {"user_idx", "rating"}
        assert out["user_idx"].dtype == torch.float32
        assert out["user_idx"].shape == (3, 1)


# -----------------------------------------------------------------------------
# LiteralEvalFloat32Collate
# -----------------------------------------------------------------------------


class TestLiteralEvalFloat32Collate:
    """OO wrapper around the same collate behavior."""

    def test_defaults_match_function_path(self):
        """Default-constructed instance reproduces the functional path."""
        c = LiteralEvalFloat32Collate()
        batch = {"x": np.array([1, 2, 3])}
        out_obj = c(batch)
        out_fn = collate_batch_to_float32_tensors(batch)
        for k in batch:
            assert torch.equal(out_obj[k], out_fn[k])

    def test_subclass_can_override_per_value(self):
        """Subclasses can override :meth:`collate_value_to_tensor`."""

        class HalfCollate(LiteralEvalFloat32Collate):
            def collate_value_to_tensor(self, value):
                return torch.tensor(value, dtype=torch.float32) / 2.0

        c = HalfCollate()
        out = c({"x": np.array([2.0, 4.0, 6.0])})
        np.testing.assert_array_equal(out["x"].numpy(), [1.0, 2.0, 3.0])

    def test_custom_numpy_dtype(self):
        """Constructor kwarg ``numpy_dtype`` is honored end-to-end."""
        c = LiteralEvalFloat32Collate(numpy_dtype=np.int64)
        out = c({"x": np.array([1, 2, 3])})
        assert out["x"].dtype == torch.int64


# -----------------------------------------------------------------------------
# literal_eval_data_collate_function
# -----------------------------------------------------------------------------


class TestLiteralEvalDataCollateFunction:
    """Module-level default collate."""

    def test_round_trip(self):
        """Default-collate the same shape data goes through with float32 tensors."""
        batch = {"x": np.array([1.0, 2.0, 3.0])}
        out = literal_eval_data_collate_function(batch)
        assert out["x"].dtype == torch.float32
        assert out["x"].shape == (3, 1)


# -----------------------------------------------------------------------------
# Sentinel-injection coverage
# -----------------------------------------------------------------------------


class TestSentinelInjection:
    """End-to-end float/int sentinel injection on ragged padding."""

    def test_float_padding_uses_nan_sentinel(self):
        """Ragged float rows pad with ``FLOAT_SENTINEL`` (NaN)."""
        out = pad_ragged_lists([[1.0, 2.0], [3.0]], numpy_dtype=np.float32)
        # The default sentinel is the float NaN.
        assert np.isnan(FLOAT_SENTINEL)
        assert np.isnan(out[1, 1])

    def test_int_padding_uses_int_sentinel(self):
        """Ragged int rows pad with ``INT32_SENTINEL``."""
        out = pad_ragged_lists([[1, 2], [3]], numpy_dtype=np.int64)
        assert out[1, 1] == INT32_SENTINEL
