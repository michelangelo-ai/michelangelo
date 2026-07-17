"""Tests for ``michelangelo.lib._internal.utils.numpy_utils.pyarrow_conversion``."""

from __future__ import annotations

import logging

import numpy as np
import pyarrow as pa
import pytest

from michelangelo.lib._internal.utils.numpy_utils.pyarrow_conversion import (
    assemble_output_table,
    numpy_to_pyarrow,
    pyarrow_to_numpy,
)

# -----------------------------------------------------------------------------
# pyarrow_to_numpy
# -----------------------------------------------------------------------------


class TestPyarrowToNumpy:
    """Tests for ``pyarrow_to_numpy``."""

    def test_flat_primitive(self):
        """A flat primitive array converts to a 1D array."""
        arr = pa.array([1, 2, 3, 4, 5])
        result = pyarrow_to_numpy(arr)
        assert result.shape == (5,)
        np.testing.assert_array_equal(result, [1, 2, 3, 4, 5])

    def test_fixed_size_list(self):
        """A fixed-size list converts to a rectangular 2D array."""
        arr = pa.array([[1, 2], [3, 4]], type=pa.list_(pa.int64(), 2))
        result = pyarrow_to_numpy(arr)
        assert result.shape == (2, 2)
        np.testing.assert_array_equal(result, [[1, 2], [3, 4]])

    def test_uniform_variable_list(self):
        """A uniform variable-length list converts to a 2D array."""
        arr = pa.array([[1, 2], [3, 4], [5, 6]])
        result = pyarrow_to_numpy(arr)
        assert result.shape == (3, 2)
        np.testing.assert_array_equal(result, [[1, 2], [3, 4], [5, 6]])

    def test_uniform_nested_3d(self):
        """A uniform nested list converts to a 3D array."""
        arr = pa.array([[[1.0, 2.0], [3.0, 4.0]], [[5.0, 6.0], [7.0, 8.0]]])
        result = pyarrow_to_numpy(arr)
        assert result.shape == (2, 2, 2)
        assert result[1, 1, 1] == 8.0

    def test_ragged_falls_back_to_object(self):
        """Ragged lengths fall back to an object array of lists."""
        arr = pa.array([[1, 2, 3], [4]])
        result = pyarrow_to_numpy(arr)
        # Ragged lengths cannot be uniformly reshaped -> object array of lists.
        assert result.shape == (2,)
        assert list(result[0]) == [1, 2, 3]
        assert list(result[1]) == [4]

    def test_empty_inner_list_falls_back(self):
        """Empty inner lists fall back without error."""
        arr = pa.array([[], []], type=pa.list_(pa.int64()))
        result = pyarrow_to_numpy(arr)
        assert len(result) == 2

    def test_chunked_array_is_combined(self):
        """A chunked array is combined before conversion."""
        chunked = pa.chunked_array([[[1, 2]], [[3, 4]]])
        result = pyarrow_to_numpy(chunked)
        assert result.shape == (2, 2)
        np.testing.assert_array_equal(result, [[1, 2], [3, 4]])

    def test_large_list(self):
        """A large_list type converts like a regular list."""
        arr = pa.array([[1, 2], [3, 4]], type=pa.large_list(pa.int32()))
        result = pyarrow_to_numpy(arr)
        assert result.shape == (2, 2)

    def test_sliced_fixed_size_list_respects_offset(self):
        """A sliced fixed-size list honors the parent offset."""
        # Regression: a sliced bare fixed_size_list Array shares the full
        # underlying values buffer. Reading values without accounting for the
        # parent offset previously raised "cannot reshape array of size ...".
        arr = pa.array([[1, 2], [3, 4], [5, 6]], type=pa.list_(pa.int64(), 2))
        result = pyarrow_to_numpy(arr.slice(1))
        assert result.shape == (2, 2)
        np.testing.assert_array_equal(result, [[3, 4], [5, 6]])

    def test_sliced_fixed_size_list_length_bounded(self):
        """A length-bounded slice reads only its own rows."""
        arr = pa.array([[1, 2], [3, 4], [5, 6]], type=pa.list_(pa.int64(), 2))
        result = pyarrow_to_numpy(arr.slice(1, 1))
        assert result.shape == (1, 2)
        np.testing.assert_array_equal(result, [[3, 4]])

    def test_sliced_nested_fixed_size_list_respects_offset(self):
        """A sliced nested fixed-size list honors the parent offset."""
        arr = pa.array(
            [[[1, 2], [3, 4]], [[5, 6], [7, 8]], [[9, 10], [11, 12]]],
            type=pa.list_(pa.list_(pa.int64(), 2), 2),
        )
        result = pyarrow_to_numpy(arr.slice(1))
        assert result.shape == (2, 2, 2)
        np.testing.assert_array_equal(result, [[[5, 6], [7, 8]], [[9, 10], [11, 12]]])

    def test_null_row_in_fixed_size_list_not_preserved(self):
        """A null row in a nested list is not preserved (documented limitation)."""
        # Documented limitation: nulls in a nested list column are not
        # preserved. The null row materializes from its backing slots (NaN,
        # with the int leaf promoted to float) rather than staying null. Pins
        # the current behavior so any future change is deliberate.
        arr = pa.array([[1, 2], None, [5, 6]], type=pa.list_(pa.int64(), 2))
        result = pyarrow_to_numpy(arr)
        assert result.shape == (3, 2)
        np.testing.assert_array_equal(result[0], [1.0, 2.0])
        np.testing.assert_array_equal(result[2], [5.0, 6.0])
        assert np.isnan(result[1]).all()


# -----------------------------------------------------------------------------
# numpy_to_pyarrow
# -----------------------------------------------------------------------------


class TestNumpyToPyarrowArray:
    """Tests for ``numpy_to_pyarrow``."""

    def test_1d_flat(self):
        """A 1D array converts to a flat pyarrow array."""
        arr = np.array([1, 2, 3], dtype=np.int64)
        result = numpy_to_pyarrow(arr)
        assert result.to_pylist() == [1, 2, 3]

    def test_1d_target_type_prevents_promotion(self):
        """A target type prevents dtype promotion."""
        arr = np.array([1, 2, 3], dtype=np.int32)
        result = numpy_to_pyarrow(arr, target_type=pa.int32())
        assert result.type == pa.int32()

    def test_2d_single_column_uses_list(self):
        """A single-column 2D array uses a list type."""
        arr = np.array([[1], [2], [3]], dtype=np.int64)
        result = numpy_to_pyarrow(arr)
        assert pa.types.is_list(result.type)
        assert result.to_pylist() == [[1], [2], [3]]

    def test_2d_general_uses_fixed_size_list(self):
        """A general 2D array uses a fixed-size list type."""
        arr = np.array([[1, 2], [3, 4]], dtype=np.int64)
        result = numpy_to_pyarrow(arr)
        assert pa.types.is_fixed_size_list(result.type)
        assert result.to_pylist() == [[1, 2], [3, 4]]

    def test_3d_nested_fixed_size_list(self):
        """A 3D array uses nested fixed-size lists."""
        arr = np.arange(8, dtype=np.float32).reshape(2, 2, 2)
        result = numpy_to_pyarrow(arr)
        assert pa.types.is_fixed_size_list(result.type)
        assert result.to_pylist() == arr.tolist()

    def test_object_ragged_defers_to_tolist(self):
        """A ragged object array defers to ``tolist``."""
        arr = np.empty(2, dtype=object)
        arr[0] = [1, 2, 3]
        arr[1] = [4]
        result = numpy_to_pyarrow(arr)
        assert result.to_pylist() == [[1, 2, 3], [4]]

    def test_roundtrip_nd_shape(self):
        """An N-D array round-trips through pyarrow and back."""
        arr = np.arange(12, dtype=np.float64).reshape(3, 2, 2)
        table_col = numpy_to_pyarrow(arr)
        restored = pyarrow_to_numpy(table_col)
        assert restored.shape == (3, 2, 2)
        np.testing.assert_array_equal(restored, arr)

    def test_fallback_logs_warning_and_chains_on_failure(self, caplog):
        """A failed conversion logs a warning and chains the original cause."""
        # complex dtype is unsupported by both the direct path and the
        # tolist() fallback. The direct failure must be logged at warning (not
        # silently at debug) and chained as __cause__ so it is not lost.
        arr = np.arange(8, dtype=np.complex128).reshape(2, 2, 2)
        with (
            caplog.at_level(logging.WARNING),
            pytest.raises(Exception) as excinfo,
        ):
            numpy_to_pyarrow(arr)
        assert excinfo.value.__cause__ is not None
        assert any(
            "Direct PyArrow conversion failed" in rec.message
            and rec.levelno == logging.WARNING
            for rec in caplog.records
        )


# -----------------------------------------------------------------------------
# assemble_output_table
# -----------------------------------------------------------------------------


class TestAssembleOutputTable:
    """Tests for ``assemble_output_table``."""

    def _input_table(self) -> pa.Table:
        return pa.table({"id": [1, 2, 3], "feature": [10.0, 20.0, 30.0]})

    def test_passthrough_and_predictions(self):
        """Predictions are appended after passthrough columns."""
        table = self._input_table()
        preds = {"score": np.array([0.1, 0.2, 0.3], dtype=np.float64)}
        result = assemble_output_table(table, preds)
        assert result.column_names == ["id", "feature", "score"]
        assert result.column("score").to_pylist() == pytest.approx([0.1, 0.2, 0.3])
        assert result.column("id").to_pylist() == [1, 2, 3]

    def test_prediction_overwrites_input_by_default(self):
        """A prediction overwrites a colliding input column by default."""
        table = self._input_table()
        preds = {"feature": np.array([1.0, 2.0, 3.0], dtype=np.float64)}
        result = assemble_output_table(table, preds)
        assert result.column("feature").to_pylist() == pytest.approx([1.0, 2.0, 3.0])
        assert result.column_names == ["id", "feature"]

    def test_raise_on_collision(self):
        """``raise_on_collision`` rejects a colliding prediction."""
        table = self._input_table()
        preds = {"feature": np.array([1.0, 2.0, 3.0], dtype=np.float64)}
        with pytest.raises(ValueError, match="already exist"):
            assemble_output_table(table, preds, raise_on_collision=True)

    def test_columns_to_keep_subset(self):
        """``columns_to_keep`` selects a subset of columns."""
        table = self._input_table()
        preds = {"score": np.array([0.1, 0.2, 0.3], dtype=np.float64)}
        result = assemble_output_table(table, preds, columns_to_keep=["id", "score"])
        assert result.column_names == ["id", "score"]

    def test_columns_to_keep_none_keeps_all(self):
        """``columns_to_keep=None`` keeps every column."""
        table = self._input_table()
        preds = {"score": np.array([0.1, 0.2, 0.3], dtype=np.float64)}
        result = assemble_output_table(table, preds, columns_to_keep=None)
        assert result.column_names == ["id", "feature", "score"]

    def test_columns_to_keep_empty_keeps_all(self):
        """An empty ``columns_to_keep`` keeps every column, same as None."""
        # An empty collection is falsy and keeps every column, same as None.
        table = self._input_table()
        preds = {"score": np.array([0.1, 0.2, 0.3], dtype=np.float64)}
        result = assemble_output_table(table, preds, columns_to_keep=[])
        assert result.column_names == ["id", "feature", "score"]

    def test_columns_to_keep_filters_but_does_not_reorder(self):
        """``columns_to_keep`` filters without reordering."""
        # columns_to_keep selects a subset in the assembled order; it is a
        # filter, not a reorder. Requesting ["score", "id"] still yields the
        # assembled order ["id", "score"].
        table = self._input_table()
        preds = {"score": np.array([0.1, 0.2, 0.3], dtype=np.float64)}
        result = assemble_output_table(table, preds, columns_to_keep=["score", "id"])
        assert result.column_names == ["id", "score"]

    def test_extra_columns_appended(self):
        """Extra columns are appended after predictions."""
        table = self._input_table()
        preds = {"score": np.array([0.1, 0.2, 0.3], dtype=np.float64)}
        extra = {"tag": pa.array(["a", "b", "c"])}
        result = assemble_output_table(table, preds, extra_columns=extra)
        assert result.column_names == ["id", "feature", "score", "tag"]
        assert result.column("tag").to_pylist() == ["a", "b", "c"]

    def test_extra_column_collision_raises(self):
        """A colliding extra column is rejected under ``raise_on_collision``."""
        table = self._input_table()
        extra = {"id": pa.array([9, 9, 9])}
        with pytest.raises(ValueError, match="already exist"):
            assemble_output_table(
                table, {}, extra_columns=extra, raise_on_collision=True
            )

    def test_multi_dim_prediction_encoded(self):
        """A multi-dimensional prediction is encoded as a fixed-size list."""
        table = self._input_table()
        preds = {"embedding": np.arange(6, dtype=np.float32).reshape(3, 2)}
        result = assemble_output_table(table, preds)
        assert pa.types.is_fixed_size_list(result.schema.field("embedding").type)
        assert result.column("embedding").to_pylist() == [
            [0.0, 1.0],
            [2.0, 3.0],
            [4.0, 5.0],
        ]
