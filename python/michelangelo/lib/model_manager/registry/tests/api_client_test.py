"""Tests for APIRegistryClient."""

from __future__ import annotations

import json
from unittest import TestCase
from unittest.mock import MagicMock

import grpc

from michelangelo.gen.api.v2 import model_pb2
from michelangelo.lib.model_manager.registry.api_client import (
    METADATA_ANNOTATION_KEY,
    APIRegistryClient,
)


class _RpcError(grpc.RpcError):
    """Minimal RpcError subclass — grpc.RpcError itself is not raiseable."""

    def __init__(self, status_code: grpc.StatusCode) -> None:
        self._code = status_code

    def code(self) -> grpc.StatusCode:
        return self._code


def _model(
    name: str = "my-model",
    namespace: str = "default",
    revision_id: int = 1,
    artifact_uri: str = "s3://bucket/raw/model.ubj",
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


def _mock_svc(
    create_return: model_pb2.Model | None = None,
    get_return: model_pb2.Model | None = None,
    update_return: model_pb2.Model | None = None,
) -> MagicMock:
    """Return a mock ModelService with sensible defaults."""
    svc = MagicMock()
    svc.create_model.return_value = create_return or _model()
    svc.get_model.return_value = get_return or _model()
    svc.update_model.return_value = update_return or _model()
    return svc


def _client(svc=None, namespace="default") -> APIRegistryClient:
    return APIRegistryClient(svc=svc or _mock_svc(), namespace=namespace)


# ---------------------------------------------------------------------------
# Constructor
# ---------------------------------------------------------------------------


class TestAPIRegistryClientConstructor(TestCase):
    """Tests for APIRegistryClient.__init__()."""

    def test_accepts_explicit_svc(self):
        """It stores the supplied ModelService and does not call APIClient."""
        svc = _mock_svc()
        client = APIRegistryClient(svc=svc, namespace="ns")
        self.assertIs(client._svc, svc)
        self.assertEqual(client._namespace, "ns")

    def test_default_namespace_is_default(self):
        """Namespace defaults to 'default' when not supplied."""
        client = APIRegistryClient(svc=_mock_svc())
        self.assertEqual(client._namespace, "default")

    def test_singleton_raises_when_model_service_not_initialised(self):
        """It raises RuntimeError when APIClient.ModelService is None."""
        import unittest.mock as mock

        with mock.patch("michelangelo.api.v2.APIClient") as mock_api:
            mock_api.ModelService = None
            with self.assertRaises(RuntimeError) as ctx:
                APIRegistryClient()
            self.assertIn("MA_API_SERVER", str(ctx.exception))


# ---------------------------------------------------------------------------
# register_model — happy path
# ---------------------------------------------------------------------------


class TestAPIRegistryClientRegisterModel(TestCase):
    """Tests for APIRegistryClient.register_model() happy path."""

    def test_calls_create_model_and_returns_registered_model(self):
        """It calls create_model() and maps the response to RegisteredModel."""
        svc = _mock_svc(_model("my-model", revision_id=1))
        reg = _client(svc).register_model("my-model", "s3://bucket/raw/model.ubj")
        svc.create_model.assert_called_once()
        self.assertEqual(reg.name, "my-model")
        self.assertEqual(reg.version, "1")
        self.assertEqual(reg.artifact_uri, "s3://bucket/raw/model.ubj")

    def test_registry_uri_format(self):
        """registry_uri uses models:/{namespace}/{name}/{version} format."""
        svc = _mock_svc(_model("clf", namespace="ns", revision_id=3))
        reg = _client(svc, namespace="ns").register_model("clf", "s3://b/raw")
        self.assertEqual(reg.registry_uri, "models:/ns/clf/3")

    def test_namespace_set_on_model_proto(self):
        """The namespace is written to model.metadata.namespace."""
        svc = _mock_svc()
        _client(svc, namespace="my-ns").register_model("m", "s3://b/raw")
        created_model = svc.create_model.call_args[0][0]
        self.assertEqual(created_model.metadata.namespace, "my-ns")

    def test_labels_set_on_model_metadata(self):
        """Labels are stored in model.metadata.labels."""
        svc = _mock_svc()
        _client(svc).register_model("m", "s3://b/raw", labels={"fw": "xgboost"})
        created_model = svc.create_model.call_args[0][0]
        self.assertEqual(created_model.metadata.labels["fw"], "xgboost")

    def test_oversized_label_value_demoted_to_metadata_annotation(self):
        """A label value over 63 chars is moved to the metadata annotation.

        Regression test: ModelMetadata.to_registry_dict() commonly emits a
        fully-qualified Python class path (e.g. `model_class`) that exceeds
        Kubernetes' 63-character label-value limit. Writing it straight to
        model.metadata.labels made create_model()/update_model() fail with
        INVALID_ARGUMENT against a real apiserver.
        """
        svc = _mock_svc()
        long_value = (
            "examples.pipelines.california_housing_lightning.model.TorchRegressionModel"
        )
        self.assertGreater(len(long_value), 63)
        _client(svc).register_model(
            "m", "s3://b/raw", labels={"model_class": long_value, "fw": "lightning"}
        )
        created_model = svc.create_model.call_args[0][0]
        self.assertNotIn("model_class", created_model.metadata.labels)
        self.assertEqual(created_model.metadata.labels["fw"], "lightning")
        annotation = json.loads(
            created_model.metadata.annotations[METADATA_ANNOTATION_KEY]
        )
        self.assertEqual(annotation["model_class"], long_value)

    def test_label_value_with_invalid_characters_demoted_to_metadata_annotation(self):
        """A label value with disallowed characters is moved to the annotation."""
        svc = _mock_svc()
        _client(svc).register_model("m", "s3://b/raw", labels={"note": "50% faster!"})
        created_model = svc.create_model.call_args[0][0]
        self.assertNotIn("note", created_model.metadata.labels)
        annotation = json.loads(
            created_model.metadata.annotations[METADATA_ANNOTATION_KEY]
        )
        self.assertEqual(annotation["note"], "50% faster!")

    def test_demoted_label_does_not_override_explicit_metadata(self):
        """An explicit metadata key wins over a same-named demoted label."""
        svc = _mock_svc()
        long_value = "a" * 64
        _client(svc).register_model(
            "m",
            "s3://b/raw",
            labels={"model_class": long_value},
            metadata={"model_class": "explicit-value"},
        )
        created_model = svc.create_model.call_args[0][0]
        annotation = json.loads(
            created_model.metadata.annotations[METADATA_ANNOTATION_KEY]
        )
        self.assertEqual(annotation["model_class"], "explicit-value")

    def test_metadata_stored_as_annotation(self):
        """Metadata is JSON-encoded under the michelangelo.io/metadata annotation."""
        svc = _mock_svc()
        _client(svc).register_model(
            "m", "s3://b/raw", metadata={"run_id": "r1", "rmse": 2.4}
        )
        created_model = svc.create_model.call_args[0][0]
        raw = created_model.metadata.annotations[METADATA_ANNOTATION_KEY]
        parsed = json.loads(raw)
        self.assertEqual(parsed["run_id"], "r1")
        self.assertAlmostEqual(parsed["rmse"], 2.4)

    def test_deployable_artifact_uri_set_when_provided(self):
        """deployable_artifact_uri is appended to spec when provided."""
        svc = _mock_svc()
        _client(svc).register_model(
            "m", "s3://b/raw", deployable_artifact_uri="s3://b/dep"
        )
        created_model = svc.create_model.call_args[0][0]
        self.assertIn("s3://b/dep", list(created_model.spec.deployable_artifact_uri))

    def test_deployable_artifact_uri_absent_when_none(self):
        """spec.deployable_artifact_uri is empty when not provided."""
        svc = _mock_svc()
        _client(svc).register_model("m", "s3://b/raw")
        created_model = svc.create_model.call_args[0][0]
        self.assertEqual(len(created_model.spec.deployable_artifact_uri), 0)


# ---------------------------------------------------------------------------
# register_model — ALREADY_EXISTS retry
# ---------------------------------------------------------------------------


class TestAPIRegistryClientAlreadyExists(TestCase):
    """Tests for the ALREADY_EXISTS → get + update retry path."""

    def test_falls_back_to_get_then_update_on_already_exists(self):
        """On ALREADY_EXISTS it fetches resourceVersion and calls update_model."""
        existing = _model("m", revision_id=2)
        existing.metadata.resourceVersion = "rv-42"

        svc = _mock_svc(
            get_return=existing,
            update_return=_model("m", revision_id=3),
        )
        svc.create_model.side_effect = _RpcError(grpc.StatusCode.ALREADY_EXISTS)

        reg = _client(svc).register_model("m", "s3://b/raw")

        svc.get_model.assert_called_once()
        svc.update_model.assert_called_once()
        self.assertEqual(reg.version, "3")

    def test_resource_version_propagated_to_update(self):
        """The resourceVersion from get_model is set on the update proto."""
        existing = _model("m")
        existing.metadata.resourceVersion = "rv-99"

        svc = _mock_svc(get_return=existing)
        svc.create_model.side_effect = _RpcError(grpc.StatusCode.ALREADY_EXISTS)

        _client(svc).register_model("m", "s3://b/raw")

        updated_model = svc.update_model.call_args[0][0]
        self.assertEqual(updated_model.metadata.resourceVersion, "rv-99")

    def test_non_already_exists_error_is_reraised(self):
        """Non-ALREADY_EXISTS gRPC errors propagate immediately."""
        svc = _mock_svc()
        svc.create_model.side_effect = _RpcError(grpc.StatusCode.UNAVAILABLE)
        with self.assertRaises(grpc.RpcError):
            _client(svc).register_model("m", "s3://b/raw")
        svc.get_model.assert_not_called()

    def test_failed_precondition_retries_up_to_max(self):
        """FAILED_PRECONDITION on update retries the full sequence."""
        existing = _model("m")
        svc = _mock_svc(
            get_return=existing,
            update_return=_model("m", revision_id=5),
        )
        svc.create_model.side_effect = _RpcError(grpc.StatusCode.ALREADY_EXISTS)
        # Fail twice with FAILED_PRECONDITION, succeed on third attempt.
        svc.update_model.side_effect = [
            _RpcError(grpc.StatusCode.FAILED_PRECONDITION),
            _RpcError(grpc.StatusCode.FAILED_PRECONDITION),
            _model("m", revision_id=5),
        ]

        reg = _client(svc).register_model("m", "s3://b/raw")
        self.assertEqual(reg.version, "5")
        self.assertEqual(svc.update_model.call_count, 3)

    def test_exhausted_retries_raises_runtime_error(self):
        """Exhausting all FAILED_PRECONDITION retries raises RuntimeError."""
        svc = _mock_svc()
        svc.create_model.side_effect = _RpcError(grpc.StatusCode.ALREADY_EXISTS)
        svc.update_model.side_effect = _RpcError(grpc.StatusCode.FAILED_PRECONDITION)

        with self.assertRaises(RuntimeError):
            _client(svc).register_model("m", "s3://b/raw")


# ---------------------------------------------------------------------------
# get_model
# ---------------------------------------------------------------------------


class TestAPIRegistryClientGetModel(TestCase):
    """Tests for APIRegistryClient.get_model()."""

    def test_calls_get_model_and_returns_registered_model(self):
        """It delegates to svc.get_model() and maps the response."""
        svc = _mock_svc(get_return=_model("clf", revision_id=7))
        reg = _client(svc, namespace="ns").get_model("clf")
        svc.get_model.assert_called_once_with("ns", "clf")
        self.assertEqual(reg.name, "clf")
        self.assertEqual(reg.version, "7")

    def test_version_arg_emits_warning(self):
        """Passing a version emits a log warning (per-revision lookup unsupported)."""
        svc = _mock_svc()
        with self.assertLogs(
            "michelangelo.lib.model_manager.registry.api_client", level="WARNING"
        ) as cm:
            _client(svc).get_model("m", version="2")
        self.assertTrue(any("version" in msg.lower() for msg in cm.output))
