"""Tests for ModelPusherPlugin — upload dispatch and registry registration."""

from __future__ import annotations

import os
import tempfile
from unittest import TestCase
from unittest.mock import MagicMock

from michelangelo.lib.model_manager.registry.client import RegisteredModel
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.pusher import ModelPluginConfig
from michelangelo.workflow.tasks.pusher.plugins.model_plugin import ModelPusherPlugin
from michelangelo.workflow.variables.metadata import ModelMetadata
from michelangelo.workflow.variables.types import AssembledModel, ModelArtifact

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _artifact_file() -> str:
    """Return a real temp file path."""
    fd, path = tempfile.mkstemp()
    os.close(fd)
    return path


def _assembled() -> AssembledModel:
    """Return an AssembledModel backed by two real temp files."""
    return AssembledModel(
        raw_model=ModelArtifact(path=_artifact_file()),
        deployable_model=ModelArtifact(path=_artifact_file()),
    )


def _mock_registry(name: str = "m", version: str = "1") -> MagicMock:
    """Return a mock ModelRegistryClient that echoes the supplied name/version."""
    client = MagicMock()
    client.register_model.return_value = RegisteredModel(
        name=name,
        version=version,
        registry_uri=f"mock://{name}/{version}",
    )
    return client


def _mock_backend(raw_uri: str = "raw://uri", dep_uri: str = "dep://uri") -> MagicMock:
    """Return a mock StorageBackend whose upload() returns predictable URIs."""
    backend = MagicMock()
    backend.upload.side_effect = [raw_uri, dep_uri]
    return backend


def _plugin(
    model_name: str | None = "test-model",
    artifact: AssembledModel | None = None,
    backend: MagicMock | None = None,
    registry: MagicMock | None = None,
    extra_metadata: dict | None = None,
) -> ModelPusherPlugin:
    """Return a fully-configured ModelPusherPlugin using mock infrastructure."""
    return ModelPusherPlugin(
        config=ModelPluginConfig(
            model_name=model_name,
            extra_metadata=extra_metadata or {},
        ),
        artifact=artifact or _assembled(),
        storage_backend=backend or _mock_backend(),
        registry_client=registry or _mock_registry(name=model_name or "model-x"),
    )


# ---------------------------------------------------------------------------
# Init validation
# ---------------------------------------------------------------------------


class TestModelPusherPluginInit(TestCase):
    """Tests for ModelPusherPlugin.__init__() validation."""

    def test_raises_when_artifact_is_none(self):
        """It raises ConfigurationError when artifact=None."""
        with self.assertRaises(ConfigurationError) as ctx:
            ModelPusherPlugin(
                config=ModelPluginConfig(),
                artifact=None,
                storage_backend=_mock_backend(),
                registry_client=_mock_registry(),
            )
        self.assertIn("artifact", str(ctx.exception).lower())

    def test_raises_when_storage_backend_is_none(self):
        """It raises ConfigurationError when storage_backend=None."""
        with self.assertRaises(ConfigurationError) as ctx:
            ModelPusherPlugin(
                config=ModelPluginConfig(),
                artifact=_assembled(),
                storage_backend=None,
                registry_client=_mock_registry(),
            )
        self.assertIn("storage_backend", str(ctx.exception))

    def test_raises_when_registry_client_is_none(self):
        """It raises ConfigurationError when registry_client=None."""
        with self.assertRaises(ConfigurationError) as ctx:
            ModelPusherPlugin(
                config=ModelPluginConfig(),
                artifact=_assembled(),
                storage_backend=_mock_backend(),
                registry_client=None,
            )
        self.assertIn("registry_client", str(ctx.exception))


# ---------------------------------------------------------------------------
# Execute — upload and registry dispatch
# ---------------------------------------------------------------------------


class TestModelPusherPluginExecute(TestCase):
    """Tests for ModelPusherPlugin.execute()."""

    def test_uploads_twice_registers_once_returns_four_keys(self):
        """It calls upload() twice and register_model() once, returning a 4-key dict."""
        backend = _mock_backend()
        registry = _mock_registry(name="clf", version="3")
        result = _plugin(model_name="clf", backend=backend, registry=registry).execute()

        self.assertEqual(backend.upload.call_count, 2)
        self.assertEqual(registry.register_model.call_count, 1)
        self.assertIn("model_name", result)
        self.assertIn("version", result)
        self.assertIn("raw_artifact_uri", result)
        self.assertIn("deployable_artifact_uri", result)

    def test_uses_config_model_name(self):
        """It registers the model under the name set in ModelPluginConfig."""
        registry = _mock_registry(name="my-clf")
        _plugin(model_name="my-clf", registry=registry).execute()
        call_kwargs = registry.register_model.call_args.kwargs
        self.assertEqual(call_kwargs["name"], "my-clf")

    def test_generates_name_when_model_name_is_none(self):
        """It auto-generates a 'model-{uuid8}' name when config.model_name is None."""
        registry = MagicMock()
        registry.register_model.side_effect = lambda name, **kw: RegisteredModel(
            name=name, version="1", registry_uri=f"mock://{name}/1"
        )
        result = ModelPusherPlugin(
            config=ModelPluginConfig(model_name=None),
            artifact=_assembled(),
            storage_backend=_mock_backend(),
            registry_client=registry,
        ).execute()
        self.assertTrue(result["model_name"].startswith("model-"))
        self.assertEqual(len(result["model_name"]), len("model-") + 8)

    def test_metadata_merges_model_metadata_and_extra_metadata(self):
        """It merges ModelMetadata fields with extra_metadata; omits private fields."""
        registry = MagicMock()
        registry.register_model.side_effect = lambda name, **kw: RegisteredModel(
            name=name, version="1", registry_uri=f"mock://{name}/1"
        )
        artifact = _assembled()
        artifact.raw_model.metadata = ModelMetadata(
            training_framework="xgboost",
            deployable=True,
        )
        ModelPusherPlugin(
            config=ModelPluginConfig(
                model_name="m", extra_metadata={"team": "pricing"}
            ),
            artifact=artifact,
            storage_backend=_mock_backend(),
            registry_client=registry,
        ).execute()

        metadata = registry.register_model.call_args.kwargs["metadata"]
        self.assertEqual(metadata["training_framework"], "xgboost")
        self.assertEqual(metadata["deployable"], "true")
        self.assertEqual(metadata["team"], "pricing")
        self.assertNotIn("_schema", metadata)
        self.assertNotIn("_sample_data", metadata)
        self.assertNotIn("_hyperparameters", metadata)

    def test_raw_artifact_uploaded_before_deployable(self):
        """It uploads raw_model before deployable_model; verified via call_args_list."""
        raw_path = _artifact_file()
        dep_path = _artifact_file()
        backend = _mock_backend()
        ModelPusherPlugin(
            config=ModelPluginConfig(model_name="m"),
            artifact=AssembledModel(
                raw_model=ModelArtifact(path=raw_path),
                deployable_model=ModelArtifact(path=dep_path),
            ),
            storage_backend=backend,
            registry_client=_mock_registry(),
        ).execute()
        calls = backend.upload.call_args_list
        self.assertEqual(calls[0][0][0], raw_path)
        self.assertEqual(calls[1][0][0], dep_path)

    def test_upload_uris_forwarded_to_register_model_and_result(self):
        """It passes upload() return values to register_model() and the result dict."""
        backend = _mock_backend(raw_uri="s3://bucket/raw", dep_uri="s3://bucket/dep")
        registry = MagicMock()
        registry.register_model.return_value = RegisteredModel(
            name="m", version="2", registry_uri="mock://m/2"
        )
        result = ModelPusherPlugin(
            config=ModelPluginConfig(model_name="m"),
            artifact=_assembled(),
            storage_backend=backend,
            registry_client=registry,
        ).execute()

        call_kwargs = registry.register_model.call_args.kwargs
        self.assertEqual(call_kwargs["artifact_uri"], "s3://bucket/raw")
        self.assertEqual(call_kwargs["deployable_artifact_uri"], "s3://bucket/dep")
        self.assertEqual(result["raw_artifact_uri"], "s3://bucket/raw")
        self.assertEqual(result["deployable_artifact_uri"], "s3://bucket/dep")
        self.assertEqual(result["version"], "2")
