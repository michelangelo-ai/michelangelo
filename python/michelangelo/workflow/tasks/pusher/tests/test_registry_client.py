"""Tests for the model registry client module."""

from __future__ import annotations

from typing import Any
from unittest import TestCase

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

    def test_metadata_defaults_to_empty_dict(self):
        """It defaults metadata to an empty dict when not provided."""
        model = RegisteredModel(name="m", version="1", registry_uri="uri")
        self.assertEqual(model.metadata, {})

    def test_metadata_instances_are_independent(self):
        """It creates a separate metadata dict for each instance."""
        a = RegisteredModel(name="a", version="1", registry_uri="u1")
        b = RegisteredModel(name="b", version="1", registry_uri="u2")
        a.metadata["k"] = "v"
        self.assertEqual(b.metadata, {})

    def test_metadata_can_be_provided(self):
        """It stores explicitly provided metadata."""
        model = RegisteredModel(
            name="m",
            version="2",
            registry_uri="uri",
            metadata={"framework": "xgboost"},
        )
        self.assertEqual(model.metadata["framework"], "xgboost")

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
        schema: dict[str, Any] | None = None,
        metadata: dict[str, str] | None = None,
    ) -> RegisteredModel:
        """Register a model and return a stub RegisteredModel."""
        return RegisteredModel(name=name, version="1", registry_uri=f"mem://{name}/1")

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
        result = client.register_model(
            name="clf",
            artifact_uri="local:///tmp/raw",
        )
        self.assertIsInstance(result, RegisteredModel)
        self.assertEqual(result.name, "clf")

    def test_get_model_returns_registered_model(self):
        """It returns a RegisteredModel from a concrete get_model implementation."""
        client = _ConcreteClient()
        result = client.get_model(name="clf")
        self.assertIsInstance(result, RegisteredModel)
