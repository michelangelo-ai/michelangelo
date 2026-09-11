"""Tests for MLflowRegistryClient against a faked ``MlflowClient``.

No live MLflow server: the ``MlflowClient`` constructor is patched with an
in-memory fake that mimics the registry semantics the client relies on
(duplicate-name error on ``create_registered_model``, monotonically
increasing version numbers, name-scoped search). MLflow itself does not
need to be installed: when it is absent, minimal stand-in modules exposing
``MlflowException`` and the error-code constants are planted in
``sys.modules`` for the duration of this module, so the suite runs the
same everywhere (the module under test only imports mlflow lazily, which
``TestMlflowNotInstalled`` covers by patching it away).
"""

from __future__ import annotations

import re
import sys
from types import ModuleType, SimpleNamespace
from unittest import TestCase
from unittest.mock import MagicMock, patch

from michelangelo.lib.model_manager.registry.client import (
    ModelRegistryClient,
    RegisteredModel,
)
from michelangelo.workflow.tasks.pusher.implementations.mlflow_client import (
    _TAG_DEPLOYABLE_ARTIFACT_URI,
    _TAG_KIND,
    _TAG_METADATA,
    MLflowRegistryClient,
)

try:
    from mlflow.exceptions import MlflowException
    from mlflow.protos.databricks_pb2 import (
        INTERNAL_ERROR,
        INVALID_PARAMETER_VALUE,
        RESOURCE_ALREADY_EXISTS,
        RESOURCE_DOES_NOT_EXIST,
    )

    _MLFLOW_STUBBED = False
except ImportError:
    # mlflow is optional (the ``pusher-mlflow`` extra) and not installed in
    # the default CI environment. Mirror the pieces the tests and the module
    # under test touch: the proto error-code values and an exception whose
    # ``error_code`` attribute is the code's string name, exactly like the
    # real ``MlflowException``.
    INTERNAL_ERROR = 1
    INVALID_PARAMETER_VALUE = 1000
    RESOURCE_ALREADY_EXISTS = 3001
    RESOURCE_DOES_NOT_EXIST = 3002
    _ERROR_CODE_NAMES = {
        INTERNAL_ERROR: "INTERNAL_ERROR",
        INVALID_PARAMETER_VALUE: "INVALID_PARAMETER_VALUE",
        RESOURCE_ALREADY_EXISTS: "RESOURCE_ALREADY_EXISTS",
        RESOURCE_DOES_NOT_EXIST: "RESOURCE_DOES_NOT_EXIST",
    }

    class MlflowException(Exception):  # noqa: N818 - mirrors the real name
        """Stand-in matching the real exception's ``error_code`` contract."""

        def __init__(self, message, error_code=None):
            """Store the code's string name on ``error_code``."""
            super().__init__(message)
            self.error_code = _ERROR_CODE_NAMES.get(error_code, error_code)

    _MLFLOW_STUBBED = True

_STUB_MODULE_NAMES = (
    "mlflow",
    "mlflow.tracking",
    "mlflow.exceptions",
    "mlflow.protos",
    "mlflow.protos.databricks_pb2",
)


def _unpatched_client(*args, **kwargs):
    raise AssertionError("tests must patch mlflow.tracking.MlflowClient before use")


def setUpModule():
    """Plant stub mlflow modules when the real package is not installed.

    ``patch("mlflow.tracking.MlflowClient", ...)`` needs the module to exist
    in ``sys.modules``, and the module under test lazily imports
    ``MlflowException`` from ``mlflow.exceptions`` at call time. Scoped to
    this module so other tests' ``importorskip("mlflow")`` behavior is
    unaffected.
    """
    if not _MLFLOW_STUBBED:
        return
    tracking = ModuleType("mlflow.tracking")
    tracking.MlflowClient = _unpatched_client
    exceptions = ModuleType("mlflow.exceptions")
    exceptions.MlflowException = MlflowException
    databricks_pb2 = ModuleType("mlflow.protos.databricks_pb2")
    for code, name in _ERROR_CODE_NAMES.items():
        setattr(databricks_pb2, name, code)
    protos = ModuleType("mlflow.protos")
    protos.databricks_pb2 = databricks_pb2
    mlflow_stub = ModuleType("mlflow")
    mlflow_stub.tracking = tracking
    mlflow_stub.exceptions = exceptions
    mlflow_stub.protos = protos
    for name, module in zip(
        _STUB_MODULE_NAMES,
        (mlflow_stub, tracking, exceptions, protos, databricks_pb2),
    ):
        sys.modules[name] = module


def tearDownModule():
    """Remove the stub modules so nothing outlives this module's tests."""
    if not _MLFLOW_STUBBED:
        return
    for name in _STUB_MODULE_NAMES:
        sys.modules.pop(name, None)


_FILTER_RE = re.compile(r"""name = (?:'([^']*)'|"([^"]*)")""")


class _FakeMlflowClient:
    """In-memory stand-in for ``mlflow.tracking.MlflowClient``.

    Implements the four registry calls the client makes, with the error
    semantics the client's code paths depend on: duplicate registered-model
    creation raises ``RESOURCE_ALREADY_EXISTS``, unknown lookups raise
    ``RESOURCE_DOES_NOT_EXIST``, and ``search_model_versions`` honors the
    ``name = <literal>`` filter (parsed with a regex) so name isolation is
    actually exercised.
    """

    def __init__(self, tracking_uri=None, registry_uri=None):
        self.tracking_uri = tracking_uri
        self.registry_uri = registry_uri
        self.models: dict[str, list[SimpleNamespace]] = {}
        self.create_version_kwargs: list[dict] = []
        self.last_search_kwargs: dict | None = None

    def create_registered_model(self, name):
        if name in self.models:
            raise MlflowException(
                f"Registered Model (name={name}) already exists.",
                error_code=RESOURCE_ALREADY_EXISTS,
            )
        self.models[name] = []
        return SimpleNamespace(name=name)

    def create_model_version(
        self, name, source, run_id=None, tags=None, description=None
    ):
        self.create_version_kwargs.append(
            {
                "name": name,
                "source": source,
                "run_id": run_id,
                "tags": dict(tags or {}),
                "description": description,
            }
        )
        versions = self.models.setdefault(name, [])
        model_version = SimpleNamespace(
            name=name,
            version=str(len(versions) + 1),
            source=source,
            run_id=run_id,
            tags=dict(tags or {}),
            description=description,
        )
        versions.append(model_version)
        return model_version

    def get_model_version(self, name, version):
        for model_version in self.models.get(name, []):
            if model_version.version == version:
                return model_version
        raise MlflowException(
            f"Model Version (name={name}, version={version}) not found",
            error_code=RESOURCE_DOES_NOT_EXIST,
        )

    def search_model_versions(self, filter_string, order_by, max_results):
        self.last_search_kwargs = {
            "filter_string": filter_string,
            "order_by": order_by,
            "max_results": max_results,
        }
        match = _FILTER_RE.fullmatch(filter_string)
        assert match, f"fake got unparseable filter: {filter_string!r}"
        name = match.group(1) if match.group(1) is not None else match.group(2)
        versions = sorted(
            self.models.get(name, []), key=lambda v: int(v.version), reverse=True
        )
        return versions[:max_results]


class _FakeClientTestCase(TestCase):
    """Base fixture: a fresh fake patched in as the ``MlflowClient`` constructor."""

    def setUp(self):
        super().setUp()
        self.fake = _FakeMlflowClient()
        patcher = patch("mlflow.tracking.MlflowClient", return_value=self.fake)
        patcher.start()
        self.addCleanup(patcher.stop)
        self.client = MLflowRegistryClient()


class TestProtocolConformance(TestCase):
    """The client satisfies and exports the ModelRegistryClient seam."""

    def test_is_model_registry_client(self):
        """The client subclasses the ABC and is exported from the pusher package."""
        self.assertIsInstance(MLflowRegistryClient(), ModelRegistryClient)
        import michelangelo.workflow.tasks.pusher as pusher_pkg

        self.assertIs(pusher_pkg.MLflowRegistryClient, MLflowRegistryClient)


class TestRegisterModel(_FakeClientTestCase):
    """register_model(): version creation, tag mapping, and error contracts."""

    def test_basic_registration(self):
        """First registration creates the model group and version 1."""
        registered = self.client.register_model(name="m", artifact_uri="s3://b/raw")
        self.assertEqual(registered.name, "m")
        self.assertEqual(registered.version, "1")
        self.assertEqual(registered.registry_uri, "models:/m/1")
        self.assertEqual(registered.artifact_uri, "s3://b/raw")
        self.assertIsNone(registered.deployable_artifact_uri)
        self.assertIsNone(registered.kind)
        self.assertEqual(registered.labels, {})
        self.assertEqual(registered.metadata, {})
        self.assertIsInstance(registered, RegisteredModel)

    def test_fields_travel_as_tags(self):
        """Deployable URI, kind, and metadata become reserved model-version tags."""
        self.client.register_model(
            name="m",
            artifact_uri="s3://b/raw",
            deployable_artifact_uri="s3://b/deploy",
            description="a model",
            kind="regression",
            labels={"owner": "ml-platform"},
            metadata={"accuracy": 0.94},
        )
        kwargs = self.fake.create_version_kwargs[-1]
        self.assertEqual(kwargs["source"], "s3://b/raw")
        self.assertEqual(kwargs["description"], "a model")
        self.assertEqual(kwargs["tags"]["owner"], "ml-platform")
        self.assertEqual(kwargs["tags"][_TAG_DEPLOYABLE_ARTIFACT_URI], "s3://b/deploy")
        self.assertEqual(kwargs["tags"][_TAG_KIND], "regression")
        self.assertEqual(kwargs["tags"][_TAG_METADATA], '{"accuracy": 0.94}')

    def test_no_reserved_tags_for_absent_fields(self):
        """Optional fields left unset add no tags at all."""
        self.client.register_model(name="m", artifact_uri="s3://b/raw")
        self.assertEqual(self.fake.create_version_kwargs[-1]["tags"], {})

    def test_run_id_uses_native_linkage(self):
        """``metadata["run_id"]`` goes to MLflow's run linkage, not the tag."""
        registered = self.client.register_model(
            name="m",
            artifact_uri="s3://b/raw",
            metadata={"run_id": "abc123", "accuracy": 0.94},
        )
        kwargs = self.fake.create_version_kwargs[-1]
        self.assertEqual(kwargs["run_id"], "abc123")
        self.assertEqual(kwargs["tags"][_TAG_METADATA], '{"accuracy": 0.94}')
        # The returned record still mirrors the full metadata argument.
        self.assertEqual(registered.metadata, {"run_id": "abc123", "accuracy": 0.94})

    def test_non_string_run_id_stays_in_metadata_tag(self):
        """A non-string run_id cannot be linked natively and stays in the tag."""
        self.client.register_model(
            name="m", artifact_uri="s3://b/raw", metadata={"run_id": 42}
        )
        kwargs = self.fake.create_version_kwargs[-1]
        self.assertIsNone(kwargs["run_id"])
        self.assertEqual(kwargs["tags"][_TAG_METADATA], '{"run_id": 42}')

    def test_existing_model_group_is_reused(self):
        """A second registration under the same name gets version 2."""
        self.client.register_model(name="m", artifact_uri="s3://b/v1")
        registered = self.client.register_model(name="m", artifact_uri="s3://b/v2")
        self.assertEqual(registered.version, "2")
        self.assertEqual(registered.registry_uri, "models:/m/2")

    def test_schema_is_silently_ignored(self):
        """Passing a schema neither raises nor adds a tag, per the ABC contract."""
        self.client.register_model(
            name="m", artifact_uri="s3://b/raw", schema={"inputs": []}
        )
        self.assertEqual(self.fake.create_version_kwargs[-1]["tags"], {})

    def test_reserved_tag_collision_resolves_to_reserved_value(self):
        """A user label colliding with a reserved tag key loses to the field."""
        self.client.register_model(
            name="m",
            artifact_uri="s3://b/raw",
            kind="regression",
            labels={_TAG_KIND: "user-value"},
        )
        self.assertEqual(
            self.fake.create_version_kwargs[-1]["tags"][_TAG_KIND], "regression"
        )

    def test_invalid_name_raises_value_error(self):
        """MLflow's INVALID_PARAMETER_VALUE maps to ValueError per the ABC."""
        broken = MagicMock()
        broken.create_registered_model.side_effect = MlflowException(
            "bad name", error_code=INVALID_PARAMETER_VALUE
        )
        with (
            patch("mlflow.tracking.MlflowClient", return_value=broken),
            self.assertRaises(ValueError),
        ):
            self.client.register_model(name="bad/../name", artifact_uri="s3://b")

    def test_other_registry_errors_propagate(self):
        """Errors other than already-exists/invalid-parameter are not swallowed."""
        broken = MagicMock()
        broken.create_registered_model.side_effect = MlflowException(
            "boom", error_code=INTERNAL_ERROR
        )
        with (
            patch("mlflow.tracking.MlflowClient", return_value=broken),
            self.assertRaises(MlflowException),
        ):
            self.client.register_model(name="m", artifact_uri="s3://b")

    def test_invalid_version_source_raises_value_error(self):
        """INVALID_PARAMETER_VALUE from version creation also maps to ValueError."""
        broken = MagicMock()
        broken.create_model_version.side_effect = MlflowException(
            "bad source", error_code=INVALID_PARAMETER_VALUE
        )
        with (
            patch("mlflow.tracking.MlflowClient", return_value=broken),
            self.assertRaises(ValueError),
        ):
            self.client.register_model(name="m", artifact_uri="")

    def test_version_creation_errors_propagate(self):
        """Non-validation errors from version creation are not swallowed."""
        broken = MagicMock()
        broken.create_model_version.side_effect = MlflowException(
            "boom", error_code=INTERNAL_ERROR
        )
        with (
            patch("mlflow.tracking.MlflowClient", return_value=broken),
            self.assertRaises(MlflowException),
        ):
            self.client.register_model(name="m", artifact_uri="s3://b")


class TestGetModel(_FakeClientTestCase):
    """get_model(): explicit-version and latest-version lookup paths."""

    def test_round_trip_with_explicit_version(self):
        """A registration is retrievable by exact version with all fields intact."""
        self.client.register_model(
            name="m",
            artifact_uri="s3://b/raw",
            deployable_artifact_uri="s3://b/deploy",
            kind="regression",
            labels={"owner": "ml-platform"},
            metadata={"run_id": "abc123", "accuracy": 0.94},
        )
        registered = self.client.get_model("m", version="1")
        self.assertEqual(registered.name, "m")
        self.assertEqual(registered.version, "1")
        self.assertEqual(registered.registry_uri, "models:/m/1")
        self.assertEqual(registered.artifact_uri, "s3://b/raw")
        self.assertEqual(registered.deployable_artifact_uri, "s3://b/deploy")
        self.assertEqual(registered.kind, "regression")
        self.assertEqual(registered.labels, {"owner": "ml-platform"})
        self.assertEqual(registered.metadata, {"run_id": "abc123", "accuracy": 0.94})

    def test_latest_version_wins_without_explicit_version(self):
        """``version=None`` returns the highest version number."""
        self.client.register_model(name="m", artifact_uri="s3://b/v1")
        self.client.register_model(name="m", artifact_uri="s3://b/v2")
        registered = self.client.get_model("m")
        self.assertEqual(registered.version, "2")
        self.assertEqual(registered.artifact_uri, "s3://b/v2")
        kwargs = self.fake.last_search_kwargs
        self.assertEqual(kwargs["filter_string"], "name = 'm'")
        self.assertEqual(kwargs["order_by"], ["version_number DESC"])
        self.assertEqual(kwargs["max_results"], 1)

    def test_names_are_isolated(self):
        """Latest-version lookup never crosses model names."""
        self.client.register_model(name="a", artifact_uri="s3://b/a1")
        self.client.register_model(name="b", artifact_uri="s3://b/b1")
        self.client.register_model(name="b", artifact_uri="s3://b/b2")
        self.assertEqual(self.client.get_model("a").artifact_uri, "s3://b/a1")
        self.assertEqual(self.client.get_model("b").artifact_uri, "s3://b/b2")

    def test_single_quote_name_switches_to_double_quoted_literal(self):
        """MLflow filters cannot escape quotes; the literal's quote style flips."""
        self.client.register_model(name="it's m", artifact_uri="s3://b/raw")
        registered = self.client.get_model("it's m")
        self.assertEqual(registered.artifact_uri, "s3://b/raw")
        self.assertIn('name = "it\'s m"', self.fake.last_search_kwargs["filter_string"])

    def test_both_quote_styles_raise_key_error_without_search(self):
        """An unfilterable name fails loudly for latest-version lookup..."""
        tricky = 'it\'s "m"'
        self.client.register_model(name=tricky, artifact_uri="s3://b/raw")
        with self.assertRaises(KeyError):
            self.client.get_model(tricky)
        self.assertIsNone(self.fake.last_search_kwargs)

    def test_both_quote_styles_still_work_with_explicit_version(self):
        """...but the explicit-version path bypasses the filter entirely."""
        tricky = 'it\'s "m"'
        self.client.register_model(name=tricky, artifact_uri="s3://b/raw")
        registered = self.client.get_model(tricky, version="1")
        self.assertEqual(registered.artifact_uri, "s3://b/raw")

    def test_missing_model_raises_key_error(self):
        """Unknown names raise KeyError on both lookup paths."""
        with self.assertRaises(KeyError):
            self.client.get_model("ghost")
        with self.assertRaises(KeyError):
            self.client.get_model("ghost", version="1")

    def test_missing_version_raises_key_error(self):
        """A known name with an unknown version raises KeyError."""
        self.client.register_model(name="m", artifact_uri="s3://b/raw")
        with self.assertRaises(KeyError):
            self.client.get_model("m", version="99")

    def test_other_registry_errors_propagate(self):
        """Errors other than not-found are not converted to KeyError."""
        broken = MagicMock()
        broken.get_model_version.side_effect = MlflowException(
            "boom", error_code=INTERNAL_ERROR
        )
        with (
            patch("mlflow.tracking.MlflowClient", return_value=broken),
            self.assertRaises(MlflowException),
        ):
            self.client.get_model("m", version="1")


class TestMlflowNotInstalled(TestCase):
    """The lazy import fails with an actionable message when mlflow is absent."""

    def test_import_error_names_the_extra(self):
        """Without mlflow installed, use raises ImportError naming pusher-mlflow."""
        client = MLflowRegistryClient()
        import sys

        # A None module entry makes ``from mlflow.tracking import MlflowClient``
        # raise ImportError, simulating the extra not being installed.
        with patch.dict(sys.modules, {"mlflow.tracking": None}):
            with self.assertRaisesRegex(ImportError, "pusher-mlflow"):
                client.register_model(name="m", artifact_uri="s3://b")
            with self.assertRaisesRegex(ImportError, "pusher-mlflow"):
                client.get_model("m")


class TestConstruction(TestCase):
    """Constructor arguments flow through; the client stays picklable."""

    def test_uris_forwarded_to_mlflow_client(self):
        """tracking_uri and registry_uri reach the MlflowClient constructor."""
        client = MLflowRegistryClient(
            tracking_uri="http://tracking.example.com",
            registry_uri="http://registry.example.com",
        )
        constructor = MagicMock()
        constructor.return_value.get_model_version.side_effect = MlflowException(
            "nope", error_code=RESOURCE_DOES_NOT_EXIST
        )
        with (
            patch("mlflow.tracking.MlflowClient", constructor),
            self.assertRaises(KeyError),
        ):
            client.get_model("m", version="1")
        constructor.assert_called_once_with(
            tracking_uri="http://tracking.example.com",
            registry_uri="http://registry.example.com",
        )

    def test_client_is_picklable(self):
        """The client holds only plain strings, so it pickles cleanly."""
        import pickle

        restored = pickle.loads(
            pickle.dumps(MLflowRegistryClient(tracking_uri="http://t"))
        )
        self.assertIsInstance(restored, MLflowRegistryClient)
        self.assertEqual(restored._tracking_uri, "http://t")
