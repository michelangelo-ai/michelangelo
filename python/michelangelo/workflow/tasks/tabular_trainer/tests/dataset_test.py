"""Tests for michelangelo.workflow.tasks.tabular_trainer._dataset."""

from __future__ import annotations

import warnings
from unittest import TestCase
from unittest.mock import patch

import numpy as np

from michelangelo.lib.model_manager.schema.data_type import DataType
from michelangelo.lib.model_manager.schema.model_schema import ModelSchema
from michelangelo.workflow.schema.tabular_trainer import (
    ColumnConfig,
    LightningTrainerConfig,
)
from michelangelo.workflow.tasks.tabular_trainer._dataset import (
    _map_torch_dtype_to_datatype,
    _map_torch_dtype_to_numpy,
    _pad_row,
    collate_sample_row,
    construct_read_kwargs,
    get_model_schema,
    get_sample_data,
    raise_lightning_trainer_config_deprecation_warnings,
)

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

_BASE_LIGHTNING_CFG = {
    "model_class": "m",
    "input_columns": {"x": ColumnConfig("torch.float32")},
    "output_columns": {"y": ColumnConfig("torch.float32")},
    "labels": {"label": ColumnConfig("torch.long")},
    "metadata_columns": [],
}


def _lightning_cfg(**overrides) -> LightningTrainerConfig:
    """Build a minimal valid LightningTrainerConfig with optional overrides."""
    return LightningTrainerConfig(**{**_BASE_LIGHTNING_CFG, **overrides})


# ---------------------------------------------------------------------------
# _map_torch_dtype_to_numpy
# ---------------------------------------------------------------------------


class TestMapTorchDtypeToNumpy(TestCase):
    """Tests for _map_torch_dtype_to_numpy."""

    def test_float32(self):
        """Maps torch.float32 to np.float32."""
        self.assertIs(_map_torch_dtype_to_numpy("torch.float32"), np.float32)

    def test_float32_alias(self):
        """Maps torch.float alias to np.float32."""
        self.assertIs(_map_torch_dtype_to_numpy("torch.float"), np.float32)

    def test_float64(self):
        """Maps torch.float64 to np.float64."""
        self.assertIs(_map_torch_dtype_to_numpy("torch.float64"), np.float64)

    def test_int32(self):
        """Maps torch.int32 to np.int32."""
        self.assertIs(_map_torch_dtype_to_numpy("torch.int32"), np.int32)

    def test_int32_alias(self):
        """Maps torch.int alias to np.int32."""
        self.assertIs(_map_torch_dtype_to_numpy("torch.int"), np.int32)

    def test_int64(self):
        """Maps torch.int64 to np.int64."""
        self.assertIs(_map_torch_dtype_to_numpy("torch.int64"), np.int64)

    def test_long(self):
        """Maps torch.long to np.int64."""
        self.assertIs(_map_torch_dtype_to_numpy("torch.long"), np.int64)

    def test_int16(self):
        """Maps torch.int16 to np.int16."""
        self.assertIs(_map_torch_dtype_to_numpy("torch.int16"), np.int16)

    def test_short(self):
        """Maps torch.short to np.int16."""
        self.assertIs(_map_torch_dtype_to_numpy("torch.short"), np.int16)

    def test_int8(self):
        """Maps torch.int8 to np.int8."""
        self.assertIs(_map_torch_dtype_to_numpy("torch.int8"), np.int8)

    def test_uint8(self):
        """Maps torch.uint8 to np.uint8."""
        self.assertIs(_map_torch_dtype_to_numpy("torch.uint8"), np.uint8)

    def test_bool(self):
        """Maps torch.bool to np.bool_."""
        self.assertIs(_map_torch_dtype_to_numpy("torch.bool"), np.bool_)

    def test_unknown_falls_back_to_float32(self):
        """Falls back to np.float32 for unknown dtype strings."""
        self.assertIs(_map_torch_dtype_to_numpy("torch.bfloat16"), np.float32)


# ---------------------------------------------------------------------------
# _map_torch_dtype_to_datatype
# ---------------------------------------------------------------------------


class TestMapTorchDtypeToDatatype(TestCase):
    """Tests for _map_torch_dtype_to_datatype."""

    def test_float32(self):
        """Maps torch.float32 to DataType.FLOAT."""
        self.assertEqual(_map_torch_dtype_to_datatype("torch.float32"), DataType.FLOAT)

    def test_float32_alias(self):
        """Maps torch.float alias to DataType.FLOAT."""
        self.assertEqual(_map_torch_dtype_to_datatype("torch.float"), DataType.FLOAT)

    def test_float64(self):
        """Maps torch.float64 to DataType.DOUBLE."""
        self.assertEqual(_map_torch_dtype_to_datatype("torch.float64"), DataType.DOUBLE)

    def test_int32(self):
        """Maps torch.int32 to DataType.INT."""
        self.assertEqual(_map_torch_dtype_to_datatype("torch.int32"), DataType.INT)

    def test_int32_alias(self):
        """Maps torch.int alias to DataType.INT."""
        self.assertEqual(_map_torch_dtype_to_datatype("torch.int"), DataType.INT)

    def test_int64(self):
        """Maps torch.int64 to DataType.LONG."""
        self.assertEqual(_map_torch_dtype_to_datatype("torch.int64"), DataType.LONG)

    def test_long(self):
        """Maps torch.long to DataType.LONG."""
        self.assertEqual(_map_torch_dtype_to_datatype("torch.long"), DataType.LONG)

    def test_int16(self):
        """Maps torch.int16 to DataType.SHORT."""
        self.assertEqual(_map_torch_dtype_to_datatype("torch.int16"), DataType.SHORT)

    def test_short(self):
        """Maps torch.short to DataType.SHORT."""
        self.assertEqual(_map_torch_dtype_to_datatype("torch.short"), DataType.SHORT)

    def test_int8(self):
        """Maps torch.int8 to DataType.BYTE."""
        self.assertEqual(_map_torch_dtype_to_datatype("torch.int8"), DataType.BYTE)

    def test_uint8(self):
        """Maps torch.uint8 to DataType.BYTE."""
        self.assertEqual(_map_torch_dtype_to_datatype("torch.uint8"), DataType.BYTE)

    def test_bool(self):
        """Maps torch.bool to DataType.BOOLEAN."""
        self.assertEqual(_map_torch_dtype_to_datatype("torch.bool"), DataType.BOOLEAN)

    def test_unknown_falls_back_to_float(self):
        """Falls back to DataType.FLOAT for unknown dtype strings."""
        self.assertEqual(_map_torch_dtype_to_datatype("torch.bfloat16"), DataType.FLOAT)


# ---------------------------------------------------------------------------
# get_model_schema
# ---------------------------------------------------------------------------


class TestGetModelSchema(TestCase):
    """Tests for get_model_schema."""

    def test_returns_model_schema(self):
        """It returns a ModelSchema instance."""
        schema = get_model_schema(
            input_columns={"x": ColumnConfig("torch.float32")},
            output_columns={"y": ColumnConfig("torch.float32")},
        )
        self.assertIsInstance(schema, ModelSchema)

    def test_input_schema_names(self):
        """Input schema items carry the correct feature names."""
        schema = get_model_schema(
            input_columns={
                "age": ColumnConfig("torch.float32"),
                "income": ColumnConfig("torch.float32"),
            },
            output_columns={},
        )
        names = [item.name for item in schema.input_schema]
        self.assertIn("age", names)
        self.assertIn("income", names)

    def test_output_schema_names(self):
        """Output schema items carry the correct names."""
        schema = get_model_schema(
            input_columns={},
            output_columns={"score": ColumnConfig("torch.float32")},
        )
        self.assertEqual(schema.output_schema[0].name, "score")

    def test_input_dtype_mapping(self):
        """DataType is correctly derived from the ColumnConfig data_type."""
        schema = get_model_schema(
            input_columns={"emb": ColumnConfig("torch.float64", [128])},
            output_columns={},
        )
        self.assertEqual(schema.input_schema[0].data_type, DataType.DOUBLE)

    def test_input_shape_propagated(self):
        """Shape is propagated to ModelSchemaItem."""
        schema = get_model_schema(
            input_columns={"emb": ColumnConfig("torch.float32", [64])},
            output_columns={},
        )
        self.assertEqual(schema.input_schema[0].shape, [64])

    def test_empty_configs(self):
        """Empty dicts produce empty schemas."""
        schema = get_model_schema(input_columns={}, output_columns={})
        self.assertEqual(schema.input_schema, [])
        self.assertEqual(schema.output_schema, [])

    def test_feature_store_schema_empty(self):
        """feature_store_features_schema is always empty (not set here)."""
        schema = get_model_schema(
            input_columns={"x": ColumnConfig("torch.float32")},
            output_columns={"y": ColumnConfig("torch.float32")},
        )
        self.assertEqual(schema.feature_store_features_schema, [])


# ---------------------------------------------------------------------------
# _pad_row
# ---------------------------------------------------------------------------


class TestPadRow(TestCase):
    """Tests for _pad_row -- the no-collate-fn normalisation path."""

    def test_plain_ndarray_unchanged(self):
        """A non-object numpy array is returned as-is."""
        arr = np.array([1.0, 2.0], dtype=np.float32)
        result = _pad_row({"x": arr})
        np.testing.assert_array_equal(result["x"], arr)

    def test_scalar_wrapped(self):
        """A scalar is wrapped in a 1-D array."""
        result = _pad_row({"x": 3.0})
        self.assertIsInstance(result["x"], np.ndarray)
        self.assertAlmostEqual(result["x"].item(), 3.0)

    def test_list_padded(self):
        """A list value is padded to a dense numpy array."""
        result = _pad_row({"x": [1.0, 2.0, 3.0]})
        self.assertIsInstance(result["x"], np.ndarray)
        self.assertEqual(result["x"].shape[0], 3)

    def test_string_literal_eval(self):
        """A stringified list is parsed via literal_eval."""
        result = _pad_row({"x": "[1.0, 2.0]"})
        self.assertIsInstance(result["x"], np.ndarray)
        self.assertEqual(result["x"].shape[0], 2)

    def test_string_unparseable_kept_as_object(self):
        """An unparseable string is stored in an object array."""
        result = _pad_row({"x": "not_a_list"})
        self.assertEqual(result["x"].dtype, object)

    def test_object_array_with_string_cells(self):
        """Object array whose cells are stringified arrays is parsed and padded."""
        obj = np.array(["[1.0, 2.0]", "[3.0, 4.0]"], dtype=object)
        result = _pad_row({"x": obj})
        self.assertIsInstance(result["x"], np.ndarray)
        self.assertNotEqual(result["x"].dtype, object)

    def test_multiple_keys(self):
        """All keys in the row are processed."""
        result = _pad_row({"a": 1.0, "b": np.array([2.0])})
        self.assertIn("a", result)
        self.assertIn("b", result)


# ---------------------------------------------------------------------------
# collate_sample_row
# ---------------------------------------------------------------------------


class TestCollateSampleRow(TestCase):
    """Tests for collate_sample_row."""

    def test_no_collate_fn_calls_pad_row(self):
        """Without a collate fn, _pad_row is invoked."""
        with patch(
            "michelangelo.workflow.tasks.tabular_trainer._dataset._pad_row",
            return_value={"x": np.array([1.0])},
        ) as mock_pad:
            result = collate_sample_row({"x": np.array([1.0])})
        mock_pad.assert_called_once()
        self.assertIn("x", result)

    def test_no_collate_fn_metadata_removed(self):
        """Metadata columns are stripped before _pad_row is called."""
        row = {"x": np.array([1.0]), "meta_id": "abc"}
        result = collate_sample_row(row, metadata_columns=["meta_id"])
        self.assertIn("x", result)
        self.assertNotIn("meta_id", result)

    def test_with_collate_fn_returns_numpy(self):
        """With a collate fn, tensors are squeezed to numpy."""
        import torch

        def _collate(batch):
            return {"x": torch.tensor([[1.0, 2.0]])}

        result = collate_sample_row(
            {"x": np.array([1.0, 2.0])}, data_collate_fn=_collate
        )
        self.assertIsInstance(result["x"], np.ndarray)
        self.assertEqual(result["x"].shape, (2,))

    def test_with_collate_fn_metadata_removed(self):
        """Metadata columns are popped from the collate output."""
        import torch

        def _collate(batch):
            return {"x": torch.tensor([[1.0]]), "meta_id": torch.tensor([[0]])}

        result = collate_sample_row(
            {"x": np.array([1.0]), "meta_id": np.array([0])},
            data_collate_fn=_collate,
            metadata_columns=["meta_id"],
        )
        self.assertIn("x", result)
        self.assertNotIn("meta_id", result)

    def test_with_collate_fn_string_wrapped_as_object(self):
        """String values are wrapped in object arrays before collate."""
        import torch

        captured = {}

        def _collate(batch):
            captured.update(batch)
            return {"x": torch.tensor([[1.0]])}

        collate_sample_row({"x": "abc"}, data_collate_fn=_collate)
        self.assertEqual(captured["x"].dtype, object)

    def test_without_collate_fn_no_metadata_columns(self):
        """Passing metadata_columns=None does not raise."""
        result = collate_sample_row({"x": np.array(1.0)}, metadata_columns=None)
        self.assertIn("x", result)


# ---------------------------------------------------------------------------
# get_sample_data
# ---------------------------------------------------------------------------


class TestGetSampleData(TestCase):
    """Tests for get_sample_data."""

    def test_returns_list_of_one_dict(self):
        """It always returns a single-element list."""
        result = get_sample_data(
            {"x": np.array(1.0)},
            {"x": ColumnConfig("torch.float32")},
        )
        self.assertIsInstance(result, list)
        self.assertEqual(len(result), 1)
        self.assertIsInstance(result[0], dict)

    def test_dtype_cast(self):
        """Values are cast to the dtype specified in ColumnConfig."""
        result = get_sample_data(
            {"x": np.array(1.0, dtype=np.float64)},
            {"x": ColumnConfig("torch.float32")},
        )
        self.assertEqual(result[0]["x"].dtype, np.float32)

    def test_reshape_when_element_count_matches(self):
        """Data is reshaped when the total element count matches expected shape."""
        result = get_sample_data(
            {"x": np.array([1.0, 2.0, 3.0, 4.0])},
            {"x": ColumnConfig("torch.float32", [2, 2])},
        )
        self.assertEqual(result[0]["x"].shape, (2, 2))

    def test_no_reshape_when_shape_empty(self):
        """Scalar columns (shape=[]) are not reshaped."""
        result = get_sample_data(
            {"x": np.array(1.0)},
            {"x": ColumnConfig("torch.float32", [])},
        )
        self.assertIn("x", result[0])

    def test_missing_feature_skipped(self):
        """A feature not in sample_data_dict is skipped (no KeyError)."""
        result = get_sample_data(
            {},
            {"x": ColumnConfig("torch.float32")},
        )
        self.assertNotIn("x", result[0])

    def test_stringified_list_parsed(self):
        """A string value starting with '[' is parsed via ast.literal_eval."""
        result = get_sample_data(
            {"x": "[1.0, 2.0]"},
            {"x": ColumnConfig("torch.float32")},
        )
        self.assertEqual(result[0]["x"].dtype, np.float32)
        self.assertEqual(len(result[0]["x"]), 2)

    def test_object_array_of_strings_parsed(self):
        """Object arrays whose elements are stringified lists are parsed."""
        raw = np.array(["[1.0, 2.0]"], dtype=object)
        result = get_sample_data(
            {"x": raw},
            {"x": ColumnConfig("torch.float32")},
        )
        self.assertEqual(result[0]["x"].dtype, np.float32)

    def test_object_array_ragged_strings_padded(self):
        """Ragged stringified rows in an object array are padded (not crashed)."""
        raw = np.array(["[1.0]", "[2.0, 3.0]"], dtype=object)
        result = get_sample_data(
            {"x": raw},
            {"x": ColumnConfig("torch.float32")},
        )
        self.assertIsInstance(result[0]["x"], np.ndarray)
        self.assertNotEqual(result[0]["x"].dtype, object)

    def test_shape_mismatch_no_reshape_warn_and_continue(self):
        """Shape mismatch with no valid reshape logs a warning and passes through."""
        raw = np.array([1.0, 2.0, 3.0], dtype=np.float32)
        with self.assertLogs(
            "michelangelo.workflow.tasks.tabular_trainer._dataset", level="WARNING"
        ) as log:
            result = get_sample_data(
                {"x": raw},
                {"x": ColumnConfig("torch.float32", [2, 2])},
            )
        self.assertIn("shape mismatch", " ".join(log.output))
        self.assertEqual(result[0]["x"].shape, (3,))

    def test_only_input_columns_included(self):
        """Keys not in input_columns are excluded from the result."""
        result = get_sample_data(
            {"x": np.array(1.0), "y": np.array(0)},
            {"x": ColumnConfig("torch.float32")},
        )
        self.assertIn("x", result[0])
        self.assertNotIn("y", result[0])


# ---------------------------------------------------------------------------
# Deprecation warnings
# ---------------------------------------------------------------------------


class TestDeprecationWarnings(TestCase):
    """Tests for raise_lightning_trainer_config_deprecation_warnings."""

    def test_no_hyperparameters_no_warning(self):
        """No warning is emitted when hyperparameters is None."""
        cfg = _lightning_cfg(hyperparameters=None)
        with warnings.catch_warnings():
            warnings.simplefilter("error")
            raise_lightning_trainer_config_deprecation_warnings(cfg)

    def test_deprecated_hyperparameter_num_epochs(self):
        """Num_epochs in hyperparameters emits a DeprecationWarning."""
        cfg = _lightning_cfg(hyperparameters={"num_epochs": 10})
        with self.assertWarns(DeprecationWarning):
            raise_lightning_trainer_config_deprecation_warnings(cfg)

    def test_deprecated_hyperparameter_precision(self):
        """Precision in hyperparameters emits a DeprecationWarning."""
        cfg = _lightning_cfg(hyperparameters={"precision": "bf16-mixed"})
        with self.assertWarns(DeprecationWarning):
            raise_lightning_trainer_config_deprecation_warnings(cfg)

    def test_deprecated_hyperparameter_batch_size(self):
        """Batch_size in hyperparameters emits a DeprecationWarning."""
        cfg = _lightning_cfg(hyperparameters={"batch_size": 64})
        with self.assertWarns(DeprecationWarning):
            raise_lightning_trainer_config_deprecation_warnings(cfg)

    def test_deprecated_hyperparameter_num_shuffle_batches(self):
        """Num_shuffle_batches in hyperparameters emits a DeprecationWarning."""
        cfg = _lightning_cfg(hyperparameters={"num_shuffle_batches": 4})
        with self.assertWarns(DeprecationWarning):
            raise_lightning_trainer_config_deprecation_warnings(cfg)

    def test_unknown_hyperparameter_emits_user_warning(self):
        """An unknown hyperparameter key emits a UserWarning."""
        cfg = _lightning_cfg(hyperparameters={"unknown_key": "value"})
        with self.assertWarns(UserWarning):
            raise_lightning_trainer_config_deprecation_warnings(cfg)

    def test_no_deprecated_config_fields_no_warning(self):
        """No warning is emitted for a clean config with no deprecated fields."""
        cfg = _lightning_cfg()
        with warnings.catch_warnings():
            warnings.simplefilter("error")
            raise_lightning_trainer_config_deprecation_warnings(cfg)


# ---------------------------------------------------------------------------
# construct_read_kwargs
# ---------------------------------------------------------------------------


class TestConstructReadKwargs(TestCase):
    """Tests for construct_read_kwargs."""

    def test_columns_include_inputs_labels_metadata(self):
        """Columns = sorted(inputs | labels | metadata)."""
        cfg = _lightning_cfg(
            input_columns={"feat_a": ColumnConfig("torch.float32")},
            labels={"label": ColumnConfig("torch.long")},
            metadata_columns=["user_id"],
        )
        result = construct_read_kwargs(cfg)
        self.assertEqual(result["columns"], sorted(["feat_a", "label", "user_id"]))

    def test_output_columns_excluded(self):
        """output_columns are NOT included in the column projection."""
        cfg = _lightning_cfg(
            input_columns={"x": ColumnConfig("torch.float32")},
            output_columns={"y": ColumnConfig("torch.float32")},
            labels={"label": ColumnConfig("torch.long")},
            metadata_columns=[],
        )
        result = construct_read_kwargs(cfg)
        self.assertNotIn("y", result.get("columns", []))

    def test_columns_sorted(self):
        """Columns list is always sorted."""
        cfg = _lightning_cfg(
            input_columns={
                "z_feat": ColumnConfig("torch.float32"),
                "a_feat": ColumnConfig("torch.float32"),
            },
            labels={"m_label": ColumnConfig("torch.long")},
            metadata_columns=[],
        )
        result = construct_read_kwargs(cfg)
        self.assertEqual(result["columns"], sorted(result["columns"]))

    def test_no_dataloading_config(self):
        """With no dataloading_config, only columns key is set."""
        cfg = _lightning_cfg(dataloading_config=None)
        result = construct_read_kwargs(cfg)
        self.assertIn("columns", result)
        self.assertNotIn("num_cpus", result)

    def test_parquet_read_config_num_cpus(self):
        """ParquetReadConfig.num_cpus is forwarded."""
        from michelangelo.workflow.schema.ray_data_io import (
            DataloadingConfig,
            ParquetReadConfig,
        )

        cfg = _lightning_cfg(
            dataloading_config=DataloadingConfig(
                parquet_read_config=ParquetReadConfig(num_cpus=4.0)
            )
        )
        result = construct_read_kwargs(cfg)
        self.assertEqual(result["num_cpus"], 4.0)

    def test_parquet_read_config_shuffle(self):
        """ParquetReadConfig.shuffle is forwarded."""
        from michelangelo.workflow.schema.ray_data_io import (
            DataloadingConfig,
            ParquetReadConfig,
        )

        cfg = _lightning_cfg(
            dataloading_config=DataloadingConfig(
                parquet_read_config=ParquetReadConfig(shuffle="files")
            )
        )
        result = construct_read_kwargs(cfg)
        self.assertEqual(result["shuffle"], "files")

    def test_none_parquet_fields_not_included(self):
        """ParquetReadConfig fields that are None are not added to the dict."""
        from michelangelo.workflow.schema.ray_data_io import (
            DataloadingConfig,
            ParquetReadConfig,
        )

        cfg = _lightning_cfg(
            dataloading_config=DataloadingConfig(
                parquet_read_config=ParquetReadConfig(num_cpus=None, shuffle=None)
            )
        )
        result = construct_read_kwargs(cfg)
        self.assertNotIn("num_cpus", result)
        self.assertNotIn("shuffle", result)

    def test_empty_metadata_columns(self):
        """Empty metadata_columns list does not add spurious entries."""
        cfg = _lightning_cfg(metadata_columns=[])
        result = construct_read_kwargs(cfg)
        # columns should be inputs | labels only
        expected = sorted(list(cfg.input_columns.keys()) + list(cfg.labels.keys()))
        self.assertEqual(result["columns"], expected)
