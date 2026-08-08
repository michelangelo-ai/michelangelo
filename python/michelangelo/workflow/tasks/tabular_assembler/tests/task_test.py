"""Unit tests for ``michelangelo.workflow.tasks.tabular_assembler.task``."""

from __future__ import annotations

import tempfile
import unittest
from typing import TYPE_CHECKING
from unittest.mock import patch

from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend
from michelangelo.lib.model_manager.interface.custom_model import Model
from michelangelo.workflow.schema.assembler import TabularAssemblerConfig
from michelangelo.workflow.tasks.tabular_assembler.task import tabular_assembler
from michelangelo.workflow.variables.metadata import (
    TRAINING_FRAMEWORK_CUSTOM,
    TRAINING_FRAMEWORK_LIGHTNING,
    TRAINING_FRAMEWORK_PYTORCH,
    ModelMetadata,
)
from michelangelo.workflow.variables.types import (
    AssembledModel,
    FeaturePackageArtifact,
    ModelArtifact,
)

if TYPE_CHECKING:
    from numpy import ndarray

_TASK_MODULE = "michelangelo.workflow.tasks.tabular_assembler.task"


class _CustomModelFixture(Model):
    """Minimal concrete ``Model`` used only as an importable dotted path."""

    def save(self, path: str) -> None:
        pass

    @classmethod
    def load(cls, path: str) -> _CustomModelFixture:
        return cls()

    def predict(self, inputs: dict[str, ndarray]) -> dict[str, ndarray]:
        return inputs


CUSTOM_MODEL_CLASS_PATH = (
    "michelangelo.workflow.tasks.tabular_assembler.tests.task_test._CustomModelFixture"
)


class TabularAssemblerDispatchTest(unittest.TestCase):
    """Tests for ``tabular_assembler``'s framework dispatch."""

    def setUp(self) -> None:
        """Create a fresh ``LocalStorageBackend`` rooted at a temp dir per test."""
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.storage_backend = LocalStorageBackend(self._tmp.name)

    def _sentinel_result(self) -> AssembledModel:
        """Return a placeholder ``AssembledModel`` for mocking downstream assemblers."""
        return AssembledModel(
            raw_model=ModelArtifact(path="raw"),
            deployable_model=ModelArtifact(path="deployable"),
        )

    @patch(f"{_TASK_MODULE}.custom_assembler")
    def test_dispatches_to_custom_when_metadata_framework_custom(self, mock_custom):
        """``training_framework == custom`` routes straight to ``custom_assembler``."""
        mock_custom.return_value = self._sentinel_result()
        config = TabularAssemblerConfig()
        raw_model = ModelArtifact(
            path="p",
            metadata=ModelMetadata(training_framework=TRAINING_FRAMEWORK_CUSTOM),
        )

        result = tabular_assembler(
            config, raw_model, storage_backend=self.storage_backend
        )

        self.assertIs(result, mock_custom.return_value)
        mock_custom.assert_called_once_with(
            config, raw_model, None, None, storage_backend=self.storage_backend
        )

    @patch(f"{_TASK_MODULE}.custom_assembler")
    def test_dispatches_to_custom_with_feature_package(self, mock_custom):
        """A supplied ``feature_package`` is forwarded to ``custom_assembler``."""
        mock_custom.return_value = self._sentinel_result()
        config = TabularAssemblerConfig()
        raw_model = ModelArtifact(
            path="p",
            metadata=ModelMetadata(training_framework=TRAINING_FRAMEWORK_CUSTOM),
        )
        feature_package = FeaturePackageArtifact(path="features")

        tabular_assembler(
            config,
            raw_model,
            feature_package=feature_package,
            storage_backend=self.storage_backend,
        )

        mock_custom.assert_called_once_with(
            config,
            raw_model,
            None,
            feature_package,
            storage_backend=self.storage_backend,
        )

    @patch(f"{_TASK_MODULE}.custom_assembler")
    def test_dispatches_to_custom_when_config_model_class_is_custom_model(
        self, mock_custom
    ):
        """A config-supplied custom ``Model`` class forces the custom path.

        This holds even when the recorded training framework is ``lightning``.
        """
        mock_custom.return_value = self._sentinel_result()
        config = TabularAssemblerConfig(model_class=CUSTOM_MODEL_CLASS_PATH)
        raw_model = ModelArtifact(
            path="p",
            metadata=ModelMetadata(training_framework=TRAINING_FRAMEWORK_LIGHTNING),
        )
        native_tx = ModelArtifact(path="tx")

        tabular_assembler(
            config, raw_model, native_tx, storage_backend=self.storage_backend
        )

        mock_custom.assert_called_once_with(
            config, raw_model, native_tx, None, storage_backend=self.storage_backend
        )

    def test_torch_dispatch_resolves_now_that_torch_assembler_exists(self):
        """``torch.assembler`` now exists and is imported directly by ``task``."""
        from michelangelo.workflow.tasks.tabular_assembler.task import (
            torch_assembler,
        )

        self.assertTrue(callable(torch_assembler))

    @patch("michelangelo.workflow.tasks.tabular_assembler.task.torch_assembler")
    def test_dispatches_to_torch_assembler_for_pytorch_and_lightning(self, mock_torch):
        """``pytorch``/``lightning`` frameworks route to the real torch assembler.

        The ``pytorch`` framework does not forward ``native_transform_model``
        to ``torch_assembler`` (pre-existing dispatch behavior); ``lightning``
        does.
        """
        mock_torch.reset_mock()
        mock_torch.return_value = self._sentinel_result()
        config = TabularAssemblerConfig()
        raw_model = ModelArtifact(
            path="p",
            metadata=ModelMetadata(training_framework=TRAINING_FRAMEWORK_PYTORCH),
        )

        result = tabular_assembler(
            config, raw_model, storage_backend=self.storage_backend
        )

        self.assertIs(result, mock_torch.return_value)
        mock_torch.assert_called_once_with(
            config,
            raw_model,
            feature_package=None,
            storage_backend=self.storage_backend,
        )

        mock_torch.reset_mock()
        mock_torch.return_value = self._sentinel_result()
        native_tx = ModelArtifact(path="tx")
        raw_model = ModelArtifact(
            path="p",
            metadata=ModelMetadata(training_framework=TRAINING_FRAMEWORK_LIGHTNING),
        )

        result = tabular_assembler(
            config, raw_model, native_tx, storage_backend=self.storage_backend
        )

        self.assertIs(result, mock_torch.return_value)
        mock_torch.assert_called_once_with(
            config, raw_model, native_tx, None, storage_backend=self.storage_backend
        )

    @patch("michelangelo.workflow.tasks.tabular_assembler.task.torch_assembler")
    def test_dispatches_to_torch_assembler_with_feature_package(self, mock_torch):
        """A supplied ``feature_package`` is forwarded to ``torch_assembler``.

        Covers both the pytorch and lightning frameworks.
        """
        mock_torch.return_value = self._sentinel_result()
        config = TabularAssemblerConfig()
        feature_package = FeaturePackageArtifact(path="features")
        raw_model = ModelArtifact(
            path="p",
            metadata=ModelMetadata(training_framework=TRAINING_FRAMEWORK_PYTORCH),
        )

        tabular_assembler(
            config,
            raw_model,
            feature_package=feature_package,
            storage_backend=self.storage_backend,
        )

        mock_torch.assert_called_once_with(
            config,
            raw_model,
            feature_package=feature_package,
            storage_backend=self.storage_backend,
        )

        mock_torch.reset_mock()
        mock_torch.return_value = self._sentinel_result()
        native_tx = ModelArtifact(path="tx")
        raw_model = ModelArtifact(
            path="p",
            metadata=ModelMetadata(training_framework=TRAINING_FRAMEWORK_LIGHTNING),
        )

        tabular_assembler(
            config,
            raw_model,
            native_tx,
            feature_package,
            storage_backend=self.storage_backend,
        )

        mock_torch.assert_called_once_with(
            config,
            raw_model,
            native_tx,
            feature_package,
            storage_backend=self.storage_backend,
        )

    def test_unsupported_framework_returns_empty_pair(self):
        """An unrecognized, non-empty framework yields the empty pair."""
        config = TabularAssemblerConfig()
        raw_model = ModelArtifact(
            path="p", metadata=ModelMetadata(training_framework="unsupported_framework")
        )

        result = tabular_assembler(
            config, raw_model, storage_backend=self.storage_backend
        )

        self.assertEqual(result.raw_model.path, "")
        self.assertEqual(result.deployable_model.path, "")

    def test_no_framework_recorded_and_no_config_model_class_returns_empty_pair(self):
        """No recorded framework and no config model class yields the empty pair."""
        config = TabularAssemblerConfig()
        raw_model = ModelArtifact(path="p", metadata=ModelMetadata())

        result = tabular_assembler(
            config, raw_model, storage_backend=self.storage_backend
        )

        self.assertEqual(result.raw_model.path, "")
        self.assertEqual(result.deployable_model.path, "")


if __name__ == "__main__":
    unittest.main()
