"""Tests for the pusher types module."""

from __future__ import annotations

from unittest import TestCase

from michelangelo.workflow.variables.types import (
    AssembledModel,
    ModelArtifact,
    PusherResult,
)


class TestModelArtifact(TestCase):
    """Tests for ModelArtifact."""

    def test_stores_path(self):
        """It stores the provided path."""
        artifact = ModelArtifact(path="/tmp/model")
        self.assertEqual(artifact.path, "/tmp/model")

    def test_metadata_defaults_to_empty_dict(self):
        """It defaults metadata to an empty dict when not provided."""
        artifact = ModelArtifact(path="/tmp/model")
        self.assertEqual(artifact.metadata, {})

    def test_metadata_instances_are_independent(self):
        """It creates a separate metadata dict for each instance."""
        a = ModelArtifact(path="/tmp/a")
        b = ModelArtifact(path="/tmp/b")
        a.metadata["key"] = "value"
        self.assertEqual(b.metadata, {})

    def test_metadata_can_be_provided(self):
        """It stores explicitly provided metadata."""
        artifact = ModelArtifact(path="/tmp/m", metadata={"framework": "xgboost"})
        self.assertEqual(artifact.metadata["framework"], "xgboost")


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
