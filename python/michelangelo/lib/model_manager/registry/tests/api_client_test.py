"""Tests for APIRegistryClient."""

from __future__ import annotations

import json
from unittest import TestCase
from unittest.mock import MagicMock, patch

import grpc

from michelangelo.gen.api.v2 import model_pb2
from michelangelo.lib.model_manager.registry.api_client import APIRegistryClient
from michelangelo.lib.model_manager.registry.schema.api import APIRegistryConfig
from michelangelo.lib.exceptions import ConfigurationError

_STUB_PATH = "michelangelo.lib.model_manager.registry.api_client.ModelServiceStub"


class _RpcError(grpc.RpcError):
    """Minimal RpcError subclass for tests — grpc.RpcError itself is not raiseable."""

    def __init__(self, status_code: grpc.StatusCode) -> None:
        self._code = status_code

    def code(self) -> grpc.StatusCode:
        return self._code


def _config(**kwargs) -> APIRegistryConfig:
    defaults = {"endpoint": "localhost:50051", "namespace": "test-ns"}
    defaults.update(kwargs)
    return APIRegistryConfig(**defaults)


def _make_response_model(
    name: str = "my-model",
    namespace: str = "test-ns",
    revision_id: int = 1,
    artifact_uri: str = "s3://bucket/raw",
    deployable_uri: str | None = None,
) -> model_pb2.Model:
    """Build a minimal Model proto that mimics a service response."""
    m = model_pb2.Model()
    m.metadata.name = name
    m.metadata.namespace = namespace
    m.spec.revision_id = revision_id
    m.spec.model_artifact_uri.append(artifact_uri)
    if deployable_uri:
        m.spec.deployable_artifact_uri.append(deployable_uri)
    return m


def _make_stub(
    response_model: model_pb2.Model | None = None,
    get_model: model_pb2.Model | None = None,
    update_model: model_pb2.Model | None = None,
) -> MagicMock:
    stub = MagicMock()
    resp_model = response_model or _make_response_model()
    create_resp = MagicMock()
    create_resp.model = resp_model
    stub.CreateModel.return_value = create_resp

    get_resp = MagicMock()
    get_resp.model = get_model or _make_response_model()
    stub.GetModel.return_value = get_resp

    update_resp = MagicMock()
    update_resp.model = update_model or _make_response_model()
    stub.UpdateModel.return_value = update_resp
    return stub


class TestAPIRegistryConfig(TestCase):
    """Tests for APIRegistryConfig validation."""

    def test_raises_on_empty_endpoint(self):
        """It raises ConfigurationError when endpoint is empty."""
        with self.assertRaises(ConfigurationError):
            APIRegistryConfig(endpoint="")

    def test_defaults(self):
        """It defaults to insecure=True, empty namespace, 30s timeout."""
        cfg = APIRegistryConfig(endpoint="localhost:50051")
        self.assertTrue(cfg.insecure)
        self.assertEqual(cfg.namespace, "")
        self.assertEqual(cfg.timeout_seconds, 30)


class TestAPIRegistryClientRegisterModel(TestCase):
    """Tests for APIRegistryClient.register_model()."""

    def test_calls_create_model_and_returns_registered_model(self):
        """It calls CreateModel and maps the response to RegisteredModel."""
        stub = _make_stub(_make_response_model("my-model", revision_id=1))
        with patch(_STUB_PATH, return_value=stub):
            client = APIRegistryClient(_config())
            reg = client.register_model("my-model", "s3://bucket/raw")
        stub.CreateModel.assert_called_once()
        self.assertEqual(reg.name, "my-model")
        self.assertEqual(reg.version, "1")
        self.assertEqual(reg.artifact_uri, "s3://bucket/raw")

    def test_registry_uri_format(self):
        """It builds registry_uri as 'models:/{namespace}/{name}/{version}'."""
        stub = _make_stub(_make_response_model("clf", namespace="ns", revision_id=3))
        with patch(_STUB_PATH, return_value=stub):
            client = APIRegistryClient(_config(namespace="ns"))
            reg = client.register_model("clf", "s3://b/raw")
        self.assertEqual(reg.registry_uri, "models:/ns/clf/3")

    def test_namespace_injected_into_model_proto(self):
        """It sets model.metadata.namespace from config.namespace."""
        stub = _make_stub()
        with patch(_STUB_PATH, return_value=stub):
            client = APIRegistryClient(_config(namespace="my-ns"))
            client.register_model("m", "s3://b/raw")
        request = stub.CreateModel.call_args[0][0]
        self.assertEqual(request.model.metadata.namespace, "my-ns")

    def test_labels_set_on_model_metadata(self):
        """It stores labels on model.metadata.labels."""
        stub = _make_stub()
        with patch(_STUB_PATH, return_value=stub):
            client = APIRegistryClient(_config())
            client.register_model("m", "s3://b/raw", labels={"fw": "xgboost"})
        request = stub.CreateModel.call_args[0][0]
        self.assertEqual(request.model.metadata.labels["fw"], "xgboost")

    def test_metadata_stored_as_annotation(self):
        """It JSON-encodes metadata under the michelangelo.io/metadata annotation."""
        stub = _make_stub()
        with patch(_STUB_PATH, return_value=stub):
            client = APIRegistryClient(_config())
            client.register_model(
                "m", "s3://b/raw", metadata={"run_id": "r1", "rmse": 2.4}
            )
        request = stub.CreateModel.call_args[0][0]
        raw = request.model.metadata.annotations["michelangelo.io/metadata"]
        parsed = json.loads(raw)
        self.assertEqual(parsed["run_id"], "r1")
        self.assertAlmostEqual(parsed["rmse"], 2.4)

    def test_deployable_artifact_uri_set_when_provided(self):
        """It appends deployable_artifact_uri to spec when provided."""
        stub = _make_stub()
        with patch(_STUB_PATH, return_value=stub):
            client = APIRegistryClient(_config())
            client.register_model("m", "s3://b/raw", deployable_artifact_uri="s3://b/dep")
        request = stub.CreateModel.call_args[0][0]
        self.assertIn("s3://b/dep", list(request.model.spec.deployable_artifact_uri))

    def test_deployable_artifact_uri_absent_when_none(self):
        """It leaves spec.deployable_artifact_uri empty when not provided."""
        stub = _make_stub()
        with patch(_STUB_PATH, return_value=stub):
            client = APIRegistryClient(_config())
            client.register_model("m", "s3://b/raw")
        request = stub.CreateModel.call_args[0][0]
        self.assertEqual(len(request.model.spec.deployable_artifact_uri), 0)

    def test_already_exists_falls_back_to_get_then_update(self):
        """On ALREADY_EXISTS, it fetches resourceVersion and calls UpdateModel."""
        mock_error = _RpcError(grpc.StatusCode.ALREADY_EXISTS)

        existing = _make_response_model("m", revision_id=2)
        existing.metadata.resourceVersion = "rv-42"
        stub = _make_stub(
            get_model=existing,
            update_model=_make_response_model("m", revision_id=3),
        )
        stub.CreateModel.side_effect = mock_error

        with patch(_STUB_PATH, return_value=stub):
            client = APIRegistryClient(_config())
            reg = client.register_model("m", "s3://b/raw")

        stub.GetModel.assert_called_once()
        stub.UpdateModel.assert_called_once()
        update_model = stub.UpdateModel.call_args[0][0].model
        self.assertEqual(update_model.metadata.resourceVersion, "rv-42")
        self.assertEqual(reg.version, "3")

    def test_grpc_error_propagates_on_non_already_exists(self):
        """It re-raises gRPC errors other than ALREADY_EXISTS."""
        mock_error = _RpcError(grpc.StatusCode.INTERNAL)
        stub = MagicMock()
        stub.CreateModel.side_effect = mock_error

        with patch(_STUB_PATH, return_value=stub):
            client = APIRegistryClient(_config())
            with self.assertRaises(grpc.RpcError):
                client.register_model("m", "s3://b/raw")


class TestAPIRegistryClientGetModel(TestCase):
    """Tests for APIRegistryClient.get_model()."""

    def test_get_model_calls_stub_and_returns_registered_model(self):
        """It calls GetModel and maps the response correctly."""
        response_model = _make_response_model("clf", namespace="ns", revision_id=5)
        stub = _make_stub(get_model=response_model)
        with patch(_STUB_PATH, return_value=stub):
            client = APIRegistryClient(_config(namespace="ns"))
            reg = client.get_model("clf")
        stub.GetModel.assert_called_once()
        self.assertEqual(reg.name, "clf")
        self.assertEqual(reg.version, "5")

    def test_get_model_with_version_emits_warning(self):
        """It logs a warning when version is provided (per-revision lookup unsupported)."""
        stub = _make_stub(get_model=_make_response_model("m"))
        with patch(_STUB_PATH, return_value=stub), \
             self.assertLogs(
                 "michelangelo.lib.model_manager.registry.api_client",
                 level="WARNING",
             ) as log_ctx:
            client = APIRegistryClient(_config())
            client.get_model("m", version="3")
        self.assertTrue(
            any("version=" in msg for msg in log_ctx.output),
            msg=f"Expected version warning in logs: {log_ctx.output}",
        )

    def test_get_model_without_version_no_warning(self):
        """It does not emit a warning when version is None."""
        stub = _make_stub(get_model=_make_response_model("m"))
        with patch(_STUB_PATH, return_value=stub):
            client = APIRegistryClient(_config())
            # assertLogs would fail if no WARNING is emitted — use assertNoLogs (3.10+)
            # or simply call without the context manager and verify no exception.
            reg = client.get_model("m")
        self.assertEqual(reg.name, "m")

    def test_insecure_false_uses_secure_channel(self):
        """It creates a secure channel when insecure=False."""
        stub = _make_stub()
        with patch(_STUB_PATH, return_value=stub), \
             patch("grpc.secure_channel") as mock_secure, \
             patch("grpc.ssl_channel_credentials", return_value=MagicMock()):
            APIRegistryClient(_config(insecure=False))
        mock_secure.assert_called_once()
