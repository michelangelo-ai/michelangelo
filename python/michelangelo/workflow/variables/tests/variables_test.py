"""Tests for workflow variable types: ModelArtifact, AssembledModel, PusherResult."""

from __future__ import annotations

import sys
import types as _types
import unittest
from dataclasses import dataclass
from io import BytesIO
from unittest import TestCase
from unittest.mock import MagicMock, patch

import pandas as pd

from michelangelo.workflow.variables._private.dataset import DatasetVariable
from michelangelo.workflow.variables._private.model import ModelVariable
from michelangelo.workflow.variables.metadata import (
    TRAINING_FRAMEWORK_CUSTOM,
    TRAINING_FRAMEWORK_LIGHTNING,
    TRAINING_FRAMEWORK_PYTORCH,
    ModelMetadata,
)
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
        with patch.dict(
            sys.modules,
            {"pyspark": None, "pyspark.sql": None, "ray": None, "ray.data": None},
        ):
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
        with (
            patch.dict(sys.modules, _spark_mods(mock_sql)),
            patch.object(artifact, "save_spark_dataframe") as mock_save,
        ):
            artifact.save()
            mock_save.assert_called_once()

    def test_save_dispatches_to_ray(self):
        """save() calls save_ray_dataset() for a Ray Dataset value."""
        mock_data, ray_ds = _mock_ray()
        artifact = DatasetVariable(value=ray_ds)
        with (
            patch.dict(sys.modules, _ray_mods(mock_data)),
            patch.object(artifact, "save_ray_dataset") as mock_save,
        ):
            artifact.save()
            mock_save.assert_called_once()

    def test_save_spark_dataframe_uses_io(self):
        """save_spark_dataframe() calls _save_value_using_io with SparkIO."""
        artifact = DatasetVariable(value=object(), path="/tmp/spark-out")
        mock_spark_io = type("SparkIO", (), {})
        mock_module = _types.SimpleNamespace(SparkIO=mock_spark_io)
        spark_patch = {"michelangelo.uniflow.plugins.spark.io": mock_module}
        with (
            patch.dict(sys.modules, spark_patch),
            patch.object(artifact, "_save_value_using_io") as mock_io,
        ):
            artifact.save_spark_dataframe()
            mock_io.assert_called_once_with(mock_spark_io)

    def test_save_ray_dataset_uses_io(self):
        """save_ray_dataset() calls _save_value_using_io with RayDatasetIO."""
        artifact = DatasetVariable(value=object(), path="/tmp/ray-out")
        mock_ray_io = type("RayDatasetIO", (), {})
        mock_module = _types.SimpleNamespace(RayDatasetIO=mock_ray_io)
        ray_patch = {"michelangelo.uniflow.plugins.ray.io": mock_module}
        with (
            patch.dict(sys.modules, ray_patch),
            patch.object(artifact, "_save_value_using_io") as mock_io,
        ):
            artifact.save_ray_dataset()
            mock_io.assert_called_once_with(mock_ray_io)

    def test_load_spark_dataframe_uses_io(self):
        """load_spark_dataframe() calls _load_value_using_io with SparkIO."""
        artifact = DatasetVariable(path="/tmp/spark-in")
        mock_spark_io = type("SparkIO", (), {})
        mock_module = _types.SimpleNamespace(SparkIO=mock_spark_io)
        spark_patch = {"michelangelo.uniflow.plugins.spark.io": mock_module}
        with (
            patch.dict(sys.modules, spark_patch),
            patch.object(artifact, "_load_value_using_io") as mock_io,
        ):
            artifact.load_spark_dataframe()
            mock_io.assert_called_once_with(mock_spark_io)

    def test_load_ray_dataset_uses_io(self):
        """load_ray_dataset() calls _load_value_using_io with RayDatasetIO."""
        artifact = DatasetVariable(path="/tmp/ray-in")
        mock_ray_io = type("RayDatasetIO", (), {})
        mock_module = _types.SimpleNamespace(RayDatasetIO=mock_ray_io)
        ray_patch = {"michelangelo.uniflow.plugins.ray.io": mock_module}
        with (
            patch.dict(sys.modules, ray_patch),
            patch.object(artifact, "_load_value_using_io") as mock_io,
        ):
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
        with (
            patch.dict(sys.modules, _spark_mods(mock_sql)),
            patch.object(artifact, "load_spark_dataframe") as mock_load,
        ):
            artifact._load()
            mock_load.assert_called_once()

    def test_load_dispatches_to_ray_when_initialized(self):
        """_load() calls load_ray_dataset() when Ray is initialized."""
        artifact = DatasetVariable(path="/tmp/ray-lazy")
        mock_ray = _types.SimpleNamespace(is_initialized=lambda: True, data=object())
        ray_patch = {"pyspark": None, "pyspark.sql": None, "ray": mock_ray}
        with (
            patch.dict(sys.modules, ray_patch),
            patch.object(artifact, "load_ray_dataset") as mock_load,
        ):
            artifact._load()
            mock_load.assert_called_once()


# ---------------------------------------------------------------------------
# ModelVariable
# ---------------------------------------------------------------------------


_MODEL_PATH = "michelangelo.workflow.variables._private.model"


def _mock_torch(module_instance=None):
    """Return (mock_torch_module, mock_module_instance) for sys.modules patching.

    The fake torch module exposes a ``nn.Module`` class and stub ``save`` /
    ``load`` callables suitable for ``patch.object`` assertions.
    """
    mock_nn_module = type("Module", (), {})
    mock_nn = _types.SimpleNamespace(Module=mock_nn_module)
    instance = module_instance if module_instance is not None else mock_nn_module()
    mock_torch = _types.SimpleNamespace(
        nn=mock_nn,
        save=MagicMock(name="torch.save"),
        load=MagicMock(name="torch.load"),
    )
    return mock_torch, instance


def _mock_custom_model(instance=None):
    """Return (mock_module, mock_class, mock_instance) for sys.modules patching.

    The fake module mirrors
    ``michelangelo.lib.model_manager.interface.custom_model``: exposes a
    ``Model`` class that can be used for ``isinstance`` checks and as a
    ``model_class.load`` target.
    """
    mock_custom_class = type(
        "Model",
        (),
        {"save": MagicMock(name="Model.save"), "load": MagicMock(name="Model.load")},
    )
    mock_module = _types.SimpleNamespace(Model=mock_custom_class)
    inst = instance if instance is not None else mock_custom_class()
    return mock_module, mock_custom_class, inst


def _custom_mods(mock_module):
    """sys.modules patch dict for the custom_model interface module."""
    return {
        "michelangelo.lib.model_manager.interface.custom_model": mock_module,
    }


class TestModelMetadataConstantsAndHyperparameters(TestCase):
    """Tests for the TRAINING_FRAMEWORK_* constants and hyperparameters field."""

    def test_training_framework_constants_are_lowercase_strings(self):
        """The framework constants expose stable lowercase string values."""
        self.assertEqual(TRAINING_FRAMEWORK_CUSTOM, "custom")
        self.assertEqual(TRAINING_FRAMEWORK_PYTORCH, "pytorch")
        self.assertEqual(TRAINING_FRAMEWORK_LIGHTNING, "lightning")

    def test_hyperparameters_field_defaults_to_none(self):
        """ModelMetadata.hyperparameters defaults to None for backwards compat."""
        meta = ModelMetadata()
        self.assertIsNone(meta.hyperparameters)

    def test_hyperparameters_field_accepts_dict(self):
        """ModelMetadata stores a hyperparameters dict for Lightning loading."""
        meta = ModelMetadata(hyperparameters={"lr": 0.001, "batch_size": 32})
        self.assertEqual(meta.hyperparameters["lr"], 0.001)
        self.assertEqual(meta.hyperparameters["batch_size"], 32)


class TestModelVariableCreate(TestCase):
    """Tests for ModelVariable.create() auto-detection."""

    def test_create_with_non_model_leaves_framework_unset(self):
        """create() on a non-model object leaves training_framework=None."""
        var = ModelVariable.create("not-a-model")
        self.assertIsNone(var.metadata.training_framework)
        self.assertIsNone(var.metadata.model_class)

    def test_create_auto_generates_path(self):
        """create() auto-generates a memory:// path when UF_STORAGE_URL is unset."""
        var = ModelVariable.create("anything")
        self.assertTrue(var.path.startswith("memory://"))

    def test_create_attaches_fresh_model_metadata(self):
        """create() attaches a new ModelMetadata instance even for unknown types."""
        var = ModelVariable.create("anything")
        self.assertIsInstance(var.metadata, ModelMetadata)

    def test_create_detects_custom_model(self):
        """create() sets framework=custom when value is a CustomModel instance."""
        mock_module, mock_class, inst = _mock_custom_model()
        with patch.dict(sys.modules, _custom_mods(mock_module)):
            var = ModelVariable.create(inst)
        self.assertEqual(var.metadata.training_framework, TRAINING_FRAMEWORK_CUSTOM)
        self.assertEqual(
            var.metadata.model_class,
            f"{type(inst).__module__}.{type(inst).__name__}",
        )

    def test_create_detects_torch_module(self):
        """create() sets framework=pytorch when value is a torch.nn.Module instance."""
        mock_torch, inst = _mock_torch()
        with patch.dict(sys.modules, {"torch": mock_torch}):
            var = ModelVariable.create(inst)
        self.assertEqual(var.metadata.training_framework, TRAINING_FRAMEWORK_PYTORCH)
        self.assertEqual(
            var.metadata.model_class,
            f"{type(inst).__module__}.{type(inst).__name__}",
        )

    def test_create_skips_torch_check_when_torch_missing(self):
        """create() does not raise when torch is unavailable."""
        with patch.dict(sys.modules, {"torch": None}):
            var = ModelVariable.create(object())
        self.assertIsNone(var.metadata.training_framework)

    def test_create_falls_through_when_torch_importable_but_value_not_module(self):
        """create() returns unset framework when torch loads but value is non-Module."""
        mock_torch, _ = _mock_torch()
        with patch.dict(sys.modules, {"torch": mock_torch}):
            var = ModelVariable.create("not-a-module")
        self.assertIsNone(var.metadata.training_framework)
        self.assertIsNone(var.metadata.model_class)


class TestModelVariableSaveDispatch(TestCase):
    """Tests for ModelVariable.save() dispatch and pre-conditions."""

    def test_save_raises_when_no_value_set(self):
        """save() raises ValueError when _value is None (no value attached)."""
        var = ModelVariable(path="/tmp/no-value", metadata=ModelMetadata())
        with self.assertRaises(ValueError) as ctx:
            var.save()
        self.assertIn("no value has been set", str(ctx.exception))

    def test_save_raises_when_framework_unset(self):
        """save() raises ValueError when training_framework is None."""
        var = ModelVariable(path="/tmp/no-fw", metadata=ModelMetadata())
        var._value = object()
        with self.assertRaises(ValueError) as ctx:
            var.save()
        self.assertIn("Unrecognized training framework", str(ctx.exception))

    def test_save_raises_when_framework_unknown(self):
        """save() raises ValueError for an unrecognised framework string."""
        var = ModelVariable(
            path="/tmp/x", metadata=ModelMetadata(training_framework="tensorflow")
        )
        var._value = object()
        with self.assertRaises(ValueError):
            var.save()

    def test_save_dispatches_to_custom(self):
        """save() routes to save_custom_model when framework=custom."""
        var = ModelVariable(
            path="/tmp/x",
            metadata=ModelMetadata(training_framework=TRAINING_FRAMEWORK_CUSTOM),
        )
        var._value = object()
        with patch.object(var, "save_custom_model") as mock_save:
            var.save()
            mock_save.assert_called_once()

    def test_save_dispatches_to_torch(self):
        """save() routes to save_torch_model when framework=pytorch."""
        var = ModelVariable(
            path="/tmp/x",
            metadata=ModelMetadata(training_framework=TRAINING_FRAMEWORK_PYTORCH),
        )
        var._value = object()
        with patch.object(var, "save_torch_model") as mock_save:
            var.save()
            mock_save.assert_called_once()

    def test_save_dispatches_to_lightning(self):
        """save() routes to save_lightning_model when framework=lightning."""
        var = ModelVariable(
            path="/tmp/x",
            metadata=ModelMetadata(training_framework=TRAINING_FRAMEWORK_LIGHTNING),
        )
        var._value = object()
        with patch.object(var, "save_lightning_model") as mock_save:
            var.save()
            mock_save.assert_called_once()


class TestModelVariableLoadDispatch(TestCase):
    """Tests for ModelVariable._load() dispatch and pre-conditions."""

    def test_load_raises_when_framework_unset(self):
        """_load() raises ValueError when training_framework is None."""
        var = ModelVariable(path="/tmp/x", metadata=ModelMetadata())
        with self.assertRaises(ValueError):
            var._load()

    def test_load_dispatches_to_custom(self):
        """_load() routes to load_custom_model when framework=custom."""
        var = ModelVariable(
            path="/tmp/x",
            metadata=ModelMetadata(training_framework=TRAINING_FRAMEWORK_CUSTOM),
        )
        with patch.object(var, "load_custom_model") as mock_load:
            var._load()
            mock_load.assert_called_once()

    def test_load_dispatches_to_torch(self):
        """_load() routes to load_torch_model when framework=pytorch."""
        var = ModelVariable(
            path="/tmp/x",
            metadata=ModelMetadata(training_framework=TRAINING_FRAMEWORK_PYTORCH),
        )
        with patch.object(var, "load_torch_model") as mock_load:
            var._load()
            mock_load.assert_called_once()

    def test_load_dispatches_to_lightning(self):
        """_load() routes to load_lightning_model when framework=lightning."""
        var = ModelVariable(
            path="/tmp/x",
            metadata=ModelMetadata(training_framework=TRAINING_FRAMEWORK_LIGHTNING),
        )
        with patch.object(var, "load_lightning_model") as mock_load:
            var._load()
            mock_load.assert_called_once()


class TestModelVariableSkipWhenAlreadySaved(TestCase):
    """The save_* methods are idempotent — they no-op when _saved is True."""

    def test_save_custom_skips_when_already_saved(self):
        """save_custom_model() returns early without touching value or fs."""
        var = ModelVariable(path="memory://x", metadata=ModelMetadata())
        var._value = MagicMock(name="custom-model")
        var._saved = True
        with patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec:
            var.save_custom_model()
            mock_fsspec.core.url_to_fs.assert_not_called()
            var._value.save.assert_not_called()

    def test_save_torch_skips_when_already_saved(self):
        """save_torch_model() returns early without touching torch or fs."""
        mock_torch, _ = _mock_torch()
        var = ModelVariable(path="memory://x.pt", metadata=ModelMetadata())
        var._value = object()
        var._saved = True
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            var.save_torch_model()
            mock_torch.save.assert_not_called()
            mock_fsspec.core.url_to_fs.assert_not_called()

    def test_save_lightning_skips_when_already_saved(self):
        """save_lightning_model() returns early without touching torch or fs."""
        mock_torch, _ = _mock_torch()
        var = ModelVariable(path="memory://lit", metadata=ModelMetadata())
        var._value = MagicMock(name="lightning-model")
        var._saved = True
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            var.save_lightning_model()
            mock_torch.save.assert_not_called()
            mock_fsspec.core.url_to_fs.assert_not_called()
            var._value.state_dict.assert_not_called()


class TestModelVariableCustomIO(TestCase):
    """Tests for save_custom_model / load_custom_model IO."""

    def test_save_custom_uploads_temp_dir(self):
        """save_custom_model() calls model.save(temp) and fs.put(temp, path)."""
        mock_fs = MagicMock(name="fs")
        var = ModelVariable(path="memory://target", metadata=ModelMetadata())
        var._value = MagicMock(name="model")
        with patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec:
            mock_fsspec.core.url_to_fs.return_value = (mock_fs, "target")
            var.save_custom_model()
            var._value.save.assert_called_once()
            mock_fs.put.assert_called_once()
            put_args, put_kwargs = mock_fs.put.call_args
            self.assertEqual(put_args[1], "target")
            self.assertTrue(put_kwargs.get("recursive"))
        self.assertTrue(var._saved)

    def test_load_custom_imports_class_and_calls_load(self):
        """load_custom_model() imports model_class and calls .load(temp_path)."""
        var = ModelVariable(
            path="memory://target",
            metadata=ModelMetadata(model_class="pkg.mod.MyModel"),
        )
        mock_fs = MagicMock(name="fs")
        loaded_value = object()
        mock_class = MagicMock(load=MagicMock(return_value=loaded_value))
        with (
            patch(
                f"{_MODEL_PATH}.import_attribute", return_value=mock_class
            ) as mock_imp,
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (mock_fs, "target")
            var.load_custom_model()
            mock_imp.assert_called_once_with("pkg.mod.MyModel")
            mock_class.load.assert_called_once()
            mock_fs.get.assert_called_once()
        self.assertIs(var._value, loaded_value)
        self.assertTrue(var._saved)

    def test_load_custom_raises_when_model_class_missing(self):
        """load_custom_model() raises ValueError when model_class is not set."""
        var = ModelVariable(path="memory://x", metadata=ModelMetadata())
        with self.assertRaises(ValueError) as ctx:
            var.load_custom_model()
        self.assertIn("model_class must be set", str(ctx.exception))


class TestModelVariableTorchIO(TestCase):
    """Tests for save_torch_model / load_torch_model IO (full nn.Module pickle).

    ``ModelVariable`` is workflow scratch storage; the PyTorch path pickles
    the full ``nn.Module`` so fused models with nested submodules and
    non-trivial constructors round-trip without callers re-supplying
    constructor arguments. See ``save_torch_model`` / ``load_torch_model``
    docstrings for the security trade-off.
    """

    def test_save_torch_writes_full_module(self):
        """save_torch_model() persists the full nn.Module (not state_dict)."""
        mock_torch, _ = _mock_torch()
        mock_model = MagicMock(name="full-module")
        var = ModelVariable(path="memory://target")
        var._value = mock_model
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (MagicMock(), "target")
            var.save_torch_model()
        save_args, _ = mock_torch.save.call_args
        self.assertIs(save_args[0], mock_model)
        mock_model.state_dict.assert_not_called()

    def test_save_torch_does_not_mutate_path(self):
        """save_torch_model() leaves self.path unchanged (no .pt auto-append)."""
        mock_torch, _ = _mock_torch()
        var = ModelVariable(path="memory://target")
        var._value = MagicMock()
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (MagicMock(), "target")
            var.save_torch_model()
        self.assertEqual(var.path, "memory://target")

    def test_save_torch_uses_non_recursive_put(self):
        """save_torch_model() writes a single file with fs.put (no recursive)."""
        mock_torch, _ = _mock_torch()
        mock_fs = MagicMock()
        var = ModelVariable(path="memory://target")
        var._value = MagicMock()
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (mock_fs, "target")
            var.save_torch_model()
        _, put_kwargs = mock_fs.put.call_args
        self.assertNotIn("recursive", put_kwargs)

    def test_load_torch_defaults_weights_only_false(self):
        """load_torch_model() defaults weights_only=False to match full-pickle save."""
        loaded = object()
        mock_torch, _ = _mock_torch()
        mock_torch.load = MagicMock(return_value=loaded)
        var = ModelVariable(path="memory://x")
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (MagicMock(), "x")
            var.load_torch_model()
        _, load_kwargs = mock_torch.load.call_args
        self.assertFalse(load_kwargs.get("weights_only"))
        self.assertEqual(load_kwargs.get("map_location"), "cpu")
        self.assertIs(var._value, loaded)

    def test_load_torch_respects_weights_only_override(self):
        """load_torch_model(weights_only=True) forwards the override to torch.load."""
        mock_torch, _ = _mock_torch()
        mock_torch.load = MagicMock(return_value=object())
        var = ModelVariable(path="memory://x")
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (MagicMock(), "x")
            var.load_torch_model(weights_only=True)
        _, load_kwargs = mock_torch.load.call_args
        self.assertTrue(load_kwargs.get("weights_only"))

    def test_load_torch_respects_map_location_override(self):
        """load_torch_model(map_location=...) forwards the override to torch.load."""
        mock_torch, _ = _mock_torch()
        mock_torch.load = MagicMock(return_value=object())
        var = ModelVariable(path="memory://x")
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (MagicMock(), "x")
            var.load_torch_model(map_location="cuda:0")
        _, load_kwargs = mock_torch.load.call_args
        self.assertEqual(load_kwargs.get("map_location"), "cuda:0")

    def test_load_torch_accepts_map_location_none(self):
        """map_location=None is forwarded so torch.load uses original device."""
        mock_torch, _ = _mock_torch()
        mock_torch.load = MagicMock(return_value=object())
        var = ModelVariable(path="memory://x")
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (MagicMock(), "x")
            var.load_torch_model(map_location=None)
        _, load_kwargs = mock_torch.load.call_args
        self.assertIsNone(load_kwargs.get("map_location"))

    def test_load_torch_uses_non_recursive_get(self):
        """load_torch_model() reads a single file with fs.get (no recursive)."""
        mock_torch, _ = _mock_torch()
        mock_torch.load = MagicMock(return_value=object())
        mock_fs = MagicMock()
        var = ModelVariable(path="memory://x")
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (mock_fs, "x")
            var.load_torch_model()
        _, get_kwargs = mock_fs.get.call_args
        self.assertNotIn("recursive", get_kwargs)


class TestModelVariableLightningIO(TestCase):
    """Tests for save_lightning_model / load_lightning_model IO."""

    def test_save_lightning_writes_state_dict(self):
        """save_lightning_model() calls torch.save(model.state_dict(), ...)."""
        mock_torch, _ = _mock_torch()
        state_dict = {"w": "tensor"}
        mock_model = MagicMock()
        mock_model.state_dict.return_value = state_dict
        var = ModelVariable(path="memory://lit", metadata=ModelMetadata())
        var._value = mock_model
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (MagicMock(), "lit")
            var.save_lightning_model()
        save_args, _ = mock_torch.save.call_args
        self.assertIs(save_args[0], state_dict)
        mock_model.state_dict.assert_called_once()

    def test_load_lightning_raises_when_model_class_missing(self):
        """load_lightning_model() raises ValueError when model_class is not set."""
        var = ModelVariable(path="memory://x", metadata=ModelMetadata())
        with self.assertRaises(ValueError) as ctx:
            var.load_lightning_model()
        self.assertIn("model_class", str(ctx.exception))

    def test_load_lightning_instantiates_with_hyperparameters(self):
        """load_lightning_model() builds model_class(**hyperparameters)."""
        mock_torch, _ = _mock_torch()
        mock_torch.load = MagicMock(return_value={"w": 1.0})
        built = MagicMock(name="built")
        mock_class = MagicMock(return_value=built)
        var = ModelVariable(
            path="memory://lit",
            metadata=ModelMetadata(
                model_class="pkg.M",
                hyperparameters={"hidden": 16},
            ),
        )
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.import_attribute", return_value=mock_class),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (MagicMock(), "lit")
            var.load_lightning_model()
        mock_class.assert_called_once_with(hidden=16)
        built.load_state_dict.assert_called_once_with({"w": 1.0})
        built.eval.assert_called_once()
        self.assertIs(var._value, built)

    def test_load_lightning_uses_weights_only_true_for_state_dict(self):
        """load_lightning_model() forwards weights_only=True to torch.load."""
        mock_torch, _ = _mock_torch()
        mock_torch.load = MagicMock(return_value={})
        var = ModelVariable(
            path="memory://lit",
            metadata=ModelMetadata(model_class="pkg.M"),
        )
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.import_attribute", return_value=MagicMock()),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (MagicMock(), "lit")
            var.load_lightning_model()
        _, load_kwargs = mock_torch.load.call_args
        self.assertTrue(load_kwargs.get("weights_only"))
        self.assertEqual(load_kwargs.get("map_location"), "cpu")

    def test_load_lightning_respects_map_location_override(self):
        """load_lightning_model(map_location=...) is forwarded to torch.load."""
        mock_torch, _ = _mock_torch()
        mock_torch.load = MagicMock(return_value={})
        var = ModelVariable(
            path="memory://lit",
            metadata=ModelMetadata(model_class="pkg.M"),
        )
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.import_attribute", return_value=MagicMock()),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (MagicMock(), "lit")
            var.load_lightning_model(map_location="cuda:0")
        _, load_kwargs = mock_torch.load.call_args
        self.assertEqual(load_kwargs.get("map_location"), "cuda:0")

    def test_load_lightning_wraps_constructor_typeerror(self):
        """load_lightning_model() turns a TypeError from __init__ into a ValueError."""
        mock_torch, _ = _mock_torch()
        mock_torch.load = MagicMock(return_value={})

        def _bad_ctor(**kwargs):
            raise TypeError("missing 1 required positional argument: 'hidden'")

        var = ModelVariable(
            path="memory://lit", metadata=ModelMetadata(model_class="pkg.M")
        )
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.import_attribute", return_value=_bad_ctor),
            patch(f"{_MODEL_PATH}.fsspec"),
            self.assertRaises(ValueError) as ctx,
        ):
            var.load_lightning_model()
        self.assertIn("hyperparameters", str(ctx.exception))
        self.assertIn("pkg.M", str(ctx.exception))

    def test_load_lightning_defaults_hyperparameters_to_empty_dict(self):
        """load_lightning_model() treats hyperparameters=None as {}."""
        mock_torch, _ = _mock_torch()
        mock_torch.load = MagicMock(return_value={})
        mock_class = MagicMock(return_value=MagicMock())
        var = ModelVariable(
            path="memory://lit",
            metadata=ModelMetadata(model_class="pkg.M", hyperparameters=None),
        )
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.import_attribute", return_value=mock_class),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (MagicMock(), "lit")
            var.load_lightning_model()
        mock_class.assert_called_once_with()


class TestModelVariableImports(TestCase):
    """Tests for the public package surface of ModelVariable."""

    def test_init_import_from_package(self):
        """ModelVariable is importable from the package __init__."""
        from michelangelo.workflow import variables as _wv

        self.assertIs(_wv.ModelVariable, ModelVariable)


# ---------------------------------------------------------------------------
# Regression tests for review-feedback fixes (M1, M5, M6, M4-lightning)
# ---------------------------------------------------------------------------


def _mock_lightning(instance=None):
    """Return (mock_pytorch_lightning_module, mock_lightning_instance).

    The fake module exposes a ``LightningModule`` class that subclasses the
    mock torch ``nn.Module``, mirroring the real ``pl.LightningModule`` <:
    ``torch.nn.Module`` relationship that powers the M1 detection check.
    """
    mock_torch, _ = _mock_torch()
    nn_module_cls = mock_torch.nn.Module
    lit_module_cls = type("LightningModule", (nn_module_cls,), {})
    mock_pl = _types.SimpleNamespace(LightningModule=lit_module_cls)
    inst = instance if instance is not None else lit_module_cls()
    return mock_pl, mock_torch, inst


class TestModelVariableCreateLightning(TestCase):
    """Regression for M1: Lightning is detected before plain PyTorch in create()."""

    def test_create_detects_lightning_module_before_pytorch(self):
        """A LightningModule (subclass of nn.Module) is classified as lightning."""
        mock_pl, mock_torch, inst = _mock_lightning()
        with patch.dict(
            sys.modules, {"pytorch_lightning": mock_pl, "torch": mock_torch}
        ):
            var = ModelVariable.create(inst)
        self.assertEqual(var.metadata.training_framework, TRAINING_FRAMEWORK_LIGHTNING)
        self.assertTrue(var.metadata.model_class.endswith("LightningModule"))

    def test_create_falls_back_to_pytorch_when_lightning_missing(self):
        """Plain torch.nn.Module is still detected when pytorch_lightning is absent."""
        mock_torch, inst = _mock_torch()
        with patch.dict(sys.modules, {"torch": mock_torch, "pytorch_lightning": None}):
            var = ModelVariable.create(inst)
        self.assertEqual(var.metadata.training_framework, TRAINING_FRAMEWORK_PYTORCH)


class TestModelVariableInitErgonomics(TestCase):
    """Regression for M5 (metadata None footgun) and M6 (value= constructor)."""

    def test_init_accepts_value_keyword(self):
        """ModelVariable(value=model) attaches the value without using create()."""
        sentinel = object()
        var = ModelVariable(value=sentinel)
        self.assertIs(var._value, sentinel)

    def test_init_auto_generates_path_when_omitted(self):
        """ModelVariable() generates a memory:// path when none is provided."""
        var = ModelVariable()
        self.assertTrue(var.path.startswith("memory://"))

    def test_init_defaults_metadata_to_empty_model_metadata(self):
        """ModelVariable(metadata=None) attaches a fresh ModelMetadata, not None."""
        var = ModelVariable(path="/tmp/x")
        self.assertIsInstance(var.metadata, ModelMetadata)
        self.assertIsNone(var.metadata.training_framework)

    def test_save_with_default_metadata_raises_value_error(self):
        """Default metadata + save() raises a guided ValueError, not AttributeError."""
        var = ModelVariable(value=object(), path="/tmp/x")
        with self.assertRaises(ValueError) as ctx:
            var.save()
        msg = str(ctx.exception)
        self.assertIn("training_framework", msg)
        self.assertIn("pytorch", msg)
        self.assertIn("custom", msg)
        self.assertIn("lightning", msg)


class TestModelVariableLightningFsspecFlags(TestCase):
    """Regression for M4: Lightning save/load use non-recursive fsspec ops."""

    def test_save_lightning_uses_non_recursive_put(self):
        """save_lightning_model() writes a single file with fs.put (no recursive)."""
        mock_torch, _ = _mock_torch()
        mock_fs = MagicMock()
        var = ModelVariable(path="memory://lit")
        var._value = MagicMock(state_dict=MagicMock(return_value={}))
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (mock_fs, "lit")
            var.save_lightning_model()
        _, put_kwargs = mock_fs.put.call_args
        self.assertNotIn("recursive", put_kwargs)

    def test_load_lightning_uses_non_recursive_get(self):
        """load_lightning_model() reads a single file with fs.get (no recursive)."""
        mock_torch, _ = _mock_torch()
        mock_torch.load = MagicMock(return_value={})
        mock_fs = MagicMock()
        var = ModelVariable(
            path="memory://lit", metadata=ModelMetadata(model_class="pkg.M")
        )
        with (
            patch.dict(sys.modules, {"torch": mock_torch}),
            patch(f"{_MODEL_PATH}.import_attribute", return_value=MagicMock()),
            patch(f"{_MODEL_PATH}.fsspec") as mock_fsspec,
        ):
            mock_fsspec.core.url_to_fs.return_value = (mock_fs, "lit")
            var.load_lightning_model()
        _, get_kwargs = mock_fs.get.call_args
        self.assertNotIn("recursive", get_kwargs)


# ---------------------------------------------------------------------------
# B3 — End-to-end round-trip tests (guarded; run only when torch/lightning
# are installed). These would have caught the previous
# save(full-pickle) / load(weights_only=True) contract mismatch in CI.
# ---------------------------------------------------------------------------

try:
    import torch as _torch_real

    _HAS_TORCH = True
except ImportError:
    _HAS_TORCH = False

try:
    import pytorch_lightning as _pl_real

    _HAS_LIGHTNING = True
except ImportError:
    _HAS_LIGHTNING = False


if _HAS_TORCH:

    class _FusedRoundTripModel(_torch_real.nn.Module):
        """Fused model with nested submodules and a non-trivial constructor.

        Exercises the exact case raised in review (kenns29, 2026-06-16):
        ``state_dict + model_class(**hyperparameters)`` could not reconstruct
        fused models whose ``__init__`` builds nested submodules whose
        shapes depend on positional/keyword args not preserved in a flat
        ``hyperparameters`` dict. The full-pickle path round-trips them.
        """

        def __init__(self, encoder, decoder, scale: float):
            super().__init__()
            self.encoder = encoder
            self.decoder = decoder
            self.register_buffer("scale", _torch_real.tensor(scale))

        def forward(self, x):
            return self.decoder(self.encoder(x)) * self.scale


@unittest.skipUnless(_HAS_TORCH, "torch not installed")
class TestModelVariableTorchRoundTrip(TestCase):
    """End-to-end save/load round-trip for plain torch.nn.Module."""

    def test_create_save_then_lazy_load_via_value(self):
        """create() -> save() -> ModelVariable(path, metadata).value reconstructs.

        No ``hyperparameters`` are set on the metadata: the full ``nn.Module``
        is pickled by ``save_torch_model``, so the loader does not need to
        re-instantiate the class from constructor args.
        """
        import tempfile

        torch = _torch_real
        original = torch.nn.Linear(2, 1)

        var = ModelVariable.create(original)
        with tempfile.TemporaryDirectory() as tmp:
            var.path = f"{tmp}/model"
            var.save()

            restored = ModelVariable(path=var.path, metadata=var.metadata)
            loaded = restored.value
            self.assertIsInstance(loaded, torch.nn.Linear)
            self.assertEqual(loaded.in_features, 2)
            self.assertEqual(loaded.out_features, 1)

            for k, v in original.state_dict().items():
                self.assertTrue(torch.equal(loaded.state_dict()[k], v))

    def test_fused_model_with_nested_submodules_round_trips(self):
        """Fused module whose ctor takes prebuilt submodules round-trips.

        ``_FusedRoundTripModel.__init__(encoder, decoder, scale)`` cannot
        be reconstructed from a flat hyperparameters dict, but the
        full-pickle save/load contract handles it transparently.
        """
        import tempfile

        torch = _torch_real
        encoder = torch.nn.Sequential(torch.nn.Linear(3, 4), torch.nn.ReLU())
        decoder = torch.nn.Linear(4, 2)
        original = _FusedRoundTripModel(encoder, decoder, scale=0.5)

        var = ModelVariable.create(original)
        with tempfile.TemporaryDirectory() as tmp:
            var.path = f"{tmp}/fused"
            var.save()

            restored = ModelVariable(path=var.path, metadata=var.metadata)
            loaded = restored.value
            self.assertIsInstance(loaded, _FusedRoundTripModel)
            self.assertTrue(torch.equal(loaded.scale, original.scale))
            for k, v in original.state_dict().items():
                self.assertTrue(torch.equal(loaded.state_dict()[k], v))


if _HAS_TORCH and _HAS_LIGHTNING:

    class _RoundTripLightningModel(_pl_real.LightningModule):
        """Module-level Lightning model so dot-path import_attribute can find it."""

        def __init__(self, hidden: int = 4):
            super().__init__()
            self.hidden = hidden
            self.layer = _torch_real.nn.Linear(hidden, 1)

        def forward(self, x):
            return self.layer(x)


@unittest.skipUnless(
    _HAS_TORCH and _HAS_LIGHTNING, "torch + pytorch_lightning not installed"
)
class TestModelVariableLightningRoundTrip(TestCase):
    """End-to-end save/load round-trip for pytorch_lightning.LightningModule."""

    def test_create_save_then_lazy_load_via_value(self):
        """create() detects lightning, save+reload reconstructs and calls eval()."""
        import tempfile

        torch = _torch_real

        original = _RoundTripLightningModel(hidden=4)
        var = ModelVariable.create(original)
        self.assertEqual(var.metadata.training_framework, TRAINING_FRAMEWORK_LIGHTNING)
        var.metadata.hyperparameters = {"hidden": 4}

        with tempfile.TemporaryDirectory() as tmp:
            var.path = f"{tmp}/lit"
            var.save()

            restored = ModelVariable(path=var.path, metadata=var.metadata)
            loaded = restored.value
            self.assertIsInstance(loaded, _RoundTripLightningModel)
            self.assertEqual(loaded.hidden, 4)
            self.assertFalse(loaded.training)
            for k, v in original.state_dict().items():
                self.assertTrue(torch.equal(loaded.state_dict()[k], v))
