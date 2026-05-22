"""Tests for workflow variable types: ModelArtifact, AssembledModel, PusherResult."""

from __future__ import annotations

import sys
import types as _types
from dataclasses import dataclass
from io import BytesIO
from unittest import TestCase
from unittest.mock import patch

import pandas as pd

from michelangelo.workflow.variables._private.dataset import DatasetVariable
from michelangelo.workflow.variables.metadata import ModelMetadata
from michelangelo.workflow.variables.types import (
    AssembledModel,
    ModelArtifact,
    PusherResult,
)


class TestModelMetadata(TestCase):
    """Tests for ModelMetadata."""

    def test_defaults_all_none_or_false(self):
        """It initialises with all fields at their defaults."""
        meta = ModelMetadata()
        self.assertIsNone(meta.training_framework)
        self.assertIsNone(meta.model_class)
        self.assertFalse(meta.assembled)
        self.assertFalse(meta.deployable)
        self.assertIsNone(meta._schema)
        self.assertIsNone(meta._sample_data)
        self.assertIsNone(meta._hyperparameters)

    def test_stores_training_framework(self):
        """It stores the provided training_framework."""
        meta = ModelMetadata(training_framework="xgboost")
        self.assertEqual(meta.training_framework, "xgboost")

    def test_stores_model_class(self):
        """It stores the provided model_class import path."""
        meta = ModelMetadata(model_class="mypackage.models.Clf")
        self.assertEqual(meta.model_class, "mypackage.models.Clf")

    def test_stores_assembled_and_deployable_flags(self):
        """It stores assembled and deployable boolean flags."""
        meta = ModelMetadata(assembled=True, deployable=True)
        self.assertTrue(meta.assembled)
        self.assertTrue(meta.deployable)

    def test_stores_binary_payloads(self):
        """It stores schema, sample_data, and hyperparameters as BytesIO."""
        schema = BytesIO(b"schema-bytes")
        sample = BytesIO(b"sample-bytes")
        hparams = BytesIO(b"hparam-bytes")
        meta = ModelMetadata(
            _schema=schema,
            _sample_data=sample,
            _hyperparameters=hparams,
        )
        self.assertEqual(meta._schema.read(), b"schema-bytes")
        self.assertEqual(meta._sample_data.read(), b"sample-bytes")
        self.assertEqual(meta._hyperparameters.read(), b"hparam-bytes")

    def test_is_subclassable(self):
        """A subclass can add provider-specific fields."""

        @dataclass
        class UberModelMetadata(ModelMetadata):
            training_job_id: str | None = None

        uber_meta = UberModelMetadata(
            training_framework="pytorch",
            training_job_id="job-1",
        )
        self.assertEqual(uber_meta.training_framework, "pytorch")
        self.assertEqual(uber_meta.training_job_id, "job-1")
        self.assertIsInstance(uber_meta, ModelMetadata)


class TestModelArtifact(TestCase):
    """Tests for ModelArtifact."""

    def test_stores_path(self):
        """It stores the provided path."""
        artifact = ModelArtifact(path="/tmp/model")
        self.assertEqual(artifact.path, "/tmp/model")

    def test_metadata_defaults_to_empty_model_metadata(self):
        """It defaults metadata to a ModelMetadata instance with defaults."""
        artifact = ModelArtifact(path="/tmp/model")
        self.assertIsInstance(artifact.metadata, ModelMetadata)
        self.assertIsNone(artifact.metadata.training_framework)

    def test_metadata_instances_are_independent(self):
        """It creates a separate ModelMetadata instance for each artifact."""
        a = ModelArtifact(path="/tmp/a")
        b = ModelArtifact(path="/tmp/b")
        a.metadata.training_framework = "pytorch"
        self.assertIsNone(b.metadata.training_framework)

    def test_metadata_can_be_provided(self):
        """It stores an explicitly provided ModelMetadata."""
        meta = ModelMetadata(training_framework="xgboost", deployable=True)
        artifact = ModelArtifact(path="/tmp/m", metadata=meta)
        self.assertEqual(artifact.metadata.training_framework, "xgboost")
        self.assertTrue(artifact.metadata.deployable)

    def test_metadata_accepts_subclass(self):
        """It stores a ModelMetadata subclass without modification."""

        @dataclass
        class UberModelMetadata(ModelMetadata):
            training_job_id: str | None = None

        uber_meta = UberModelMetadata(
            training_framework="huggingface",
            training_job_id="j-42",
        )
        artifact = ModelArtifact(path="/tmp/m", metadata=uber_meta)
        self.assertIsInstance(artifact.metadata, ModelMetadata)
        # type: ignore[attr-defined]
        self.assertEqual(artifact.metadata.training_job_id, "j-42")  # type: ignore[attr-defined]


class TestAssembledModel(TestCase):
    """Tests for AssembledModel."""

    def _make_artifact(self, path: str = "/tmp/model") -> ModelArtifact:
        return ModelArtifact(path=path)

    def test_stores_raw_and_deployable_models(self):
        """It stores both raw_model and deployable_model."""
        raw = self._make_artifact("/tmp/raw")
        deployable = self._make_artifact("/tmp/deployable")
        model = AssembledModel(raw_model=raw, deployable_model=deployable)
        self.assertEqual(model.raw_model.path, "/tmp/raw")
        self.assertEqual(model.deployable_model.path, "/tmp/deployable")


class TestPusherResult(TestCase):
    """Tests for PusherResult."""

    def test_successful_result_fields(self):
        """It stores name, plugin, success, and value for a successful result."""
        result = PusherResult(
            name="model",
            plugin="model_plugin",
            success=True,
            value={"model_name": "clf", "version": "1"},
        )
        self.assertEqual(result.name, "model")
        self.assertEqual(result.plugin, "model_plugin")
        self.assertTrue(result.success)
        self.assertEqual(result.value["version"], "1")

    def test_failed_result_fields(self):
        """It stores error message and empty value for a failed result."""
        result = PusherResult(
            name="model",
            plugin="model_plugin",
            success=False,
            value={},
            error="Upload failed.",
        )
        self.assertFalse(result.success)
        self.assertEqual(result.error, "Upload failed.")

    def test_value_defaults_to_empty_dict(self):
        """It defaults value to an empty dict."""
        result = PusherResult(name="r", plugin="p", success=True)
        self.assertEqual(result.value, {})

    def test_value_instances_are_independent(self):
        """It creates a separate value dict for each instance."""
        a = PusherResult(name="a", plugin="p", success=True)
        b = PusherResult(name="b", plugin="p", success=True)
        a.value["key"] = "v"
        self.assertEqual(b.value, {})

    def test_error_defaults_to_none(self):
        """It defaults error to None."""
        result = PusherResult(name="r", plugin="p", success=True)
        self.assertIsNone(result.error)


# ---------------------------------------------------------------------------
# Helpers for mocking optional backends (pyspark / ray not installed)
# ---------------------------------------------------------------------------

def _mock_pyspark(df_instance=None):
    """Return (mock_pyspark_sql_module, mock_spark_df) for sys.modules patching."""
    mock_df_class = type("DataFrame", (), {})
    mock_sql = _types.SimpleNamespace(DataFrame=mock_df_class)
    spark_df = df_instance if df_instance is not None else mock_df_class()
    return mock_sql, spark_df


def _mock_ray(ds_instance=None):
    """Return (mock_ray_data_module, mock_ray_dataset) for sys.modules patching."""
    mock_dataset_class = type("Dataset", (), {})
    mock_data = _types.SimpleNamespace(Dataset=mock_dataset_class)
    ray_ds = ds_instance if ds_instance is not None else mock_dataset_class()
    return mock_data, ray_ds


def _spark_mods(mock_sql):
    """sys.modules patch dict for pyspark."""
    return {"pyspark": _types.SimpleNamespace(sql=mock_sql), "pyspark.sql": mock_sql}


def _ray_mods(mock_data):
    """sys.modules patch dict for ray.data."""
    return {"ray": _types.SimpleNamespace(data=mock_data), "ray.data": mock_data}


class TestDatasetVariablePandas(TestCase):
    """Tests for DatasetVariable with a pandas DataFrame value."""

    def test_stores_dataframe(self):
        """DatasetVariable(value=df) stores the DataFrame in .value."""
        df = pd.DataFrame([{"x": 1}])
        artifact = DatasetVariable(value=df)
        self.assertIs(artifact.value, df)

    def test_backend_is_pandas(self):
        """It reports 'pandas' as the backend."""
        artifact = DatasetVariable(value=pd.DataFrame())
        self.assertEqual(artifact.backend, "pandas")

    def test_create_factory(self):
        """DatasetVariable.create(value) is equivalent to DatasetVariable(value=...)."""
        df = pd.DataFrame([{"x": 1}])
        artifact = DatasetVariable.create(df)
        self.assertIs(artifact.value, df)
        self.assertEqual(artifact.backend, "pandas")

    def test_path_auto_generated(self):
        """A memory:// path is generated when none is provided."""
        artifact = DatasetVariable(value=pd.DataFrame())
        self.assertTrue(artifact.path.startswith("memory://"))

    def test_custom_path_stored(self):
        """An explicitly provided path is stored as-is."""
        artifact = DatasetVariable(value=pd.DataFrame(), path="/tmp/mydata")
        self.assertEqual(artifact.path, "/tmp/mydata")

    def test_save_and_load_roundtrip(self):
        """save() persists to path; load_pandas_dataframe() restores the value."""
        import tempfile

        df = pd.DataFrame([{"name": "alice", "score": 0.9}])
        dest = tempfile.mkdtemp()
        artifact = DatasetVariable(value=df, path=dest)
        artifact.save()
        restored = DatasetVariable(path=dest)
        restored.load_pandas_dataframe()
        self.assertEqual(len(restored.value), len(df))
        self.assertEqual(restored.value["name"].tolist(), df["name"].tolist())

    def test_lazy_load_on_value_access(self):
        """Accessing .value on a path-only artifact triggers _load() → load_pandas."""
        import tempfile

        df = pd.DataFrame([{"x": 42}])
        dest = tempfile.mkdtemp()
        DatasetVariable(value=df, path=dest).save()
        artifact = DatasetVariable(path=dest)
        # No explicit load — lazy via value property
        self.assertEqual(artifact.value["x"].tolist(), [42])

    def test_save_raises_for_unsupported_type(self):
        """save() raises TypeError for an unrecognised value type."""
        artifact = DatasetVariable(value={"not": "a dataframe"})
        with self.assertRaises(TypeError):
            artifact.save()

    def test_save_raises_value_error_when_no_value_set(self):
        """save() raises ValueError when _value is None (path-only artifact)."""
        artifact = DatasetVariable(path="/tmp/no-value")
        with self.assertRaises(ValueError, msg="Cannot save"):
            artifact.save()

    def test_repr_shows_path_and_backend(self):
        """repr() includes path and backend for debugging."""
        df = pd.DataFrame([{"x": 1}])
        artifact = DatasetVariable(value=df, path="/tmp/mydata")
        r = repr(artifact)
        self.assertIn("/tmp/mydata", r)
        self.assertIn("pandas", r)

    def test_init_import_from_package(self):
        """DatasetVariable is importable from the package __init__."""
        from michelangelo.workflow import variables as _wv
        self.assertIs(_wv.DatasetVariable, DatasetVariable)


class TestDatasetVariableBackendSpark(TestCase):
    """Tests for DatasetVariable backend detection with Spark DataFrames."""

    def test_wraps_spark_dataframe(self):
        """DatasetVariable(value=spark_df) stores the value directly."""
        _, spark_df = _mock_pyspark()
        artifact = DatasetVariable(value=spark_df)
        self.assertIs(artifact.value, spark_df)

    def test_backend_is_spark(self):
        """It reports 'spark' when value is a pyspark.sql.DataFrame."""
        mock_sql, spark_df = _mock_pyspark()
        with patch.dict(sys.modules, _spark_mods(mock_sql)):
            artifact = DatasetVariable(value=spark_df)
            self.assertEqual(artifact.backend, "spark")

    def test_backend_unknown_when_pyspark_not_installed(self):
        """It returns 'unknown' when pyspark is absent and value is unrecognised."""
        artifact = DatasetVariable(value=object())
        with patch.dict(sys.modules, {"pyspark": None, "pyspark.sql": None,
                                      "ray": None, "ray.data": None}):
            self.assertEqual(artifact.backend, "unknown")


class TestDatasetVariableBackendRay(TestCase):
    """Tests for DatasetVariable backend detection with Ray Datasets."""

    def test_wraps_ray_dataset(self):
        """DatasetVariable(value=ray_ds) stores the value directly."""
        _, ray_ds = _mock_ray()
        artifact = DatasetVariable(value=ray_ds)
        self.assertIs(artifact.value, ray_ds)

    def test_backend_is_ray(self):
        """It reports 'ray' when value is a ray.data.Dataset."""
        mock_data, ray_ds = _mock_ray()
        with patch.dict(sys.modules, _ray_mods(mock_data)):
            artifact = DatasetVariable(value=ray_ds)
            self.assertEqual(artifact.backend, "ray")


class TestDatasetVariableSaveLoadSparkRay(TestCase):
    """Tests for DatasetVariable save/load dispatch to Spark and Ray IO."""

    def test_save_dispatches_to_spark(self):
        """save() calls save_spark_dataframe() for a Spark DataFrame value."""
        mock_sql, spark_df = _mock_pyspark()
        artifact = DatasetVariable(value=spark_df)
        with patch.dict(sys.modules, _spark_mods(mock_sql)), \
                patch.object(artifact, "save_spark_dataframe") as mock_save:
            artifact.save()
            mock_save.assert_called_once()

    def test_save_dispatches_to_ray(self):
        """save() calls save_ray_dataset() for a Ray Dataset value."""
        mock_data, ray_ds = _mock_ray()
        artifact = DatasetVariable(value=ray_ds)
        with patch.dict(sys.modules, _ray_mods(mock_data)), \
                patch.object(artifact, "save_ray_dataset") as mock_save:
            artifact.save()
            mock_save.assert_called_once()

    def test_save_spark_dataframe_uses_io(self):
        """save_spark_dataframe() calls _save_value_using_io with SparkIO."""
        artifact = DatasetVariable(value=object(), path="/tmp/spark-out")
        mock_spark_io = type("SparkIO", (), {})
        mock_module = _types.SimpleNamespace(SparkIO=mock_spark_io)
        spark_patch = {"michelangelo.uniflow.plugins.spark.io": mock_module}
        with patch.dict(sys.modules, spark_patch), \
                patch.object(artifact, "_save_value_using_io") as mock_io:
            artifact.save_spark_dataframe()
            mock_io.assert_called_once_with(mock_spark_io)

    def test_save_ray_dataset_uses_io(self):
        """save_ray_dataset() calls _save_value_using_io with RayDatasetIO."""
        artifact = DatasetVariable(value=object(), path="/tmp/ray-out")
        mock_ray_io = type("RayDatasetIO", (), {})
        mock_module = _types.SimpleNamespace(RayDatasetIO=mock_ray_io)
        ray_patch = {"michelangelo.uniflow.plugins.ray.io": mock_module}
        with patch.dict(sys.modules, ray_patch), \
                patch.object(artifact, "_save_value_using_io") as mock_io:
            artifact.save_ray_dataset()
            mock_io.assert_called_once_with(mock_ray_io)

    def test_load_spark_dataframe_uses_io(self):
        """load_spark_dataframe() calls _load_value_using_io with SparkIO."""
        artifact = DatasetVariable(path="/tmp/spark-in")
        mock_spark_io = type("SparkIO", (), {})
        mock_module = _types.SimpleNamespace(SparkIO=mock_spark_io)
        spark_patch = {"michelangelo.uniflow.plugins.spark.io": mock_module}
        with patch.dict(sys.modules, spark_patch), \
                patch.object(artifact, "_load_value_using_io") as mock_io:
            artifact.load_spark_dataframe()
            mock_io.assert_called_once_with(mock_spark_io)

    def test_load_ray_dataset_uses_io(self):
        """load_ray_dataset() calls _load_value_using_io with RayDatasetIO."""
        artifact = DatasetVariable(path="/tmp/ray-in")
        mock_ray_io = type("RayDatasetIO", (), {})
        mock_module = _types.SimpleNamespace(RayDatasetIO=mock_ray_io)
        ray_patch = {"michelangelo.uniflow.plugins.ray.io": mock_module}
        with patch.dict(sys.modules, ray_patch), \
                patch.object(artifact, "_load_value_using_io") as mock_io:
            artifact.load_ray_dataset()
            mock_io.assert_called_once_with(mock_ray_io)

    def test_load_dispatches_to_spark_when_session_active(self):
        """_load() calls load_spark_dataframe() when a Spark session is active."""
        artifact = DatasetVariable(path="/tmp/spark-lazy")
        mock_sql = _types.SimpleNamespace(
            SparkSession=_types.SimpleNamespace(
                getActiveSession=lambda: object()  # non-None → active session
            )
        )
        with patch.dict(sys.modules, _spark_mods(mock_sql)), \
                patch.object(artifact, "load_spark_dataframe") as mock_load:
            artifact._load()
            mock_load.assert_called_once()

    def test_load_dispatches_to_ray_when_initialized(self):
        """_load() calls load_ray_dataset() when Ray is initialized."""
        artifact = DatasetVariable(path="/tmp/ray-lazy")
        mock_ray = _types.SimpleNamespace(is_initialized=lambda: True, data=object())
        ray_patch = {"pyspark": None, "pyspark.sql": None, "ray": mock_ray}
        with patch.dict(sys.modules, ray_patch), \
                patch.object(artifact, "load_ray_dataset") as mock_load:
            artifact._load()
            mock_load.assert_called_once()

