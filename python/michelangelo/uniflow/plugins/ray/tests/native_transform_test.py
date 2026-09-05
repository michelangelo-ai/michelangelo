"""Tests for the Ray native_transform execution adapter."""

from __future__ import annotations

import unittest
from unittest.mock import MagicMock, call, patch

import numpy as np
import pyarrow as pa
import torch

from michelangelo.lib._internal.utils.numpy_utils import sentinel_for_numpy_dtype
from michelangelo.uniflow.plugins.ray.native_transform import (
    DefaultDataProcessor,
    TorchBatchPredictor,
    _pad_ragged_arrays,
    check_stats_exist,
    compute_numerical_statistics,
    filter_columns,
    get_numerical_stats_names,
    get_torch_dtype,
    get_transform_torch_inference,
    numerical_statistics_preparation,
    read_dataset,
    transform,
    write_dataset,
)

_MODULE = "michelangelo.uniflow.plugins.ray.native_transform"


class DummyModel:
    """A stand-in torch-model callable for TorchBatchPredictor tests."""

    def __init__(self, outputs=None, raise_error=False):
        """Initialize with fixed outputs, or configure a forced failure."""
        self.outputs = outputs or {}
        self.raise_error = raise_error
        self.last_inputs = None

    def __call__(self, inputs):
        """Record the received inputs and return the configured outputs."""
        self.last_inputs = inputs
        if self.raise_error:
            raise RuntimeError("inference failed")
        return self.outputs


class FilterColumnsTests(unittest.TestCase):
    """Tests for filter columns."""

    def test_keeps_existing_columns(self):
        """Keeps existing columns."""
        mock_dataset = MagicMock()
        mock_dataset.schema.return_value.names = ["col1", "col2", "col3"]

        filter_columns(mock_dataset, ["col1", "col3"])

        mock_dataset.select_columns.assert_called_once_with(["col1", "col3"])

    def test_ignores_nonexistent_columns(self):
        """Ignores nonexistent columns."""
        mock_dataset = MagicMock()
        mock_dataset.schema.return_value.names = ["col1", "col2"]

        filter_columns(mock_dataset, ["col1", "nonexistent"])

        mock_dataset.select_columns.assert_called_once_with(["col1"])

    def test_empty_keep_list(self):
        """Empty keep list."""
        mock_dataset = MagicMock()
        mock_dataset.schema.return_value.names = ["col1", "col2"]

        filter_columns(mock_dataset, [])

        mock_dataset.select_columns.assert_called_once_with([])


class ReadWriteDatasetTests(unittest.TestCase):
    """read_dataset/write_dataset route filesystem resolution through _fs_path."""

    @patch(f"{_MODULE}.ray")
    @patch(f"{_MODULE}._fs_path")
    def test_read_dataset_uses_fs_path(self, mock_fs_path, mock_ray):
        """Read dataset uses fs path."""
        mock_fs = MagicMock()
        mock_fs_path.return_value = (mock_fs, "/resolved/path")
        mock_dataset = MagicMock()
        mock_ray.data.read_parquet.return_value = mock_dataset

        result = read_dataset("s3://some/path")

        mock_fs_path.assert_called_once_with("s3://some/path")
        mock_ray.data.read_parquet.assert_called_once_with(
            "/resolved/path", filesystem=mock_fs
        )
        self.assertIs(result, mock_dataset)

    @patch(f"{_MODULE}._fs_path")
    def test_read_dataset_propagates_fs_path_errors(self, mock_fs_path):
        """Read dataset propagates fs path errors."""
        mock_fs_path.side_effect = ValueError("invalid path")

        with self.assertRaises(ValueError):
            read_dataset("invalid://path")

    @patch(f"{_MODULE}._fs_path")
    def test_write_dataset_default_mode(self, mock_fs_path):
        """Write dataset default mode."""
        mock_fs = MagicMock()
        mock_fs_path.return_value = (mock_fs, "/resolved/out")
        mock_dataset = MagicMock()

        write_dataset(mock_dataset, "s3://bucket/out")

        mock_fs_path.assert_called_once_with("s3://bucket/out")
        mock_dataset.write_parquet.assert_called_once_with(
            "/resolved/out", filesystem=mock_fs, mode="append"
        )

    @patch(f"{_MODULE}._fs_path")
    def test_write_dataset_overwrite_mode(self, mock_fs_path):
        """Write dataset overwrite mode."""
        mock_fs_path.return_value = (None, "/resolved/out")
        mock_dataset = MagicMock()

        write_dataset(mock_dataset, "/local/out", mode="overwrite")

        mock_dataset.write_parquet.assert_called_once_with(
            "/resolved/out", filesystem=None, mode="overwrite"
        )

    def test_no_custom_filesystem_registration_side_effect(self):
        """Importing the module must not register any custom fsspec filesystem."""
        import inspect

        from michelangelo.uniflow.plugins.ray import native_transform as module

        source = inspect.getsource(module)
        self.assertNotIn("register_implementation", source)
        self.assertNotIn("fsspec.register", source)


class ComputeNumericalStatisticsTests(unittest.TestCase):
    """Tests for compute numerical statistics."""

    def test_basic(self):
        """Basic."""
        mock_dataset = MagicMock()
        mock_dataset.select_columns.return_value = mock_dataset
        mock_dataset.aggregate.return_value = {
            "feature_50": 5.0,
            "feature_75": 7.5,
            "feature_max": 10.0,
            "feature_min": 0.0,
            "feature_mean": 5.0,
            "feature_std": 2.5,
        }
        specs = {
            "feature": {
                "percentiles": ["0.5", "0.75"],
                "max": True,
                "min": True,
                "mean": True,
                "std": True,
            }
        }

        mock_aggregate = MagicMock()
        with patch.dict("sys.modules", {"ray.data.aggregate": mock_aggregate}):
            result = compute_numerical_statistics(
                mock_dataset,
                existing_numerical_stats={},
                numerical_statistics_computation_specs=specs,
            )

        mock_dataset.select_columns.assert_called_once_with(["feature"])
        self.assertTrue(mock_dataset.aggregate.called)
        self.assertIsInstance(result, dict)

    def test_skips_existing_stats(self):
        """Skips existing stats."""
        mock_dataset = MagicMock()
        mock_dataset.select_columns.return_value = mock_dataset
        mock_dataset.aggregate.return_value = {"feature_75": 7.5}
        existing_stats = {"feature_50": 5.0}
        specs = {
            "feature": {
                "percentiles": ["0.5", "0.75"],
                "max": False,
                "min": False,
                "mean": False,
                "std": False,
            }
        }

        mock_aggregate = MagicMock()
        with patch.dict("sys.modules", {"ray.data.aggregate": mock_aggregate}):
            compute_numerical_statistics(
                mock_dataset,
                existing_numerical_stats=existing_stats,
                numerical_statistics_computation_specs=specs,
            )

        self.assertTrue(mock_dataset.aggregate.called)

    def test_all_existing_returns_empty_without_aggregate_call(self):
        """All existing returns empty without aggregate call."""
        mock_dataset = MagicMock()
        existing_stats = {"feature_50": 5.0, "feature_max": 10.0, "feature_min": 0.0}
        specs = {
            "feature": {
                "percentiles": ["0.5"],
                "max": True,
                "min": True,
                "mean": False,
                "std": False,
            }
        }

        mock_aggregate = MagicMock()
        with patch.dict("sys.modules", {"ray.data.aggregate": mock_aggregate}):
            result = compute_numerical_statistics(
                mock_dataset,
                existing_numerical_stats=existing_stats,
                numerical_statistics_computation_specs=specs,
            )

        self.assertEqual(result, {})
        mock_dataset.aggregate.assert_not_called()

    def test_empty_specs_skips_projection(self):
        """Empty specs skips projection."""
        mock_dataset = MagicMock()
        mock_aggregate = MagicMock()
        with patch.dict("sys.modules", {"ray.data.aggregate": mock_aggregate}):
            result = compute_numerical_statistics(
                mock_dataset,
                existing_numerical_stats={},
                numerical_statistics_computation_specs={},
            )

        mock_dataset.select_columns.assert_not_called()
        self.assertEqual(result, {})

    def test_batches_aggregate_calls(self):
        """Batches aggregate calls.

        2 aggregate fns (max, min) with a batch size of 1 must produce
        exactly 2 non-empty batches/calls — not 3. (A prior version of this
        test used a single aggregate fn with batch size 1, which happened to
        pass under the old buggy `// batch_fn_size + 1` bound because that
        formula's extra empty trailing batch was harmless with a mock
        `return_value`; it would have crashed against real Ray, per GAP-006.)
        """
        mock_dataset = MagicMock()
        mock_dataset.select_columns.return_value = mock_dataset
        mock_dataset.aggregate.return_value = {}
        specs = {
            "col1": {
                "percentiles": [],
                "max": True,
                "min": True,
                "mean": False,
                "std": False,
            }
        }

        mock_aggregate = MagicMock()
        with patch.dict("sys.modules", {"ray.data.aggregate": mock_aggregate}):
            compute_numerical_statistics(
                mock_dataset,
                existing_numerical_stats={},
                numerical_statistics_computation_specs=specs,
                numerical_statistics_batch_fn_size=1,
            )

        self.assertEqual(mock_dataset.aggregate.call_count, 2)

    def test_batch_count_covers_all_aggregate_fns_not_just_specs(self):
        """batch_count must scale with aggregate_fns, not with column specs.

        Two columns each contribute 4 aggregate fns (max/min/mean/std), for 8
        total, while there are only 2 specs. With a batch size of 2, a
        batch_count derived from spec count would only cover the first 4
        aggregate fns, silently dropping the rest.
        """

        class _FakeAggFn:
            def __init__(self, input_col, alias_name=None, **_kwargs):
                self.input_col = input_col
                self.alias_name = alias_name

        mock_dataset = MagicMock()
        mock_dataset.select_columns.return_value = mock_dataset
        mock_dataset.aggregate.side_effect = lambda *fns: {
            fn.alias_name: 1.0 for fn in fns
        }
        specs = {
            "col1": {
                "percentiles": [],
                "max": True,
                "min": True,
                "mean": True,
                "std": True,
            },
            "col2": {
                "percentiles": [],
                "max": True,
                "min": True,
                "mean": True,
                "std": True,
            },
        }

        mock_aggregate = MagicMock()
        mock_aggregate.Max = mock_aggregate.Min = mock_aggregate.Mean = (
            mock_aggregate.Std
        ) = _FakeAggFn
        with patch.dict("sys.modules", {"ray.data.aggregate": mock_aggregate}):
            result = compute_numerical_statistics(
                mock_dataset,
                existing_numerical_stats={},
                numerical_statistics_computation_specs=specs,
                numerical_statistics_batch_fn_size=2,
            )

        expected_keys = {
            f"{col}_{stat}"
            for col in ("col1", "col2")
            for stat in ("max", "min", "mean", "std")
        }
        self.assertEqual(set(result.keys()), expected_keys)

    def test_batch_count_does_not_add_empty_trailing_batch_on_exact_multiple(self):
        """No empty batch when aggregate_fns is an exact multiple of the batch size.

        Two columns x 4 aggregate fns each = 8 total, with a batch size of 4:
        len(aggregate_fns) % batch_fn_size == 0. `// batch_fn_size + 1` would
        add a third, empty batch, and calling ray_data_df.aggregate() with no
        aggregators crashes inside Ray (assert self._columns in
        table_block.py). aggregate() must be called exactly twice, never with
        an empty argument list.
        """

        class _FakeAggFn:
            def __init__(self, input_col, alias_name=None, **_kwargs):
                self.input_col = input_col
                self.alias_name = alias_name

        def _aggregate(*fns):
            assert fns, "aggregate() must never be called with zero aggregators"
            return {fn.alias_name: 1.0 for fn in fns}

        mock_dataset = MagicMock()
        mock_dataset.select_columns.return_value = mock_dataset
        mock_dataset.aggregate.side_effect = _aggregate
        specs = {
            "col1": {
                "percentiles": [],
                "max": True,
                "min": True,
                "mean": True,
                "std": True,
            },
            "col2": {
                "percentiles": [],
                "max": True,
                "min": True,
                "mean": True,
                "std": True,
            },
        }

        mock_aggregate = MagicMock()
        mock_aggregate.Max = mock_aggregate.Min = mock_aggregate.Mean = (
            mock_aggregate.Std
        ) = _FakeAggFn
        with patch.dict("sys.modules", {"ray.data.aggregate": mock_aggregate}):
            result = compute_numerical_statistics(
                mock_dataset,
                existing_numerical_stats={},
                numerical_statistics_computation_specs=specs,
                numerical_statistics_batch_fn_size=4,
            )

        self.assertEqual(mock_dataset.aggregate.call_count, 2)
        expected_keys = {
            f"{col}_{stat}"
            for col in ("col1", "col2")
            for stat in ("max", "min", "mean", "std")
        }
        self.assertEqual(set(result.keys()), expected_keys)


class GetTorchDtypeTests(unittest.TestCase):
    """Tests for get torch dtype."""

    def test_scalar(self):
        """Scalar."""
        self.assertEqual(get_torch_dtype(pa.float32()), torch.float32)

    def test_list_type(self):
        """List type."""
        self.assertEqual(get_torch_dtype(pa.list_(pa.int32())), torch.int32)

    def test_fixed_size_list_type(self):
        """Fixed size list type."""
        self.assertEqual(get_torch_dtype(pa.list_(pa.float64(), 3)), torch.float64)

    def test_nested_list_type(self):
        """Nested list type."""
        self.assertEqual(
            get_torch_dtype(pa.list_(pa.list_(pa.float64()))), torch.float64
        )

    def test_triple_nested_list_type(self):
        """Triple nested list type."""
        self.assertEqual(
            get_torch_dtype(pa.list_(pa.list_(pa.list_(pa.int32())))), torch.int32
        )

    def test_unsupported_scalar_raises(self):
        """Unsupported scalar raises."""
        with self.assertRaises(ValueError):
            get_torch_dtype(pa.string())

    def test_unsupported_list_type_raises(self):
        """Unsupported list type raises."""
        with self.assertRaises(ValueError):
            get_torch_dtype(pa.list_(pa.bool_()))


class GetNumericalStatsNamesTests(unittest.TestCase):
    """Tests for get numerical stats names."""

    def test_empty_specs(self):
        """Empty specs."""
        self.assertEqual(get_numerical_stats_names({}), [])

    def test_all_stats_enabled(self):
        """All stats enabled."""
        specs = {
            "col1": {
                "min": True,
                "max": True,
                "std": True,
                "mean": True,
                "percentiles": ["0.25", "0.5", "0.75"],
            }
        }
        result = get_numerical_stats_names(specs)
        expected = [
            "col1_min",
            "col1_max",
            "col1_std",
            "col1_mean",
            "col1_25",
            "col1_50",
            "col1_75",
        ]
        self.assertEqual(sorted(result), sorted(expected))

    def test_partial_stats(self):
        """Partial stats."""
        specs = {
            "col1": {"min": True, "max": False, "percentiles": ["0.1"]},
            "col2": {"std": True, "mean": True, "percentiles": []},
        }
        result = get_numerical_stats_names(specs)
        expected = ["col1_min", "col1_10", "col2_std", "col2_mean"]
        self.assertEqual(sorted(result), sorted(expected))

    def test_no_stats_enabled(self):
        """No stats enabled."""
        specs = {
            "col1": {
                "min": False,
                "max": False,
                "std": False,
                "mean": False,
                "percentiles": [],
            }
        }
        self.assertEqual(get_numerical_stats_names(specs), [])


class CheckStatsExistTests(unittest.TestCase):
    """Tests for check stats exist."""

    def setUp(self):
        """Create a mock TransformSpec shared across test cases."""
        self.mock_spec = MagicMock()

    @patch(f"{_MODULE}.get_numerical_stats_names")
    def test_all_exist(self, mock_names):
        """All exist."""
        mock_names.return_value = ["col1_mean", "col2_std"]
        self.mock_spec.get_numerical_statistics_computation_specs.return_value = {}

        self.assertTrue(
            check_stats_exist({"col1_mean": 0.5, "col2_std": 1.0}, self.mock_spec, 0)
        )

    @patch(f"{_MODULE}.get_numerical_stats_names")
    def test_missing(self, mock_names):
        """Missing."""
        mock_names.return_value = ["col1_mean", "col2_std"]
        self.mock_spec.get_numerical_statistics_computation_specs.return_value = {}

        self.assertFalse(check_stats_exist({"col1_mean": 0.5}, self.mock_spec, 0))

    @patch(f"{_MODULE}.get_numerical_stats_names")
    def test_none_required(self, mock_names):
        """None required."""
        mock_names.return_value = []
        self.mock_spec.get_numerical_statistics_computation_specs.return_value = {}

        self.assertTrue(check_stats_exist({}, self.mock_spec, 0))


class NumericalStatisticsPreparationTests(unittest.TestCase):
    """Tests for numerical statistics preparation."""

    def setUp(self):
        """Create mock dataset/spec fixtures shared across test cases."""
        self.mock_dataset = MagicMock()
        self.mock_spec = MagicMock()
        self.feature_stats = {}

    @patch(f"{_MODULE}.compute_numerical_statistics")
    def test_with_specs_hydrates_all_layer_kinds(self, mock_compute):
        """With specs hydrates all layer kinds."""
        specs = {"col1": {"mean": True}}
        self.mock_spec.get_numerical_statistics_computation_specs.return_value = specs
        mock_compute.return_value = {"col1_mean": 0.5}

        numerical_statistics_preparation(
            self.mock_dataset, self.feature_stats, self.mock_spec, 0
        )

        mock_compute.assert_called_once_with(
            self.mock_dataset, self.feature_stats, specs
        )
        self.assertEqual(self.feature_stats, {"col1_mean": 0.5})
        self.mock_spec.update_numerical_standard_transform_parameters.assert_called_once_with(
            self.feature_stats, 0
        )
        self.mock_spec.update_standard_scaler_specs.assert_called_once_with(
            self.feature_stats, 0
        )
        self.mock_spec.update_min_max_scaler_specs.assert_called_once_with(
            self.feature_stats, 0
        )
        self.mock_spec.update_bucketization_specs.assert_called_once_with(
            self.feature_stats, 0
        )

    def test_without_specs_skips_hydration(self):
        """Without specs skips hydration."""
        self.mock_spec.get_numerical_statistics_computation_specs.return_value = {}

        numerical_statistics_preparation(
            self.mock_dataset, self.feature_stats, self.mock_spec, 0
        )

        self.mock_spec.update_numerical_standard_transform_parameters.assert_not_called()
        self.mock_spec.update_standard_scaler_specs.assert_not_called()
        self.mock_spec.update_min_max_scaler_specs.assert_not_called()
        self.mock_spec.update_bucketization_specs.assert_not_called()


class GetTransformTorchInferenceTests(unittest.TestCase):
    """Tests for get transform torch inference."""

    def setUp(self):
        """Create a mock TransformSpec with a fixed max transform level."""
        self.mock_spec = MagicMock()
        self.mock_spec.get_max_transform_level.return_value = 2
        self.mock_spec.columns_to_keep = ["keep_col"]

    @patch(f"{_MODULE}.TorchBatchPredictor")
    @patch(f"{_MODULE}.get_transform_module")
    def test_with_layers(self, mock_get_module, mock_predictor):
        """With layers."""
        mock_module = MagicMock(input_cols=["input1"], output_cols=["output1"])
        mock_get_module.return_value = mock_module
        self.mock_spec.get_transform_input_cols.side_effect = [["input2"], ["input3"]]

        result = get_transform_torch_inference(self.mock_spec, [0], 1)

        mock_predictor.assert_called_once()
        call_kwargs = mock_predictor.call_args.kwargs
        self.assertEqual(call_kwargs["model"], mock_module)
        self.assertIsNotNone(result)

    @patch(f"{_MODULE}.get_transform_module")
    def test_no_layers_returns_none(self, mock_get_module):
        """No layers returns none."""
        mock_get_module.return_value = None

        self.assertIsNone(get_transform_torch_inference(self.mock_spec, [], 0))

    @patch(f"{_MODULE}.TorchBatchPredictor")
    @patch(f"{_MODULE}.get_transform_module")
    def test_empty_finished_levels_starts_at_zero(
        self, mock_get_module, mock_predictor
    ):
        """Empty finished levels starts at zero."""
        mock_module = MagicMock(input_cols=["input1"], output_cols=["output1"])
        mock_get_module.return_value = mock_module
        self.mock_spec.get_transform_input_cols.side_effect = [["input2"], ["input3"]]

        get_transform_torch_inference(self.mock_spec, [], 0)

        mock_get_module.assert_called_once_with(self.mock_spec, 0, 0)

    @patch(f"{_MODULE}.TorchBatchPredictor")
    @patch(f"{_MODULE}.get_transform_module")
    def test_columns_to_keep_none_keeps_all(self, mock_get_module, mock_predictor):
        """Columns to keep none keeps all."""
        self.mock_spec.columns_to_keep = None
        mock_module = MagicMock(input_cols=["input1"], output_cols=["output1"])
        mock_get_module.return_value = mock_module

        get_transform_torch_inference(self.mock_spec, [], 0)

        self.assertIsNone(mock_predictor.call_args.kwargs["columns_to_keep"])

    @patch(f"{_MODULE}.TorchBatchPredictor")
    @patch(f"{_MODULE}.get_transform_module")
    def test_columns_to_keep_includes_future_inputs(
        self, mock_get_module, mock_predictor
    ):
        """Columns to keep includes future inputs."""
        self.mock_spec.columns_to_keep = ["user_col1", "user_col2"]
        mock_module = MagicMock(input_cols=["input1"], output_cols=["output1"])
        mock_get_module.return_value = mock_module
        self.mock_spec.get_transform_input_cols.side_effect = [
            ["future_input1"],
            ["future_input2"],
        ]

        get_transform_torch_inference(self.mock_spec, [], 0)

        self.assertEqual(
            mock_predictor.call_args.kwargs["columns_to_keep"],
            {"user_col1", "user_col2", "future_input1", "future_input2"},
        )


class TransformOrchestrationTests(unittest.TestCase):
    """Tests for transform orchestration."""

    def setUp(self):
        """Create a mock Ray dataset and TransformSpec for the transform() suite."""
        self.mock_dataset = MagicMock()
        self.mock_schema = MagicMock()
        self.mock_schema.names = ["col1", "col2", "col3"]
        self.mock_schema.types = [pa.float32(), pa.float64(), pa.int64()]
        self.mock_dataset.schema.return_value = self.mock_schema
        self.mock_dataset.map_batches.return_value = self.mock_dataset
        self.mock_dataset.materialize.return_value = self.mock_dataset

        self.mock_spec = MagicMock()
        self.mock_spec.get_max_transform_level.return_value = 2
        self.mock_spec.transform_specs = {
            "layer1": MagicMock(output_cols=["out1"], output_dtype=torch.float32),
            "layer2": MagicMock(output_cols=["out2"], output_dtype=torch.int32),
        }
        self.mock_spec.columns_to_keep = ["col1", "out1"]

        self.feature_stats = {"col1_mean": 0.5}

    @patch(f"{_MODULE}.get_transform_torch_inference")
    @patch(f"{_MODULE}.numerical_statistics_preparation")
    @patch(f"{_MODULE}.check_stats_exist")
    def test_transform_with_inference(
        self, mock_check_stats, mock_numerical_prep, mock_get_inference
    ):
        """Transform with inference."""
        mock_check_stats.return_value = False
        mock_torch_inference = MagicMock()
        mock_get_inference.return_value = mock_torch_inference

        result_df, result_spec, result_stats = transform(
            self.mock_dataset, self.mock_spec, self.feature_stats
        )

        self.assertEqual(result_df, self.mock_dataset)
        self.assertEqual(result_spec, self.mock_spec)
        self.assertEqual(result_stats, self.feature_stats)
        self.assertEqual(mock_check_stats.call_count, 3)
        self.assertEqual(mock_get_inference.call_count, 4)
        self.assertEqual(self.mock_dataset.map_batches.call_count, 4)

    @patch(f"{_MODULE}.get_transform_torch_inference")
    @patch(f"{_MODULE}.numerical_statistics_preparation")
    @patch(f"{_MODULE}.check_stats_exist")
    def test_transform_without_inference(
        self, mock_check_stats, mock_numerical_prep, mock_get_inference
    ):
        """Transform without inference."""
        mock_check_stats.return_value = True
        mock_get_inference.return_value = None

        result_df, _, _ = transform(
            self.mock_dataset, self.mock_spec, self.feature_stats
        )

        self.assertEqual(result_df, self.mock_dataset)
        self.assertEqual(self.mock_dataset.map_batches.call_count, 0)

    @patch("torch.cuda.is_available")
    @patch(f"{_MODULE}.get_transform_torch_inference")
    @patch(f"{_MODULE}.numerical_statistics_preparation")
    @patch(f"{_MODULE}.check_stats_exist")
    def test_transform_with_gpu(
        self, mock_check_stats, mock_numerical_prep, mock_get_inference, mock_cuda
    ):
        """Transform with gpu."""
        mock_check_stats.return_value = False
        mock_torch_inference = MagicMock()
        mock_get_inference.return_value = mock_torch_inference
        mock_cuda.return_value = True
        self.mock_spec.get_max_transform_level.return_value = 0

        transform(self.mock_dataset, self.mock_spec, self.feature_stats, num_gpus=2)

        expected_calls = [
            call(
                mock_torch_inference,
                batch_size=20000,
                num_gpus=2,
                zero_copy_batch=True,
                batch_format="pyarrow",
                concurrency=100,
                num_cpus=1,
            ),
            call(
                mock_torch_inference,
                batch_size=20000,
                num_gpus=2,
                zero_copy_batch=True,
                batch_format="pyarrow",
                concurrency=100,
                num_cpus=1,
            ),
        ]
        self.mock_dataset.map_batches.assert_has_calls(expected_calls)

    @patch(f"{_MODULE}.get_transform_torch_inference")
    @patch(f"{_MODULE}.numerical_statistics_preparation")
    @patch(f"{_MODULE}.check_stats_exist")
    def test_skips_non_numeric_columns(
        self, mock_check_stats, mock_numerical_prep, mock_get_inference
    ):
        """Skips non numeric columns."""
        self.mock_schema.names = ["numeric_col", "string_col", "another_numeric"]
        self.mock_schema.types = [pa.float32(), pa.string(), pa.int64()]
        mock_check_stats.return_value = True
        mock_get_inference.return_value = None

        with self.assertLogs(_MODULE, level="INFO") as logs:
            transform(self.mock_dataset, self.mock_spec, self.feature_stats)

        self.assertTrue(
            any("Skipping 1 non-numeric columns" in line for line in logs.output)
        )

    @patch(f"{_MODULE}.get_transform_torch_inference")
    @patch(f"{_MODULE}.numerical_statistics_preparation")
    @patch(f"{_MODULE}.check_stats_exist")
    def test_handles_multidim_numeric_columns(
        self, mock_check_stats, _mock_numerical_prep, mock_get_inference
    ):
        """Handles multidim numeric columns."""
        self.mock_schema.names = ["numeric_col", "embedding_col"]
        nested_list_type = pa.list_(pa.list_(pa.float64()))
        self.mock_schema.types = [pa.float32(), nested_list_type]
        mock_check_stats.return_value = True
        mock_get_inference.return_value = None

        mock_layer = MagicMock(
            input_cols=["embedding_col"],
            output_cols=["output"],
            output_dtype=torch.float32,
        )
        self.mock_spec.transform_specs = {"identity_layer": mock_layer}

        transform(self.mock_dataset, self.mock_spec, self.feature_stats)

        update_calls = self.mock_spec.update_input_dtype.call_args_list
        self.assertEqual(len(update_calls), 3)
        found = any(
            "embedding_col" in call_args.args[0]
            and call_args.args[0]["embedding_col"] == torch.float64
            for call_args in update_calls
        )
        self.assertTrue(
            found,
            "embedding_col should resolve to torch.float64 via nested list unwrapping",
        )


class DefaultDataProcessorTests(unittest.TestCase):
    """Tests for default data processor."""

    def test_prepare_inputs_numeric_arrays(self):
        """Prepare inputs numeric arrays."""
        processor = DefaultDataProcessor(input_columns=["x"])
        batch = {"x": np.array([[1, 2], [3, 4]], dtype=np.int32)}

        prepared = processor.prepare_inputs(batch)

        self.assertIsInstance(prepared["x"], torch.Tensor)
        self.assertEqual(prepared["x"].dtype, torch.int32)
        np.testing.assert_array_equal(prepared["x"].numpy(), [[1, 2], [3, 4]])

    def test_prepare_inputs_all_columns_when_none(self):
        """Prepare inputs all columns when none."""
        processor = DefaultDataProcessor(input_columns=None)
        batch = {
            "x": np.array([1.0, 2.0], dtype=np.float32),
            "y": np.array([3.0, 4.0], dtype=np.float32),
        }

        prepared = processor.prepare_inputs(batch)

        self.assertEqual(set(prepared.keys()), {"x", "y"})

    def test_prepare_inputs_string_arrays_rejected(self):
        """Prepare inputs string arrays rejected."""
        processor = DefaultDataProcessor(input_columns=["x"])
        batch = {"x": np.array(["a", "b"])}

        with self.assertRaises(ValueError):
            processor.prepare_inputs(batch)

    def test_prepare_inputs_bool_arrays_rejected(self):
        """Prepare inputs bool arrays rejected."""
        processor = DefaultDataProcessor(input_columns=["x"])
        batch = {"x": np.array([True, False])}

        with self.assertRaises(ValueError):
            processor.prepare_inputs(batch)

    def test_prepare_inputs_object_arrays_stackable(self):
        """Prepare inputs object arrays stackable."""
        processor = DefaultDataProcessor(input_columns=["x"])
        arr = np.empty(2, dtype=object)
        arr[0] = np.array([1.0, 2.0], dtype=np.float32)
        arr[1] = np.array([3.0, 4.0], dtype=np.float32)

        prepared = processor.prepare_inputs({"x": arr})

        self.assertEqual(tuple(prepared["x"].shape), (2, 2))

    def test_prepare_inputs_ragged_arrays_padded_with_sentinel(self):
        """Prepare inputs ragged arrays padded with sentinel."""
        processor = DefaultDataProcessor(input_columns=["x"])
        arr = np.empty(2, dtype=object)
        arr[0] = np.array([1, 2], dtype=np.int32)
        arr[1] = np.array([3, 4, 5], dtype=np.int32)

        prepared = processor.prepare_inputs({"x": arr})

        sentinel = sentinel_for_numpy_dtype(np.dtype(np.int32))
        self.assertEqual(tuple(prepared["x"].shape), (2, 3))
        np.testing.assert_array_equal(prepared["x"].numpy()[0], [1, 2, sentinel])

    def test_prepare_inputs_object_arrays_non_numeric_rejected(self):
        """Prepare inputs object arrays non numeric rejected."""
        processor = DefaultDataProcessor(input_columns=["x"])
        arr = np.empty(1, dtype=object)
        arr[0] = {"not": "an array"}

        with self.assertRaises(ValueError):
            processor.prepare_inputs({"x": arr})

    def test_prepare_inputs_nested_object_arrays_requiring_recursive_conversion(self):
        """Prepare inputs nested object arrays requiring recursive conversion."""
        processor = DefaultDataProcessor(input_columns=["x"])
        inner = np.empty(1, dtype=object)
        inner[0] = np.array([1.0, 2.0], dtype=np.float64)
        arr = np.empty(1, dtype=object)
        arr[0] = inner

        prepared = processor.prepare_inputs({"x": arr})

        self.assertIsInstance(prepared["x"], torch.Tensor)

    def test_prepare_inputs_large_integers_preserve_precision(self):
        """Prepare inputs large integers preserve precision."""
        processor = DefaultDataProcessor(input_columns=["x"])
        large_val = 2**53 + 1
        batch = {"x": np.array([large_val], dtype=np.int64)}

        prepared = processor.prepare_inputs(batch)

        self.assertEqual(prepared["x"].dtype, torch.int64)
        self.assertEqual(prepared["x"].item(), large_val)

    def test_postprocess_outputs_tensor_outputs(self):
        """Postprocess outputs tensor outputs."""
        processor = DefaultDataProcessor(output_columns=["y"])
        outputs = {"y": torch.tensor([1.0, 2.0])}

        result = processor.postprocess_outputs(outputs)

        np.testing.assert_array_equal(result["y"], [1.0, 2.0])

    def test_postprocess_outputs_all_when_none(self):
        """Postprocess outputs all when none."""
        processor = DefaultDataProcessor(output_columns=None)
        outputs = {"a": torch.tensor([1.0]), "b": torch.tensor([2.0])}

        result = processor.postprocess_outputs(outputs)

        self.assertEqual(set(result.keys()), {"a", "b"})

    def test_postprocess_outputs_list_outputs(self):
        """Postprocess outputs list outputs."""
        processor = DefaultDataProcessor(output_columns=["y"])
        result = processor.postprocess_outputs({"y": [1.0, 2.0, 3.0]})

        np.testing.assert_array_equal(result["y"], [1.0, 2.0, 3.0])

    def test_postprocess_outputs_missing_column_raises(self):
        """Postprocess outputs missing column raises."""
        processor = DefaultDataProcessor(output_columns=["y"])

        with self.assertRaises(ValueError):
            processor.postprocess_outputs({})


class PadRaggedArraysTests(unittest.TestCase):
    """Tests for pad ragged arrays."""

    def test_basic_int32(self):
        """Basic int32."""
        sentinel = sentinel_for_numpy_dtype(np.dtype(np.int32))
        arrays = np.array(
            [
                np.array([10, 20], dtype=np.int32),
                np.array([30, 40, 50], dtype=np.int32),
            ],
            dtype=object,
        )
        result = _pad_ragged_arrays(arrays)

        self.assertEqual(result.shape, (2, 3))
        self.assertEqual(result.dtype, np.int32)
        np.testing.assert_array_equal(result[0], [10, 20, sentinel])

    def test_basic_float64_nan_sentinel(self):
        """Basic float64 nan sentinel."""
        arrays = np.array(
            [np.array([1.5], dtype=np.float64), np.array([2.5, 3.5], dtype=np.float64)],
            dtype=object,
        )
        result = _pad_ragged_arrays(arrays)

        self.assertTrue(np.isnan(result[0, 1]))

    def test_custom_pad_value(self):
        """Custom pad value."""
        arrays = np.array(
            [np.array([1, 2], dtype=np.int32), np.array([3, 4, 5], dtype=np.int32)],
            dtype=object,
        )
        result = _pad_ragged_arrays(arrays, ragged_fill_value=0)

        np.testing.assert_array_equal(result[0], [1, 2, 0])

    def test_rejects_non_ndarray_elements(self):
        """Rejects non ndarray elements."""
        arrays = np.array([{"key": "value"}, [1, 2, 3]], dtype=object)

        with self.assertRaises(ValueError):
            _pad_ragged_arrays(arrays)

    def test_rejects_string_dtype(self):
        """Rejects string dtype."""
        arrays = np.empty(1, dtype=object)
        arrays[0] = np.array(["hello", "world"], dtype="U10")

        with self.assertRaises(ValueError):
            _pad_ragged_arrays(arrays)

    def test_rejects_empty_input(self):
        """Rejects empty input."""
        with self.assertRaises(ValueError):
            _pad_ragged_arrays(np.array([], dtype=object))

    def test_all_sub_arrays_empty_raises(self):
        """All sub arrays empty raises."""
        arrays = np.empty(2, dtype=object)
        arrays[0] = np.empty(0, dtype=object)
        arrays[1] = np.empty(0, dtype=object)

        with self.assertRaises(ValueError):
            _pad_ragged_arrays(arrays)


class TorchBatchPredictorTests(unittest.TestCase):
    """Tests for torch batch predictor."""

    def test_init_requires_model(self):
        """Init requires model."""
        with self.assertRaises(ValueError):
            TorchBatchPredictor(model=None)

    def test_data_processor_lazy_loading(self):
        """Data processor lazy loading."""
        model = DummyModel()
        predictor = TorchBatchPredictor(model=model)
        self.assertIsNone(predictor._data_processor)
        processor = predictor.data_processor
        self.assertIs(predictor.data_processor, processor)

    def test_call_assembles_native_arrow_output_table(self):
        """Call assembles native arrow output table."""
        model_outputs = {
            "predictions": torch.tensor([[0.1], [0.9]], dtype=torch.float32)
        }
        model = DummyModel(outputs=model_outputs)
        predictor = TorchBatchPredictor(
            model=model, input_columns=["x"], output_columns=["predictions"]
        )
        input_table = pa.table(
            {"x": pa.array([[5.0], [6.0]], type=pa.list_(pa.float32()))}
        )

        result = predictor(input_table)

        self.assertIn("predictions", result.column_names)
        self.assertIn("x", result.column_names)

    def test_call_passthrough_columns_not_materialized_to_numpy(self):
        """Call passthrough columns not materialized to numpy."""
        model_outputs = {
            "predictions": torch.tensor([[0.1], [0.9]], dtype=torch.float32)
        }
        model = DummyModel(outputs=model_outputs)
        predictor = TorchBatchPredictor(
            model=model, input_columns=["x"], output_columns=["predictions"]
        )
        input_table = pa.table(
            {
                "x": pa.array([[5.0], [6.0]], type=pa.list_(pa.float32())),
                "label": pa.array(["alpha", "beta"], type=pa.large_string()),
            }
        )

        result = predictor(input_table)

        self.assertEqual(result.schema.field("label").type, pa.large_string())
        self.assertEqual(set(model.last_inputs.keys()), {"x"})

    def test_call_only_input_columns_reach_model(self):
        """Call only input columns reach model."""
        model_outputs = {
            "predictions": torch.tensor([[0.1], [0.9]], dtype=torch.float32)
        }
        model = DummyModel(outputs=model_outputs)
        predictor = TorchBatchPredictor(
            model=model, input_columns=["x"], output_columns=["predictions"]
        )
        input_table = pa.table(
            {
                "x": pa.array([[5.0], [6.0]], type=pa.list_(pa.float32())),
                "y": pa.array([[1.0], [2.0]], type=pa.list_(pa.float32())),
            }
        )

        result = predictor(input_table)

        self.assertEqual(set(model.last_inputs.keys()), {"x"})
        self.assertEqual(result.column("y").to_pylist(), [[1.0], [2.0]])

    def test_call_exception_handling(self):
        """Call exception handling."""
        model = DummyModel(raise_error=True)
        predictor = TorchBatchPredictor(model=model, input_columns=["x"])
        input_table = pa.table({"x": pa.array([[1.0]], type=pa.list_(pa.float32()))})

        with self.assertRaises(ValueError):
            predictor(input_table)

    def test_inference_mode_context(self):
        """Inference mode context."""
        captured = {}

        def model(inputs):
            captured["grad_enabled"] = torch.is_grad_enabled()
            return {"y": torch.zeros(len(next(iter(inputs.values()))))}

        predictor = TorchBatchPredictor(
            model=model, input_columns=["x"], output_columns=["y"]
        )
        input_table = pa.table({"x": pa.array([1.0, 2.0], type=pa.float32())})

        predictor(input_table)

        self.assertFalse(captured["grad_enabled"])

    def test_model_kwargs_passed_to_model(self):
        """Model kwargs passed to model."""
        captured = {}

        def model(inputs, scale=1.0):
            captured["scale"] = scale
            return {"y": torch.zeros(len(next(iter(inputs.values()))))}

        predictor = TorchBatchPredictor(
            model=model,
            input_columns=["x"],
            output_columns=["y"],
            model_kwargs={"scale": 2.0},
        )
        input_table = pa.table({"x": pa.array([1.0], type=pa.float32())})

        predictor(input_table)

        self.assertEqual(captured["scale"], 2.0)

    def test_model_called_without_kwargs_when_not_provided(self):
        """Model called without kwargs when not provided."""
        calls = []

        def model(inputs):
            calls.append(inputs)
            return {"y": torch.zeros(len(next(iter(inputs.values()))))}

        predictor = TorchBatchPredictor(
            model=model, input_columns=["x"], output_columns=["y"]
        )
        input_table = pa.table({"x": pa.array([1.0], type=pa.float32())})

        predictor(input_table)

        self.assertEqual(len(calls), 1)


if __name__ == "__main__":
    unittest.main()
