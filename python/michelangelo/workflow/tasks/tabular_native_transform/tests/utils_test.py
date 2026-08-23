"""Tests for michelangelo.workflow.tasks.tabular_native_transform._private.utils."""

from __future__ import annotations

import os
from unittest import TestCase
from unittest.mock import patch

import numpy as np

from michelangelo.lib.model_manager.schema import DataType, ModelSchemaItem
from michelangelo.workflow.tasks.tabular_native_transform._private.utils import (
    _DATA_ROOT_ENV_VAR,
    col_to_numpy,
    convert_to_numpy_sample,
    data_type_to_dtype,
    get_sample_data_from_datasets,
    resolve_data_file_path,
)


class _FakeValue:
    """Minimal stand-in for a Ray/pandas dataset's ``.take(n)`` interface."""

    def __init__(self, rows=None, raises: bool = False):
        """Store the rows to return from ``take()``, or a flag to raise instead."""
        self._rows = rows or []
        self._raises = raises

    def take(self, n):
        """Return up to ``n`` rows, or raise ``RuntimeError`` if configured to."""
        if self._raises:
            raise RuntimeError("boom")
        return self._rows[:n]


class _FakeDatasetVar:
    """Minimal stand-in for a ``DatasetVariable`` exposing only ``.value``."""

    def __init__(self, value):
        """Store the fake dataset value."""
        self.value = value


class ResolveDataFilePathTest(TestCase):
    """Tests for resolve_data_file_path."""

    def test_absolute_path_passthrough(self):
        """An absolute path is returned unchanged."""
        self.assertEqual(resolve_data_file_path("/abs/path.yaml"), "/abs/path.yaml")

    def test_relative_path_uses_env_var_when_set(self):
        """A relative path resolves against MICHELANGELO_DATA_ROOT when set."""
        with patch.dict(os.environ, {_DATA_ROOT_ENV_VAR: "/data/root"}):
            self.assertEqual(
                resolve_data_file_path("spec.yaml"), "/data/root/spec.yaml"
            )

    def test_relative_path_uses_cwd_when_env_var_unset(self):
        """A relative path falls back to the current working directory."""
        env = dict(os.environ)
        env.pop(_DATA_ROOT_ENV_VAR, None)
        with patch.dict(os.environ, env, clear=True):
            self.assertEqual(
                resolve_data_file_path("spec.yaml"),
                os.path.join(os.getcwd(), "spec.yaml"),
            )


class GetSampleDataFromDatasetsTest(TestCase):
    """Tests for get_sample_data_from_datasets."""

    def test_returns_none_for_empty_datasets(self):
        """An empty datasets dict returns None."""
        self.assertIsNone(get_sample_data_from_datasets({}))

    def test_prefers_train_over_other_datasets(self):
        """The train dataset is preferred over other dataset names."""
        datasets = {
            "test": _FakeDatasetVar(_FakeValue([{"x": 99}])),
            "train": _FakeDatasetVar(_FakeValue([{"x": 1}])),
        }
        self.assertEqual(get_sample_data_from_datasets(datasets), {"x": 1})

    def test_falls_back_to_any_available_dataset(self):
        """A non-preferred dataset name is used when no preferred name is present."""
        datasets = {"custom": _FakeDatasetVar(_FakeValue([{"x": 7}]))}
        self.assertEqual(get_sample_data_from_datasets(datasets), {"x": 7})

    def test_skips_dataset_with_none_value(self):
        """A dataset variable with value=None is skipped in favor of the next one."""
        datasets = {
            "train": _FakeDatasetVar(None),
            "test": _FakeDatasetVar(_FakeValue([{"x": 3}])),
        }
        self.assertEqual(get_sample_data_from_datasets(datasets), {"x": 3})

    def test_returns_none_when_take_raises_for_every_dataset(self):
        """Broad except Exception in the sampling loop (GAP-005 parity)."""
        datasets = {"train": _FakeDatasetVar(_FakeValue(raises=True))}
        self.assertIsNone(get_sample_data_from_datasets(datasets))

    def test_falls_through_after_one_dataset_raises(self):
        """Sampling continues to the next dataset after one raises."""
        datasets = {
            "train": _FakeDatasetVar(_FakeValue(raises=True)),
            "validation": _FakeDatasetVar(_FakeValue([{"x": 5}])),
        }
        self.assertEqual(get_sample_data_from_datasets(datasets), {"x": 5})

    def test_returns_none_when_take_returns_empty(self):
        """A dataset whose take() returns no rows yields no sample."""
        datasets = {"train": _FakeDatasetVar(_FakeValue([]))}
        self.assertIsNone(get_sample_data_from_datasets(datasets))


class DataTypeToDtypeTest(TestCase):
    """Tests for data_type_to_dtype."""

    def test_known_mappings(self):
        """Every declared DataType maps to its expected numpy dtype."""
        self.assertEqual(data_type_to_dtype(DataType.FLOAT), np.float32)
        self.assertEqual(data_type_to_dtype(DataType.DOUBLE), np.float64)
        self.assertEqual(data_type_to_dtype(DataType.INT), np.int32)
        self.assertEqual(data_type_to_dtype(DataType.LONG), np.int64)
        self.assertEqual(data_type_to_dtype(DataType.SHORT), np.int16)
        self.assertEqual(data_type_to_dtype(DataType.BYTE), np.int8)
        self.assertEqual(data_type_to_dtype(DataType.BOOLEAN), np.bool_)
        self.assertEqual(data_type_to_dtype(DataType.STRING), np.object_)

    def test_unmapped_type_falls_back_to_object(self):
        """An unmapped DataType falls back to np.object_."""
        self.assertEqual(data_type_to_dtype(DataType.UNKNOWN), np.object_)


class ColToNumpyTest(TestCase):
    """Tests for col_to_numpy."""

    def test_none_value_returns_empty_array(self):
        """A None value with a dtype returns an empty array of that dtype."""
        result = col_to_numpy(None, dtype=np.float32)
        self.assertEqual(result.shape, (0,))
        self.assertEqual(result.dtype, np.float32)

    def test_none_value_default_dtype(self):
        """A None value with no dtype defaults to np.object_."""
        result = col_to_numpy(None)
        self.assertEqual(result.dtype, np.object_)

    def test_ndarray_passthrough(self):
        """An ndarray with no dtype override is returned as-is."""
        arr = np.array([1, 2, 3])
        result = col_to_numpy(arr)
        np.testing.assert_array_equal(result, arr)

    def test_ndarray_with_dtype_cast(self):
        """An ndarray is cast to the requested dtype."""
        arr = np.array([1, 2, 3], dtype=np.int32)
        result = col_to_numpy(arr, dtype=np.float64)
        self.assertEqual(result.dtype, np.float64)

    def test_torch_tensor_like_object_uses_detach_path(self):
        """An object exposing detach()/cpu()/numpy() is converted via that path."""

        class _FakeTensor:
            def detach(self):
                return self

            def cpu(self):
                return self

            def numpy(self):
                return np.array([1.0, 2.0])

        result = col_to_numpy(_FakeTensor())
        np.testing.assert_array_equal(result, np.array([1.0, 2.0]))

    def test_object_with_broken_detach_falls_through(self):
        """Broad except in the detach path (GAP-005 parity)."""

        class _BrokenTensor:
            def detach(self):
                raise RuntimeError("no detach here")

        result = col_to_numpy(_BrokenTensor())
        self.assertEqual(result.dtype, np.object_)

    def test_bytes_value(self):
        """A bytes value is wrapped in a single-element array."""
        result = col_to_numpy(b"hello")
        self.assertEqual(result[0], b"hello")

    def test_str_value(self):
        """A str value is wrapped in a single-element array."""
        result = col_to_numpy("hello")
        self.assertEqual(result[0], "hello")

    def test_scalar_numeric_value(self):
        """A scalar numeric value is wrapped in a single-element array."""
        result = col_to_numpy(5)
        np.testing.assert_array_equal(result, np.array([5]))

    def test_scalar_with_dtype(self):
        """A scalar value is cast to the requested dtype."""
        result = col_to_numpy(5, dtype=np.float32)
        self.assertEqual(result.dtype, np.float32)

    def test_ragged_list_with_none_uses_default_fill(self):
        """A list containing None fills those entries with the dtype's zero value."""
        result = col_to_numpy([1.0, None, 3.0], dtype=np.float32)
        np.testing.assert_array_equal(
            result, np.array([1.0, 0.0, 3.0], dtype=np.float32)
        )

    def test_list_without_none_and_no_dtype_uses_asarray(self):
        """A plain list with no None entries and no dtype uses np.asarray."""
        result = col_to_numpy([1, 2, 3])
        np.testing.assert_array_equal(result, np.array([1, 2, 3]))


class ConvertToNumpySampleTest(TestCase):
    """Tests for convert_to_numpy_sample."""

    def test_none_row_returns_none(self):
        """A None row returns None."""
        self.assertIsNone(convert_to_numpy_sample(None))

    def test_no_schema_converts_every_key(self):
        """With no input_schema, every row key is converted."""
        result = convert_to_numpy_sample({"a": 1, "b": "x"})
        self.assertEqual(len(result), 1)
        np.testing.assert_array_equal(result[0]["a"], np.array([1]))
        self.assertEqual(result[0]["b"][0], "x")

    def test_with_schema_filters_and_orders_columns(self):
        """With an input_schema, only its columns are kept, in schema order."""
        schema = [
            ModelSchemaItem(name="a", data_type=DataType.FLOAT),
            ModelSchemaItem(name="missing", data_type=DataType.INT),
        ]
        result = convert_to_numpy_sample(
            {"a": 1.5, "extra": "dropped"}, input_schema=schema
        )
        self.assertEqual(set(result[0].keys()), {"a", "missing"})
        self.assertEqual(result[0]["missing"].shape, (0,))
        self.assertEqual(result[0]["missing"].dtype, np.int32)

    def test_with_schema_missing_key_in_row(self):
        """A schema column absent from the row becomes an empty typed array."""
        schema = [ModelSchemaItem(name="absent", data_type=DataType.STRING)]
        result = convert_to_numpy_sample({}, input_schema=schema)
        self.assertEqual(result[0]["absent"].shape, (0,))
