"""Tests for michelangelo.workflow.tasks.tabular_native_transform.task."""

from __future__ import annotations

from unittest import TestCase
from unittest.mock import patch

import pytest

ray = pytest.importorskip("ray")
pytest.importorskip("torch")
pytest.importorskip("pydantic")

from michelangelo.lib.artifact_manager.storage_backend import (  # noqa: E402
    LocalStorageBackend,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError  # noqa: E402
from michelangelo.workflow.schema.ray_data_io import RayDataContextConfig  # noqa: E402
from michelangelo.workflow.schema.tabular_native_transform import (  # noqa: E402
    BatchOptions,
    IncrementalTrainingConfig,
    TabularNativeTransformConfig,
    TrainingType,
)
from michelangelo.workflow.tasks.tabular_native_transform._private import (  # noqa: E402
    incremental_training,
)
from michelangelo.workflow.tasks.tabular_native_transform.task import (  # noqa: E402
    _create_transform_model,
    _load_transform_spec,
    _save_datasets,
    _validate_datasets,
    tabular_native_transform,
)
from michelangelo.workflow.variables import DatasetVariable  # noqa: E402
from michelangelo.workflow.variables.types import NativeTransformResult  # noqa: E402

_TASK = "michelangelo.workflow.tasks.tabular_native_transform.task"

_SIMPLE_SPEC = {
    "transform_specs": [
        {
            "transform_name": "Concatenate",
            "input_cols": ["feature1"],
            "output_cols": ["feature1_out"],
        },
    ]
}


def _ray_dataset(rows):
    """Build a small in-memory Ray Dataset from a list of row dicts."""
    return ray.data.from_items(rows)


def setup_module(_module):
    """Start one Ray instance for every test in this module.

    Several test classes below call ``ray.data.from_items``, which silently
    auto-starts a local Ray instance if one isn't already running. Without a
    single shared init/shutdown for the whole module, a class that never
    explicitly shuts Ray down leaks a live Ray instance into whichever test
    module runs next in the same pytest process (observed against
    ``workflow/variables/tests/variables_test.py``, which dispatches
    ``DatasetVariable._load()`` differently depending on
    ``ray.is_initialized()``).
    """
    if not ray.is_initialized():
        ray.init(ignore_reinit_error=True, num_cpus=1)


def teardown_module(_module):
    """Shut down the module-shared Ray instance started by setup_module."""
    ray.shutdown()


class TabularNativeTransformTaskTest(TestCase):
    """Tests for tabular_native_transform (the top-level task function)."""

    def setUp(self):
        """Build a small two-row fixture reused across tests."""
        self.rows = [
            {"feature1": 1.0, "feature2": 2.0},
            {"feature1": 2.0, "feature2": 3.0},
        ]

    # -- set_ray_data_context wiring -----------------------------------

    @patch(f"{_TASK}.set_ray_data_context")
    def test_calls_set_ray_data_context_with_defaults(self, mock_set_context):
        """set_ray_data_context is called with None defaults when unset."""
        train = DatasetVariable.create(None)
        config = TabularNativeTransformConfig(transform_spec=None)
        tabular_native_transform(config, {"train": train})
        mock_set_context.assert_called_once_with(
            min_block_size=None, max_block_size=None, retried_io_errors=None
        )

    @patch(f"{_TASK}.set_ray_data_context")
    def test_forwards_ray_data_context_config(self, mock_set_context):
        """config.ray_data_context values are forwarded to set_ray_data_context."""
        train = DatasetVariable.create(None)
        config = TabularNativeTransformConfig(
            transform_spec=None,
            ray_data_context=RayDataContextConfig(
                min_block_size=1024, max_block_size=2048, retried_io_errors=["oops"]
            ),
        )
        tabular_native_transform(config, {"train": train})
        mock_set_context.assert_called_once_with(
            min_block_size=1024, max_block_size=2048, retried_io_errors=["oops"]
        )

    # -- early return ----------------------------------------------------

    def test_no_transform_spec_returns_datasets_unchanged(self):
        """With no transform_spec and no incremental mode, datasets pass through."""
        train = DatasetVariable.create(None)
        config = TabularNativeTransformConfig(transform_spec=None)
        result = tabular_native_transform(config, {"train": train})
        self.assertIsInstance(result, NativeTransformResult)
        self.assertIs(result.transformed_datasets["train"], train)
        self.assertIsNone(result.model)

    # -- validation --------------------------------------------------------

    def test_validate_datasets_raises_for_empty_dict(self):
        """An empty datasets dict raises ConfigurationError."""
        with self.assertRaises(ConfigurationError):
            _validate_datasets({})

    def test_validate_datasets_raises_when_all_empty(self):
        """A dict of only empty datasets raises ConfigurationError."""
        empty = DatasetVariable.create(_ray_dataset([]))
        with self.assertRaises(ConfigurationError):
            _validate_datasets({"train": empty})

    def test_validate_datasets_passes_with_one_non_empty(self):
        """At least one non-empty dataset satisfies validation."""
        empty = DatasetVariable.create(_ray_dataset([]))
        non_empty = DatasetVariable.create(_ray_dataset(self.rows))
        _validate_datasets({"train": non_empty, "test": empty})  # no raise

    # -- _load_transform_spec ----------------------------------------------

    def test_load_transform_spec_from_dict(self):
        """An inlined dict transform_spec is parsed into a TransformSpec."""
        config = TabularNativeTransformConfig(transform_spec=dict(_SIMPLE_SPEC))
        spec = _load_transform_spec(config)
        self.assertEqual(len(spec.transform_specs), 1)

    def test_load_transform_spec_invalid_type_raises(self):
        """A transform_spec that is neither dict nor str raises ConfigurationError."""
        config = TabularNativeTransformConfig(transform_spec=123)  # type: ignore[arg-type]
        with self.assertRaises(ConfigurationError):
            _load_transform_spec(config)

    def test_load_transform_spec_from_yaml_file_path(self):
        """A string transform_spec is resolved and loaded as a YAML file path."""
        import tempfile

        import yaml

        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".yaml", delete=False
        ) as tmp_file:
            yaml.safe_dump(_SIMPLE_SPEC, tmp_file)
            tmp_path = tmp_file.name
        config = TabularNativeTransformConfig(transform_spec=tmp_path)
        spec = _load_transform_spec(config)
        self.assertEqual(len(spec.transform_specs), 1)

    # -- incremental mode requires storage_backend --------------------------

    def test_incremental_without_storage_backend_raises(self):
        """INCREMENTAL mode without a storage_backend raises ConfigurationError."""
        train = DatasetVariable.create(_ray_dataset(self.rows))
        config = TabularNativeTransformConfig(
            transform_spec=None,
            incremental_training=IncrementalTrainingConfig(
                training_type=TrainingType.INCREMENTAL,
                baseline_model_uri="file:///does/not/matter",
            ),
        )
        with self.assertRaises(ConfigurationError):
            tabular_native_transform(config, {"train": train})

    # -- incremental mode end-to-end -----------------------------------

    @patch.object(DatasetVariable, "save_ray_dataset")
    @patch(f"{_TASK}.native_transform")
    @patch.object(incremental_training, "load_incremental_artifacts")
    def test_incremental_with_full_reuse_uses_base_spec_and_stats(
        self, mock_load_artifacts, mock_transform, _mock_save
    ):
        """enforce_full_reuse=True (default) reuses the base run's spec and stats."""
        train = DatasetVariable.create(_ray_dataset(self.rows))
        base_spec = _load_transform_spec(
            TabularNativeTransformConfig(transform_spec=dict(_SIMPLE_SPEC))
        )
        mock_load_artifacts.return_value = (base_spec, {"feature1_out_mean": 1.0})
        mock_transform.return_value = (_ray_dataset(self.rows), base_spec, {})

        config = TabularNativeTransformConfig(
            transform_spec=None,
            incremental_training=IncrementalTrainingConfig(
                training_type=TrainingType.INCREMENTAL,
                baseline_model_uri="file:///does/not/matter",
            ),
        )
        result = tabular_native_transform(
            config,
            {"train": train},
            storage_backend=LocalStorageBackend(base_dir="/tmp"),
        )
        mock_load_artifacts.assert_called_once()
        self.assertIsNotNone(result.model)

    @patch.object(DatasetVariable, "save_ray_dataset")
    @patch(f"{_TASK}.native_transform")
    @patch.object(incremental_training, "merge_specs_for_selective_refit")
    @patch.object(incremental_training, "load_incremental_artifacts")
    def test_incremental_without_full_reuse_merges_config_spec(
        self, mock_load_artifacts, mock_merge, mock_transform, _mock_save
    ):
        """enforce_full_reuse=False loads the config spec and merges it for refit."""
        train = DatasetVariable.create(_ray_dataset(self.rows))
        base_spec = _load_transform_spec(
            TabularNativeTransformConfig(transform_spec=dict(_SIMPLE_SPEC))
        )
        mock_load_artifacts.return_value = (base_spec, {"feature1_out_mean": 1.0})
        mock_merge.return_value = (base_spec, {})
        mock_transform.return_value = (_ray_dataset(self.rows), base_spec, {})

        config = TabularNativeTransformConfig(
            transform_spec=dict(_SIMPLE_SPEC),
            incremental_training=IncrementalTrainingConfig(
                training_type=TrainingType.INCREMENTAL,
                baseline_model_uri="file:///does/not/matter",
                enforce_full_reuse=False,
            ),
        )
        result = tabular_native_transform(
            config,
            {"train": train},
            storage_backend=LocalStorageBackend(base_dir="/tmp"),
        )
        mock_merge.assert_called_once()
        self.assertIsNotNone(result.model)

    @patch.object(DatasetVariable, "save_ray_dataset")
    @patch(f"{_TASK}.get_transform_module")
    @patch(f"{_TASK}.native_transform")
    @patch.object(incremental_training, "load_incremental_artifacts")
    def test_incremental_with_no_model_raises(
        self, mock_load_artifacts, mock_transform, mock_get_module, _mock_save
    ):
        """INCREMENTAL mode yielding no transform module raises ConfigurationError."""
        train = DatasetVariable.create(_ray_dataset(self.rows))
        base_spec = _load_transform_spec(
            TabularNativeTransformConfig(transform_spec=dict(_SIMPLE_SPEC))
        )
        mock_load_artifacts.return_value = (base_spec, {})
        mock_transform.return_value = (_ray_dataset(self.rows), base_spec, {})
        mock_get_module.return_value = None

        config = TabularNativeTransformConfig(
            transform_spec=None,
            incremental_training=IncrementalTrainingConfig(
                training_type=TrainingType.INCREMENTAL,
                baseline_model_uri="file:///does/not/matter",
            ),
        )
        with self.assertRaises(ConfigurationError):
            tabular_native_transform(
                config,
                {"train": train},
                storage_backend=LocalStorageBackend(base_dir="/tmp"),
            )

    # -- end-to-end happy path ------------------------------------------

    @patch.object(DatasetVariable, "save_ray_dataset")
    @patch(f"{_TASK}.native_transform")
    def test_happy_path_produces_model_and_transformed_datasets(
        self, mock_transform, _mock_save
    ):
        """A full run with a real spec produces a model and derived features."""
        train = DatasetVariable.create(_ray_dataset(self.rows))
        mock_transform.return_value = (
            _ray_dataset(self.rows),
            _load_transform_spec(
                TabularNativeTransformConfig(transform_spec=dict(_SIMPLE_SPEC))
            ),
            {},
        )
        config = TabularNativeTransformConfig(
            transform_spec=dict(_SIMPLE_SPEC),
            batch_options=BatchOptions(batch_size=10),
        )
        result = tabular_native_transform(config, {"train": train})
        self.assertIn("train", result.transformed_datasets)
        self.assertIsNotNone(result.model)
        self.assertEqual(
            result.transformed_datasets["train"].metadata.derived_features,
            ["feature1_out"],
        )

    @patch.object(DatasetVariable, "save_ray_dataset")
    @patch(f"{_TASK}.native_transform")
    def test_columns_to_keep_used_for_derived_features(
        self, mock_transform, _mock_save
    ):
        """When the spec sets columns_to_keep, derived features come from it."""
        train = DatasetVariable.create(_ray_dataset(self.rows))
        spec_with_keep = dict(_SIMPLE_SPEC)
        spec_with_keep["columns_to_keep"] = ["feature1_out"]
        mock_transform.return_value = (
            _ray_dataset(self.rows),
            _load_transform_spec(
                TabularNativeTransformConfig(transform_spec=spec_with_keep)
            ),
            {},
        )
        config = TabularNativeTransformConfig(transform_spec=spec_with_keep)
        result = tabular_native_transform(config, {"train": train})
        self.assertEqual(
            result.transformed_datasets["train"].metadata.derived_features,
            ["feature1_out"],
        )

    @patch.object(DatasetVariable, "save_ray_dataset")
    @patch.object(DatasetVariable, "load_ray_dataset")
    @patch(f"{_TASK}.native_transform")
    def test_parquet_read_config_forwarded_to_load_ray_dataset(
        self, mock_transform, mock_load, _mock_save
    ):
        """config.parquet_read_config kwargs are forwarded to load_ray_dataset."""
        train = DatasetVariable.create(_ray_dataset(self.rows))
        mock_transform.return_value = (
            _ray_dataset(self.rows),
            _load_transform_spec(
                TabularNativeTransformConfig(transform_spec=dict(_SIMPLE_SPEC))
            ),
            {},
        )
        from michelangelo.workflow.schema.ray_data_io import ParquetReadConfig

        config = TabularNativeTransformConfig(
            transform_spec=dict(_SIMPLE_SPEC),
            parquet_read_config=ParquetReadConfig(concurrency=4),
        )
        tabular_native_transform(config, {"train": train})
        mock_load.assert_called_once_with(concurrency=4)

    @patch.object(DatasetVariable, "save_ray_dataset")
    @patch(f"{_TASK}.native_transform")
    def test_empty_dataset_passed_through_unchanged(self, mock_transform, _mock_save):
        """A dataset with no rows is passed through unchanged, not transformed."""
        train = DatasetVariable.create(_ray_dataset(self.rows))
        empty_val = DatasetVariable.create(_ray_dataset([]))
        mock_transform.return_value = (
            _ray_dataset(self.rows),
            _load_transform_spec(
                TabularNativeTransformConfig(transform_spec=dict(_SIMPLE_SPEC))
            ),
            {},
        )
        config = TabularNativeTransformConfig(transform_spec=dict(_SIMPLE_SPEC))
        result = tabular_native_transform(
            config, {"train": train, "validation": empty_val}
        )
        self.assertIs(result.transformed_datasets["validation"], empty_val)

    @patch.object(DatasetVariable, "save_ray_dataset")
    @patch(f"{_TASK}.native_transform")
    def test_write_config_forwarded_to_save_datasets(self, mock_transform, mock_save):
        """config.write_config kwargs are forwarded to save_ray_dataset."""
        train = DatasetVariable.create(_ray_dataset(self.rows))
        mock_transform.return_value = (
            _ray_dataset(self.rows),
            _load_transform_spec(
                TabularNativeTransformConfig(transform_spec=dict(_SIMPLE_SPEC))
            ),
            {},
        )
        from michelangelo.workflow.schema.ray_data_io import WriteConfig

        config = TabularNativeTransformConfig(
            transform_spec=dict(_SIMPLE_SPEC),
            write_config=WriteConfig(max_rows_per_file=100, min_rows_per_file=10),
        )
        tabular_native_transform(config, {"train": train})
        mock_save.assert_called_once_with(max_rows_per_file=100, min_rows_per_file=10)

    @patch.object(DatasetVariable, "save_ray_dataset")
    @patch(f"{_TASK}.get_transform_module")
    @patch(f"{_TASK}.native_transform")
    def test_no_transform_module_returns_none_model(
        self, mock_transform, mock_get_module, _mock_save
    ):
        """When get_transform_module returns None, the result has no model."""
        train = DatasetVariable.create(_ray_dataset(self.rows))
        mock_transform.return_value = (
            _ray_dataset(self.rows),
            _load_transform_spec(
                TabularNativeTransformConfig(transform_spec=dict(_SIMPLE_SPEC))
            ),
            {},
        )
        mock_get_module.return_value = None
        config = TabularNativeTransformConfig(transform_spec=dict(_SIMPLE_SPEC))
        result = tabular_native_transform(config, {"train": train})
        self.assertIsNone(result.model)


class CreateTransformModelTest(TestCase):
    """Tests for _create_transform_model."""

    def test_create_transform_model_populates_metadata(self):
        """A non-empty spec produces a ModelVariable with populated metadata."""
        spec = _load_transform_spec(
            TabularNativeTransformConfig(transform_spec=dict(_SIMPLE_SPEC))
        )
        model_variable = _create_transform_model(
            spec, feature_stats={}, sample_data={"feature1": 1.0}
        )
        self.assertIsNotNone(model_variable)
        self.assertEqual(model_variable.metadata.training_framework, "pytorch")
        self.assertIsNotNone(model_variable.metadata.transform_spec)
        self.assertEqual(model_variable.metadata.feature_stats, {})

    def test_create_transform_model_returns_none_for_empty_spec(self):
        """A spec with no transform layers produces no model."""
        spec = _load_transform_spec(
            TabularNativeTransformConfig(transform_spec={"transform_specs": []})
        )
        self.assertIsNone(_create_transform_model(spec, {}, None))


class SaveDatasetsTest(TestCase):
    """Tests for _save_datasets."""

    @patch.object(DatasetVariable, "save_ray_dataset")
    def test_save_datasets_writes_directly(self, mock_save):
        """Each dataset variable's save_ray_dataset is called with no kwargs."""
        var = DatasetVariable.create(ray.data.from_items([{"x": 1}]))
        _save_datasets({"train": var})
        mock_save.assert_called_once_with()

    @patch.object(DatasetVariable, "save_ray_dataset")
    def test_save_datasets_forwards_write_kwargs(self, mock_save):
        """Extra keyword arguments are forwarded to save_ray_dataset."""
        var = DatasetVariable.create(ray.data.from_items([{"x": 1}]))
        _save_datasets({"train": var}, max_rows_per_file=5)
        mock_save.assert_called_once_with(max_rows_per_file=5)
