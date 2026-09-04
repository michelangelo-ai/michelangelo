"""Tests for ModelPusherPlugin — upload dispatch and registry registration."""

from __future__ import annotations

import itertools
import os
import re
import tempfile
from unittest import TestCase
from unittest.mock import MagicMock, patch

from michelangelo.lib.model_manager.registry.client import RegisteredModel
from michelangelo.lib.model_manager.registry.pipeline_run import (
    SourcePipelineRun,
    _reset_source_pipeline_run_cache,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.pusher import ModelPluginConfig
from michelangelo.workflow.tasks.pusher.plugins.model_plugin import (
    ModelPusherPlugin,
    PartialRegistrationError,
)
from michelangelo.workflow.variables.metadata import ModelMetadata
from michelangelo.workflow.variables.types import AssembledModel, ModelArtifact

_GENERATED_NAME_RE = re.compile(r"^model-\d{8}-\d{6}-[0-9a-f]{8}$")

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


_artifact_counter = itertools.count()


def _artifact_file() -> str:
    """Return a unique fake artifact path (no real file created — backend is mocked)."""
    return f"/tmp/fake-artifact-{next(_artifact_counter)}"


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
    labels: dict | None = None,
    description: str | None = None,
    kind: str | None = None,
    run_id: str | None = None,
    metadata: dict | None = None,
) -> ModelPusherPlugin:
    """Return a fully-configured ModelPusherPlugin using mock infrastructure."""
    return ModelPusherPlugin(
        config=ModelPluginConfig(
            model_name=model_name,
            labels=labels or {},
            description=description,
            kind=kind,
            run_id=run_id,
            metadata=metadata or {},
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

    def test_uploads_twice_registers_once_returns_six_keys(self):
        """It calls upload() twice and register_model() once; result has 6 keys."""
        backend = _mock_backend()
        registry = _mock_registry(name="clf", version="3")
        result = _plugin(model_name="clf", backend=backend, registry=registry).execute()

        self.assertEqual(backend.upload.call_count, 2)
        self.assertEqual(registry.register_model.call_count, 1)
        self.assertIn("model_name", result)
        self.assertIn("version", result)
        self.assertIn("raw_artifact_uri", result)
        self.assertIn("deployable_artifact_uri", result)
        self.assertIn("push_id", result)
        self.assertIn("registrations", result)

    def test_remote_artifact_uri_downloaded_before_upload(self):
        """A URI artifact.path is downloaded to a local path before upload().

        The temp download is cleaned up after execute() returns, so
        existence is checked from within the upload() call itself.
        """
        backend = _mock_backend()
        artifact = AssembledModel(
            raw_model=ModelArtifact(path="s3://bucket/tabular_assembler/x/raw"),
            deployable_model=ModelArtifact(
                path="s3://bucket/tabular_assembler/x/deployable"
            ),
        )

        def _fake_download(uri, local_path):
            with open(local_path, "w") as f:
                f.write("fake artifact content")

        backend.download.side_effect = _fake_download

        upload_time_existence = []

        def _fake_upload(local_path, destination_key):
            upload_time_existence.append(os.path.isfile(local_path))
            return f"s3://uploaded/{destination_key}"

        backend.upload.side_effect = _fake_upload

        result = _plugin(artifact=artifact, backend=backend).execute()

        self.assertEqual(backend.download.call_count, 2)
        raw_local_path = backend.upload.call_args_list[0][0][0]
        dep_local_path = backend.upload.call_args_list[1][0][0]
        self.assertNotEqual(raw_local_path, "s3://bucket/tabular_assembler/x/raw")
        self.assertNotEqual(
            dep_local_path, "s3://bucket/tabular_assembler/x/deployable"
        )
        self.assertEqual(upload_time_existence, [True, True])

        # The local download directory name (an implementation detail) must
        # not leak into the destination storage key.
        raw_key = backend.upload.call_args_list[0][0][1]
        dep_key = backend.upload.call_args_list[1][0][1]
        self.assertEqual(raw_key, f"models/{result['model_name']}/raw")
        self.assertEqual(dep_key, f"models/{result['model_name']}/deployable/model.tar")

    def test_local_artifact_path_uploaded_directly_without_download(self):
        """A plain local path is passed straight to upload() without download."""
        backend = _mock_backend()
        artifact = _assembled()

        _plugin(artifact=artifact, backend=backend).execute()

        self.assertEqual(backend.download.call_count, 0)
        raw_local_path = backend.upload.call_args_list[0][0][0]
        self.assertEqual(raw_local_path, artifact.raw_model.path)

    def test_uses_config_model_name(self):
        """It registers the model under the name set in ModelPluginConfig."""
        registry = _mock_registry(name="my-clf")
        _plugin(model_name="my-clf", registry=registry).execute()
        call_kwargs = registry.register_model.call_args.kwargs
        self.assertEqual(call_kwargs["name"], "my-clf")

    def test_generates_name_when_model_name_is_none(self):
        """It auto-generates a timestamped name when config.model_name is None."""
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
        self.assertRegex(result["model_name"], _GENERATED_NAME_RE)

    def test_name_generation_delegates_to_shared_api_utility(self):
        """It calls michelangelo.api.v2 generate_random_name with the 'model' prefix."""
        registry = MagicMock()
        registry.register_model.side_effect = lambda name, **kw: RegisteredModel(
            name=name, version="1", registry_uri=f"mock://{name}/1"
        )
        target = (
            "michelangelo.workflow.tasks.pusher.plugins."
            "model_plugin.generate_random_name"
        )
        with patch(target, return_value="model-20260721-114130-2d9c959d") as gen:
            result = ModelPusherPlugin(
                config=ModelPluginConfig(model_name=None),
                artifact=_assembled(),
                storage_backend=_mock_backend(),
                registry_client=registry,
            ).execute()
        gen.assert_called_once_with("model")
        self.assertEqual(result["model_name"], "model-20260721-114130-2d9c959d")

    def test_storage_key_is_stable_across_calls_with_same_model_name(self):
        """Two execute() calls with the same model_name reuse the same key.

        This is a deliberate tradeoff: storage keys have no per-push
        differentiator, so re-pushing the same model_name overwrites the
        previous push's storage objects.
        """
        backend1 = _mock_backend(raw_uri="s3://b/raw1", dep_uri="s3://b/dep1")
        backend2 = _mock_backend(raw_uri="s3://b/raw2", dep_uri="s3://b/dep2")
        _plugin(model_name="m", backend=backend1).execute()
        _plugin(model_name="m", backend=backend2).execute()

        key1_raw = backend1.upload.call_args_list[0][0][1]
        key2_raw = backend2.upload.call_args_list[0][0][1]
        self.assertEqual(key1_raw, "models/m/raw")
        self.assertEqual(key2_raw, "models/m/raw")

    def test_description_forwarded_to_register_model(self):
        """It passes config.description to register_model(description=...)."""
        registry = _mock_registry(name="m")
        _plugin(
            model_name="m", registry=registry, description="My prod classifier"
        ).execute()
        call_kwargs = registry.register_model.call_args.kwargs
        self.assertEqual(call_kwargs["description"], "My prod classifier")

    def test_description_none_when_not_set(self):
        """It forwards description=None when config.description is not set."""
        registry = _mock_registry(name="m")
        _plugin(model_name="m", registry=registry).execute()
        call_kwargs = registry.register_model.call_args.kwargs
        self.assertIsNone(call_kwargs["description"])

    def test_kind_forwarded_to_register_model(self):
        """It passes config.kind to register_model(kind=...)."""
        registry = _mock_registry(name="m")
        _plugin(model_name="m", registry=registry, kind="regression").execute()
        call_kwargs = registry.register_model.call_args.kwargs
        self.assertEqual(call_kwargs["kind"], "regression")

    def test_kind_none_when_not_set(self):
        """It forwards kind=None when config.kind is not set."""
        registry = _mock_registry(name="m")
        _plugin(model_name="m", registry=registry).execute()
        call_kwargs = registry.register_model.call_args.kwargs
        self.assertIsNone(call_kwargs["kind"])

    def test_labels_merges_model_metadata_and_config_labels(self):
        """It merges ModelMetadata.to_registry_dict() with config.labels."""
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
            config=ModelPluginConfig(model_name="m", labels={"owner": "ml-platform"}),
            artifact=artifact,
            storage_backend=_mock_backend(),
            registry_client=registry,
        ).execute()

        labels = registry.register_model.call_args.kwargs["labels"]
        self.assertEqual(labels["training_framework"], "xgboost")
        self.assertEqual(labels["deployable"], "true")
        self.assertEqual(labels["owner"], "ml-platform")
        self.assertNotIn("_schema", labels)
        self.assertNotIn("_sample_data", labels)
        self.assertNotIn("_hyperparameters", labels)

    def test_config_labels_take_precedence_over_derived_labels_on_collision(self):
        """Caller-supplied labels override artifact-derived labels on key conflict."""
        registry = MagicMock()
        registry.register_model.side_effect = lambda name, **kw: RegisteredModel(
            name=name, version="1", registry_uri=f"mock://{name}/1"
        )
        artifact = _assembled()
        artifact.raw_model.metadata = ModelMetadata(training_framework="xgboost")
        ModelPusherPlugin(
            config=ModelPluginConfig(
                model_name="m", labels={"training_framework": "override"}
            ),
            artifact=artifact,
            storage_backend=_mock_backend(),
            registry_client=registry,
        ).execute()
        labels = registry.register_model.call_args.kwargs["labels"]
        self.assertEqual(labels["training_framework"], "override")

    def test_run_id_forwarded_to_metadata_kwarg(self):
        """config.run_id is injected into metadata['run_id'] for register_model()."""
        registry = _mock_registry(name="m")
        _plugin(model_name="m", registry=registry, run_id="mlflow-run-abc123").execute()
        call_kwargs = registry.register_model.call_args.kwargs
        self.assertEqual(call_kwargs["metadata"]["run_id"], "mlflow-run-abc123")

    def test_run_id_absent_from_labels(self):
        """config.run_id does not appear in labels — only in metadata."""
        registry = _mock_registry(name="m")
        _plugin(model_name="m", registry=registry, run_id="my-run-id").execute()
        labels = registry.register_model.call_args.kwargs["labels"]
        self.assertNotIn("run_id", labels)

    def test_metadata_empty_when_run_id_not_set(self):
        """Metadata kwarg is an empty dict when config.run_id is None."""
        registry = _mock_registry(name="m")
        _plugin(model_name="m", registry=registry).execute()
        metadata = registry.register_model.call_args.kwargs["metadata"]
        self.assertEqual(metadata, {})

    def test_config_metadata_forwarded_to_register_model(self):
        """config.metadata values are forwarded in the metadata kwarg."""
        registry = _mock_registry(name="m")
        _plugin(
            model_name="m",
            registry=registry,
            metadata={"accuracy": 0.94, "git_sha": "abc123"},
        ).execute()
        metadata = registry.register_model.call_args.kwargs["metadata"]
        self.assertEqual(metadata["accuracy"], 0.94)
        self.assertEqual(metadata["git_sha"], "abc123")

    def test_run_id_takes_precedence_over_metadata_key_collision(self):
        """config.run_id overwrites metadata['run_id'] on collision."""
        registry = _mock_registry(name="m")
        _plugin(
            model_name="m",
            registry=registry,
            run_id="authoritative-run",
            metadata={"run_id": "should-be-overwritten"},
        ).execute()
        metadata = registry.register_model.call_args.kwargs["metadata"]
        self.assertEqual(metadata["run_id"], "authoritative-run")

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

    def test_raw_only_model_skips_deployable_upload(self):
        """When deployable_model is None, upload() is called once only."""
        backend = _mock_backend(raw_uri="s3://bucket/raw", dep_uri="s3://bucket/dep")
        registry = _mock_registry(name="m")
        result = ModelPusherPlugin(
            config=ModelPluginConfig(model_name="m"),
            artifact=AssembledModel(raw_model=ModelArtifact(path=_artifact_file())),
            storage_backend=backend,
            registry_client=registry,
        ).execute()
        self.assertEqual(backend.upload.call_count, 1)
        self.assertIsNone(result["deployable_artifact_uri"])

    def test_raw_only_model_passes_none_deployable_uri_to_registry(self):
        """Raw-only model: register_model receives deployable_artifact_uri=None."""
        registry = _mock_registry(name="m")
        ModelPusherPlugin(
            config=ModelPluginConfig(model_name="m"),
            artifact=AssembledModel(raw_model=ModelArtifact(path=_artifact_file())),
            storage_backend=_mock_backend(),
            registry_client=registry,
        ).execute()
        call_kwargs = registry.register_model.call_args.kwargs
        self.assertIsNone(call_kwargs["deployable_artifact_uri"])

    def test_upload_io_error_propagates_before_registry_call(self):
        """If upload() raises IOError, registry.register_model() is never called."""
        backend = MagicMock()
        backend.upload.side_effect = OSError("storage unavailable")
        registry = _mock_registry(name="m")
        with self.assertRaises(IOError, msg="storage unavailable"):
            _plugin(model_name="m", backend=backend, registry=registry).execute()
        registry.register_model.assert_not_called()

    def test_result_contains_registrations_list(self):
        """execute() returns a 'registrations' list with one entry per registry."""
        registry = _mock_registry(name="clf", version="5")
        result = _plugin(model_name="clf", registry=registry).execute()
        self.assertIn("registrations", result)
        self.assertEqual(len(result["registrations"]), 1)
        self.assertEqual(result["registrations"][0]["version"], "5")
        self.assertEqual(result["registrations"][0]["registry_uri"], "mock://clf/5")

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


# ---------------------------------------------------------------------------
# Multi-registry fan-out
# ---------------------------------------------------------------------------


class TestModelPusherPluginMultiRegistry(TestCase):
    """Tests for ModelPusherPlugin multi-registry fan-out via registry_clients."""

    def _make_registry(self, version: str, uri_prefix: str) -> MagicMock:
        client = MagicMock()
        client.register_model.side_effect = lambda name, **kw: RegisteredModel(
            name=name, version=version, registry_uri=f"{uri_prefix}/{name}/{version}"
        )
        return client

    def test_config_registry_clients_used_when_provided(self):
        """It uses config.registry_clients instead of constructor registry_client."""
        r1 = self._make_registry("1", "mlflow://")
        r2 = self._make_registry("42", "catalog://")
        plugin = ModelPusherPlugin(
            config=ModelPluginConfig(
                model_name="clf",
                registry_clients=[r1, r2],
            ),
            artifact=_assembled(),
            storage_backend=_mock_backend(),
        )
        result = plugin.execute()

        r1.register_model.assert_called_once()
        r2.register_model.assert_called_once()
        self.assertEqual(len(result["registrations"]), 2)

    def test_version_from_first_registry_in_top_level_field(self):
        """Top-level 'version' comes from the first registry in the list."""
        r1 = self._make_registry("v10", "first://")
        r2 = self._make_registry("v99", "second://")
        result = ModelPusherPlugin(
            config=ModelPluginConfig(model_name="m", registry_clients=[r1, r2]),
            artifact=_assembled(),
            storage_backend=_mock_backend(),
        ).execute()
        self.assertEqual(result["version"], "v10")

    def test_registrations_contain_per_registry_version_and_uri(self):
        """Each entry in 'registrations' carries that registry's version and URI."""
        r1 = self._make_registry("1", "mlflow://models")
        r2 = self._make_registry("abc", "catalog://models")
        result = ModelPusherPlugin(
            config=ModelPluginConfig(model_name="m", registry_clients=[r1, r2]),
            artifact=_assembled(),
            storage_backend=_mock_backend(),
        ).execute()
        self.assertEqual(
            result["registrations"][0]["registry_uri"], "mlflow://models/m/1"
        )
        self.assertEqual(
            result["registrations"][1]["registry_uri"], "catalog://models/m/abc"
        )

    def test_same_artifact_uris_sent_to_all_registries(self):
        """All registries receive the same raw_uri and deployable_uri."""
        r1 = self._make_registry("1", "a://")
        r2 = self._make_registry("1", "b://")
        backend = _mock_backend(raw_uri="s3://raw", dep_uri="s3://dep")
        ModelPusherPlugin(
            config=ModelPluginConfig(model_name="m", registry_clients=[r1, r2]),
            artifact=_assembled(),
            storage_backend=backend,
        ).execute()
        for client in (r1, r2):
            kw = client.register_model.call_args.kwargs
            self.assertEqual(kw["artifact_uri"], "s3://raw")
            self.assertEqual(kw["deployable_artifact_uri"], "s3://dep")

    def test_raises_when_no_registry_configured(self):
        """It raises ConfigurationError when no registry is configured."""
        with self.assertRaises(ConfigurationError) as ctx:
            ModelPusherPlugin(
                config=ModelPluginConfig(),
                artifact=_assembled(),
                storage_backend=_mock_backend(),
                registry_client=None,
            )
        self.assertIn("registry", str(ctx.exception).lower())

    def test_raises_when_both_registry_client_and_registry_clients_provided(self):
        """ConfigurationError when both registry_client and registry_clients are set."""
        r1 = self._make_registry("1", "a://")
        r2 = self._make_registry("1", "b://")
        with self.assertRaises(ConfigurationError) as ctx:
            ModelPusherPlugin(
                config=ModelPluginConfig(registry_clients=[r1]),
                artifact=_assembled(),
                storage_backend=_mock_backend(),
                registry_client=r2,
            )
        self.assertIn("registry_client", str(ctx.exception).lower())

    def test_labels_not_mutated_across_registries(self):
        """Each registry receives an independent copy of the labels dict."""
        r2_saw_r1_key: list[bool] = []

        def capture_r1(name, **kw):
            kw["labels"]["injected_by_r1"] = "yes"
            return RegisteredModel(name=name, version="1", registry_uri=f"x://{name}/1")

        def capture_r2(name, **kw):
            r2_saw_r1_key.append("injected_by_r1" in kw["labels"])
            return RegisteredModel(name=name, version="2", registry_uri=f"x://{name}/2")

        r1 = MagicMock()
        r1.register_model.side_effect = capture_r1
        r2 = MagicMock()
        r2.register_model.side_effect = capture_r2

        ModelPusherPlugin(
            config=ModelPluginConfig(model_name="m", registry_clients=[r1, r2]),
            artifact=_assembled(),
            storage_backend=_mock_backend(),
        ).execute()

        self.assertFalse(r2_saw_r1_key[0], "r2 received a labels dict mutated by r1")

    def test_metadata_not_mutated_across_registries(self):
        """Each registry receives an independent copy of the metadata dict."""
        r2_saw_r1_key: list[bool] = []

        def capture_r1(name, **kw):
            kw["metadata"]["injected_by_r1"] = "yes"
            return RegisteredModel(name=name, version="1", registry_uri=f"x://{name}/1")

        def capture_r2(name, **kw):
            r2_saw_r1_key.append("injected_by_r1" in kw["metadata"])
            return RegisteredModel(name=name, version="2", registry_uri=f"x://{name}/2")

        r1 = MagicMock()
        r1.register_model.side_effect = capture_r1
        r2 = MagicMock()
        r2.register_model.side_effect = capture_r2

        ModelPusherPlugin(
            config=ModelPluginConfig(
                model_name="m", registry_clients=[r1, r2], run_id="run-123"
            ),
            artifact=_assembled(),
            storage_backend=_mock_backend(),
        ).execute()

        self.assertFalse(r2_saw_r1_key[0], "r2 received a metadata dict mutated by r1")

    def test_partial_fanout_failure_propagates_after_first_registry_succeeds(self):
        """PartialRegistrationError is raised when r2 fails after r1 succeeds."""
        r1 = self._make_registry("1", "a://")
        r2 = MagicMock()
        r2.register_model.side_effect = OSError("registry-2 unavailable")

        with self.assertRaises(PartialRegistrationError) as ctx:
            ModelPusherPlugin(
                config=ModelPluginConfig(model_name="m", registry_clients=[r1, r2]),
                artifact=_assembled(),
                storage_backend=_mock_backend(),
            ).execute()

        err = ctx.exception
        self.assertEqual(len(err.registrations_completed), 1)
        self.assertEqual(err.registrations_completed[0]["registry_uri"], "a:///m/1")
        self.assertEqual(err.failed_registry_type, "MagicMock")
        self.assertIsInstance(err.__cause__, IOError)
        r1.register_model.assert_called_once()
        r2.register_model.assert_called_once()

    def test_single_registry_failure_propagates_raw(self):
        """When one registry fails, the raw exception propagates (no wrapping)."""
        registry = MagicMock()
        registry.register_model.side_effect = OSError("registry unavailable")

        with self.assertRaises(IOError):
            ModelPusherPlugin(
                config=ModelPluginConfig(model_name="m"),
                artifact=_assembled(),
                storage_backend=_mock_backend(),
                registry_client=registry,
            ).execute()


# ---------------------------------------------------------------------------
# Pipeline-run provenance
# ---------------------------------------------------------------------------


class TestModelPusherPluginSourcePipelineRun(TestCase):
    """Tests for ModelPusherPlugin.execute() threading source_pipeline_run."""

    def setUp(self) -> None:
        """Isolate the module-level pipeline-run cache and env vars per test."""
        _reset_source_pipeline_run_cache()
        self.addCleanup(_reset_source_pipeline_run_cache)
        self._env_patcher = patch.dict(os.environ, {}, clear=False)
        self._env_patcher.start()
        os.environ.pop("MA_PIPELINE_RUN_NAME", None)
        os.environ.pop("MA_NAMESPACE", None)
        self.addCleanup(self._env_patcher.stop)

    def test_source_pipeline_run_passed_when_env_vars_set(self):
        """execute() calls register_model() with source_pipeline_run from the env."""
        os.environ["MA_PIPELINE_RUN_NAME"] = "run-1"
        os.environ["MA_NAMESPACE"] = "ns-1"
        registry = _mock_registry(name="clf")
        _plugin(model_name="clf", registry=registry).execute()

        call_kwargs = registry.register_model.call_args.kwargs
        self.assertEqual(
            call_kwargs["source_pipeline_run"],
            SourcePipelineRun(name="run-1", namespace="ns-1"),
        )

    def test_source_pipeline_run_none_when_env_vars_unset(self):
        """execute() passes source_pipeline_run=None outside a pipeline (local dev).

        Regression guard: every other test in this file runs with neither env
        var set and must keep working exactly as before this change.
        """
        registry = _mock_registry(name="clf")
        _plugin(model_name="clf", registry=registry).execute()

        call_kwargs = registry.register_model.call_args.kwargs
        self.assertIsNone(call_kwargs["source_pipeline_run"])

    def test_source_pipeline_run_sent_to_every_registry_in_fanout(self):
        """Multi-registry fan-out: every registry receives the same value."""
        os.environ["MA_PIPELINE_RUN_NAME"] = "run-2"
        r1 = MagicMock()
        r1.register_model.return_value = RegisteredModel(
            name="m", version="1", registry_uri="a://m/1"
        )
        r2 = MagicMock()
        r2.register_model.return_value = RegisteredModel(
            name="m", version="1", registry_uri="b://m/1"
        )
        ModelPusherPlugin(
            config=ModelPluginConfig(model_name="m", registry_clients=[r1, r2]),
            artifact=_assembled(),
            storage_backend=_mock_backend(),
        ).execute()

        expected = SourcePipelineRun(name="run-2", namespace=None)
        for client in (r1, r2):
            kw = client.register_model.call_args.kwargs
            self.assertEqual(kw["source_pipeline_run"], expected)


# ---------------------------------------------------------------------------
# Push ID and result contract
# ---------------------------------------------------------------------------


class TestModelPusherPluginPushId(TestCase):
    """Tests for push_id presence/uniqueness in the result and its scope."""

    def test_push_id_present_in_result(self):
        """execute() result contains a 'push_id' key."""
        result = _plugin(model_name="m").execute()
        self.assertIn("push_id", result)
        self.assertEqual(len(result["push_id"]), 16)

    def test_push_id_not_in_storage_key(self):
        """push_id is a per-call correlation token only, absent from storage keys."""
        backend = _mock_backend()
        result = _plugin(model_name="m", backend=backend).execute()
        raw_key = backend.upload.call_args_list[0][0][1]
        self.assertNotIn(result["push_id"], raw_key)

    def test_push_id_not_in_registry_labels_or_metadata(self):
        """push_id is a correlation token — absent from labels and metadata."""
        registry = _mock_registry(name="m")
        _plugin(model_name="m", registry=registry).execute()
        labels = registry.register_model.call_args.kwargs["labels"]
        metadata = registry.register_model.call_args.kwargs["metadata"]
        self.assertNotIn("push_id", labels)
        self.assertNotIn("michelangelo.push_id", labels)
        self.assertNotIn("push_id", metadata)
        self.assertNotIn("michelangelo.push_id", metadata)


# ---------------------------------------------------------------------------
# Storage key — artifact filename in raw / deployable key
# ---------------------------------------------------------------------------


class TestModelPusherPluginStorageKey(TestCase):
    """Tests for the fixed destination-key format used for raw/deployable uploads."""

    def test_raw_key_is_fixed_regardless_of_artifact_filename(self):
        """Raw upload key is fixed and does not echo the artifact's filename."""
        backend = _mock_backend()
        ModelPusherPlugin(
            config=ModelPluginConfig(model_name="m"),
            artifact=AssembledModel(raw_model=ModelArtifact(path="/tmp/model.ubj")),
            storage_backend=backend,
            registry_client=_mock_registry(),
        ).execute()
        raw_key = backend.upload.call_args_list[0][0][1]
        self.assertEqual(raw_key, "models/m/raw")

    def test_deployable_key_is_fixed_regardless_of_artifact_filename(self):
        """Deployable upload key is fixed and does not echo the artifact's filename.

        A real file is used for the deployable path (rather than the
        fake nonexistent paths used elsewhere in this module) because the
        raw-vs-deployable key format now depends on whether the artifact is
        actually a file or a directory on disk.
        """
        backend = _mock_backend()
        with tempfile.NamedTemporaryFile(suffix="serving_model.zip") as f:
            ModelPusherPlugin(
                config=ModelPluginConfig(model_name="m"),
                artifact=AssembledModel(
                    raw_model=ModelArtifact(path="/tmp/model.ubj"),
                    deployable_model=ModelArtifact(path=f.name),
                ),
                storage_backend=backend,
                registry_client=_mock_registry(),
            ).execute()
        dep_key = backend.upload.call_args_list[1][0][1]
        self.assertEqual(dep_key, "models/m/deployable/model.tar")

    def test_deployable_key_has_no_model_tar_suffix_for_directory_artifact(self):
        """A deployable artifact that's a directory gets no /model.tar suffix.

        The assembler always hands off the deployable package as loose
        files (tarring, when requested via
        ModelPluginConfig.tar_deployable_package, is the pusher's job) --
        with tar_deployable_package left at its default (False) here, a
        directory artifact must be keyed like the raw model, not assumed
        to always be a tar.
        """
        backend = _mock_backend()
        with tempfile.TemporaryDirectory() as d:
            ModelPusherPlugin(
                config=ModelPluginConfig(model_name="m"),
                artifact=AssembledModel(
                    raw_model=ModelArtifact(path="/tmp/model.ubj"),
                    deployable_model=ModelArtifact(path=d),
                ),
                storage_backend=backend,
                registry_client=_mock_registry(),
            ).execute()
        dep_key = backend.upload.call_args_list[1][0][1]
        self.assertEqual(dep_key, "models/m/deployable")

    def test_tar_deployable_package_archives_directory_before_upload(self):
        """tar_deployable_package=True archives a directory deployable artifact.

        Tarring a directory deployable artifact (rather than uploading it
        as loose files) is the pusher's decision, driven by
        ModelPluginConfig.tar_deployable_package -- the assembler always
        hands off loose files regardless.
        """
        backend = _mock_backend()
        upload_time_is_file: list[bool] = []

        def _fake_upload(local_path, destination_key):
            upload_time_is_file.append(os.path.isfile(local_path))
            return f"s3://uploaded/{destination_key}"

        backend.upload.side_effect = _fake_upload

        with tempfile.TemporaryDirectory() as d:
            with open(os.path.join(d, "config.pbtxt"), "w") as f:
                f.write('name: "m"')
            ModelPusherPlugin(
                config=ModelPluginConfig(model_name="m", tar_deployable_package=True),
                artifact=AssembledModel(
                    raw_model=ModelArtifact(path="/tmp/model.ubj"),
                    deployable_model=ModelArtifact(path=d),
                ),
                storage_backend=backend,
                registry_client=_mock_registry(),
            ).execute()
        dep_key = backend.upload.call_args_list[1][0][1]
        self.assertEqual(upload_time_is_file, [False, True])
        self.assertEqual(dep_key, "models/m/deployable/model.tar")

    def test_tar_deployable_package_has_no_effect_on_file_artifact(self):
        """tar_deployable_package=True is a no-op when already a single file."""
        backend = _mock_backend()
        with tempfile.NamedTemporaryFile() as f:
            ModelPusherPlugin(
                config=ModelPluginConfig(model_name="m", tar_deployable_package=True),
                artifact=AssembledModel(
                    raw_model=ModelArtifact(path="/tmp/model.ubj"),
                    deployable_model=ModelArtifact(path=f.name),
                ),
                storage_backend=backend,
                registry_client=_mock_registry(),
            ).execute()
        dep_key = backend.upload.call_args_list[1][0][1]
        self.assertEqual(dep_key, "models/m/deployable/model.tar")

    def test_raw_key_unaffected_by_source_path_shape(self):
        """A source path ending in a category-like segment doesn't alter the key.

        Regression: previously, a local download directory name could leak
        into the destination key, producing a doubled segment (e.g.
        ``.../raw/raw``) when the source artifact's own path also ended in
        ``raw``/``deployable``. The destination key must stay fixed no
        matter what the source path looks like.
        """
        backend = _mock_backend()
        ModelPusherPlugin(
            config=ModelPluginConfig(model_name="m"),
            artifact=AssembledModel(
                raw_model=ModelArtifact(path="/tmp/checkpoints/run1/raw")
            ),
            storage_backend=backend,
            registry_client=_mock_registry(),
        ).execute()
        raw_key = backend.upload.call_args_list[0][0][1]
        self.assertEqual(raw_key, "models/m/raw")

    def test_push_id_unique_across_calls(self):
        """Consecutive execute() calls produce different push_ids."""
        r1 = _plugin(model_name="m", backend=_mock_backend()).execute()
        r2 = _plugin(model_name="m", backend=_mock_backend()).execute()
        self.assertNotEqual(r1["push_id"], r2["push_id"])

    def test_downloaded_artifact_key_is_fixed(self):
        """A downloaded URI artifact's key is fixed, matching the local-path case."""
        backend = _mock_backend()

        def _fake_download(uri, local_path):
            with open(local_path, "w") as f:
                f.write("fake artifact content")

        backend.download.side_effect = _fake_download

        ModelPusherPlugin(
            config=ModelPluginConfig(model_name="m"),
            artifact=AssembledModel(
                raw_model=ModelArtifact(path="s3://bucket/tabular_assembler/x/raw")
            ),
            storage_backend=backend,
            registry_client=_mock_registry(),
        ).execute()
        raw_key = backend.upload.call_args_list[0][0][1]
        self.assertEqual(raw_key, "models/m/raw")


# ---------------------------------------------------------------------------
# model_name validation
# ---------------------------------------------------------------------------


class TestModelPusherPluginNameValidation(TestCase):
    """Tests for model_name handling edge cases."""

    def test_empty_string_model_name_raises(self):
        """It raises ConfigurationError when model_name is an empty string."""
        with self.assertRaises(ConfigurationError) as ctx:
            ModelPusherPlugin(
                config=ModelPluginConfig(model_name=""),
                artifact=_assembled(),
                storage_backend=_mock_backend(),
                registry_client=_mock_registry(),
            )
        self.assertIn("model_name", str(ctx.exception).lower())
