"""Tests for the model registry client interface."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any
from unittest import TestCase

if TYPE_CHECKING:
    from collections.abc import Mapping

from michelangelo.lib.model_manager.registry.client import (
    ModelRegistryClient,
    RegisteredModel,
)


class TestRegisteredModel(TestCase):
    """Tests for the RegisteredModel dataclass."""

    def test_stores_required_fields(self):
        """It stores name, version, and registry_uri."""
        model = RegisteredModel(
            name="clf",
            version="1",
            registry_uri="models:/clf/1",
        )
        self.assertEqual(model.name, "clf")
        self.assertEqual(model.version, "1")
        self.assertEqual(model.registry_uri, "models:/clf/1")

    def test_labels_defaults_to_empty_dict(self):
        """It defaults labels to an empty dict when not provided."""
        model = RegisteredModel(name="m", version="1", registry_uri="uri")
        self.assertEqual(model.labels, {})

    def test_metadata_defaults_to_empty_dict(self):
        """It defaults metadata to an empty dict when not provided."""
        model = RegisteredModel(name="m", version="1", registry_uri="uri")
        self.assertEqual(model.metadata, {})

    def test_labels_instances_are_independent(self):
        """It creates a separate labels dict for each instance."""
        a = RegisteredModel(name="a", version="1", registry_uri="u1")
        b = RegisteredModel(name="b", version="1", registry_uri="u2")
        a.labels["k"] = "v"
        self.assertEqual(b.labels, {})

    def test_metadata_instances_are_independent(self):
        """It creates a separate metadata dict for each instance."""
        a = RegisteredModel(name="a", version="1", registry_uri="u1")
        b = RegisteredModel(name="b", version="1", registry_uri="u2")
        a.metadata["k"] = "v"
        self.assertEqual(b.metadata, {})

    def test_labels_can_be_provided(self):
        """It stores explicitly provided labels."""
        model = RegisteredModel(
            name="m",
            version="2",
            registry_uri="uri",
            labels={"training_framework": "xgboost"},
        )
        self.assertEqual(model.labels["training_framework"], "xgboost")

    def test_metadata_can_be_provided(self):
        """It stores explicitly provided metadata."""
        model = RegisteredModel(
            name="m",
            version="2",
            registry_uri="uri",
            metadata={"run_id": "mlflow-run-abc123", "accuracy": 0.94},
        )
        self.assertEqual(model.metadata["run_id"], "mlflow-run-abc123")
        self.assertEqual(model.metadata["accuracy"], 0.94)

    def test_artifact_uri_defaults_to_none(self):
        """It defaults artifact_uri to None when not provided."""
        model = RegisteredModel(name="m", version="1", registry_uri="uri")
        self.assertIsNone(model.artifact_uri)

    def test_deployable_artifact_uri_defaults_to_none(self):
        """It defaults deployable_artifact_uri to None when not provided."""
        model = RegisteredModel(name="m", version="1", registry_uri="uri")
        self.assertIsNone(model.deployable_artifact_uri)

    def test_artifact_uri_can_be_provided(self):
        """It stores an explicitly provided artifact_uri."""
        model = RegisteredModel(
            name="m",
            version="1",
            registry_uri="uri",
            artifact_uri="s3://bucket/raw",
        )
        self.assertEqual(model.artifact_uri, "s3://bucket/raw")

    def test_deployable_artifact_uri_can_be_provided(self):
        """It stores an explicitly provided deployable_artifact_uri."""
        model = RegisteredModel(
            name="m",
            version="1",
            registry_uri="uri",
            deployable_artifact_uri="s3://bucket/triton",
        )
        self.assertEqual(model.deployable_artifact_uri, "s3://bucket/triton")


class _ConcreteClient(ModelRegistryClient):
    """Minimal concrete implementation for testing the ABC."""

    def register_model(
        self,
        name: str,
        artifact_uri: str,
        deployable_artifact_uri: str | None = None,
        description: str | None = None,
        schema: dict[str, Any] | None = None,
        labels: Mapping[str, str] | None = None,
        metadata: Mapping[str, Any] | None = None,
    ) -> RegisteredModel:
        """Register a model and return a stub RegisteredModel."""
        return RegisteredModel(
            name=name,
            version="1",
            registry_uri=f"mem://{name}/1",
            labels=dict(labels or {}),
            metadata=dict(metadata or {}),
        )

    def get_model(self, name: str, version: str | None = None) -> RegisteredModel:
        """Return a stub RegisteredModel for the given name."""
        return RegisteredModel(name=name, version="1", registry_uri=f"mem://{name}/1")


class TestModelRegistryClientABC(TestCase):
    """Tests for the ModelRegistryClient abstract base class."""

    def test_cannot_be_instantiated_directly(self):
        """It raises TypeError when instantiated without abstract methods."""
        with self.assertRaises(TypeError):
            ModelRegistryClient()  # type: ignore[abstract]

    def test_missing_register_model_raises_type_error(self):
        """It raises TypeError when register_model is not implemented."""

        class PartialClient(ModelRegistryClient):
            def get_model(
                self, name: str, version: str | None = None
            ) -> RegisteredModel:
                return RegisteredModel(name=name, version="1", registry_uri="x")

        with self.assertRaises(TypeError):
            PartialClient()

    def test_missing_get_model_raises_type_error(self):
        """It raises TypeError when get_model is not implemented."""

        class PartialClient(ModelRegistryClient):
            def register_model(
                self, name: str, artifact_uri: str, **kwargs: Any
            ) -> RegisteredModel:
                return RegisteredModel(name=name, version="1", registry_uri="x")

        with self.assertRaises(TypeError):
            PartialClient()

    def test_full_implementation_can_be_instantiated(self):
        """It allows instantiation when all abstract methods are implemented."""
        client = _ConcreteClient()
        self.assertIsInstance(client, ModelRegistryClient)

    def test_register_model_returns_registered_model(self):
        """It returns a RegisteredModel from a concrete implementation."""
        client = _ConcreteClient()
        result = client.register_model(name="clf", artifact_uri="local:///tmp/raw")
        self.assertIsInstance(result, RegisteredModel)
        self.assertEqual(result.name, "clf")

    def test_register_model_stores_labels_and_metadata_separately(self):
        """Labels and metadata are stored in separate fields on RegisteredModel."""
        client = _ConcreteClient()
        result = client.register_model(
            name="clf",
            artifact_uri="s3://bucket/raw",
            labels={"training_framework": "xgboost"},
            metadata={"run_id": "run-abc123", "accuracy": 0.94},
        )
        self.assertEqual(result.labels["training_framework"], "xgboost")
        self.assertEqual(result.metadata["run_id"], "run-abc123")
        self.assertEqual(result.metadata["accuracy"], 0.94)
        self.assertNotIn("run_id", result.labels)

    def test_get_model_returns_registered_model(self):
        """It returns a RegisteredModel from a concrete get_model implementation."""
        client = _ConcreteClient()
        result = client.get_model(name="clf")
        self.assertIsInstance(result, RegisteredModel)


class TestInMemoryRegistryClient(TestCase):
    """Tests for InMemoryRegistryClient — the reference implementation."""

    def setUp(self) -> None:
        """Set up test fixtures."""
        from michelangelo.lib.model_manager.registry.client import (
            InMemoryRegistryClient,
        )

        self.registry = InMemoryRegistryClient()

    def test_register_model_returns_version_one_for_first_registration(self):
        """First registration of a name returns version '1'."""
        reg = self.registry.register_model(name="clf", artifact_uri="s3://raw")
        self.assertEqual(reg.version, "1")
        self.assertEqual(reg.name, "clf")

    def test_register_model_increments_version_per_name(self):
        """Each subsequent registration of the same name gets the next version."""
        self.registry.register_model(name="clf", artifact_uri="s3://v1")
        reg2 = self.registry.register_model(name="clf", artifact_uri="s3://v2")
        self.assertEqual(reg2.version, "2")

    def test_register_model_versions_are_independent_across_names(self):
        """Different model names have independent version counters."""
        self.registry.register_model(name="a", artifact_uri="s3://a1")
        self.registry.register_model(name="a", artifact_uri="s3://a2")
        reg_b = self.registry.register_model(name="b", artifact_uri="s3://b1")
        self.assertEqual(reg_b.version, "1")

    def test_register_model_stores_labels_and_metadata_separately(self):
        """Labels and metadata are stored in separate fields on the returned record."""
        reg = self.registry.register_model(
            name="clf",
            artifact_uri="s3://raw",
            labels={"framework": "xgboost"},
            metadata={"run_id": "run-abc"},
        )
        self.assertEqual(reg.labels["framework"], "xgboost")
        self.assertEqual(reg.metadata["run_id"], "run-abc")
        self.assertNotIn("run_id", reg.labels)

    def test_get_model_returns_latest_when_version_is_none(self):
        """get_model() returns the most recently registered version when version=None.

        The latest registration (highest version) is returned by default.
        """
        self.registry.register_model(name="clf", artifact_uri="s3://v1")
        self.registry.register_model(name="clf", artifact_uri="s3://v2")
        reg = self.registry.get_model(name="clf")
        self.assertEqual(reg.version, "2")
        self.assertEqual(reg.artifact_uri, "s3://v2")

    def test_get_model_returns_specific_version(self):
        """get_model(version='1') returns the first registration, not the latest."""
        self.registry.register_model(name="clf", artifact_uri="s3://v1")
        self.registry.register_model(name="clf", artifact_uri="s3://v2")
        reg = self.registry.get_model(name="clf", version="1")
        self.assertEqual(reg.version, "1")
        self.assertEqual(reg.artifact_uri, "s3://v1")

    def test_get_model_raises_key_error_for_unknown_name(self):
        """get_model() raises KeyError when the model name has never been registered."""
        with self.assertRaises(KeyError) as ctx:
            self.registry.get_model(name="does-not-exist")
        self.assertIn("does-not-exist", str(ctx.exception))

    def test_get_model_raises_key_error_for_unknown_version(self):
        """get_model() raises KeyError when the version does not exist."""
        self.registry.register_model(name="clf", artifact_uri="s3://v1")
        with self.assertRaises(KeyError) as ctx:
            self.registry.get_model(name="clf", version="99")
        self.assertIn("99", str(ctx.exception))
