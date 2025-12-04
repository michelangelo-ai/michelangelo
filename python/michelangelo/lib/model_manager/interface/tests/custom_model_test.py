"""Tests for CustomModel interface."""

import tempfile
from unittest import TestCase

import numpy as np

from michelangelo.lib.model_manager.interface.custom_model import Model
from michelangelo.lib.model_manager.interface.tests.fixtures.custom_model import (
    CustomModel,
)


class IncompleteModel(Model):
    """Minimal implementation used for interface contract tests."""

    def save(self, path: str):
        """Stub save implementation for testing."""
        pass

    def predict(
        self,
        inputs: dict[str, np.ndarray],
    ):
        """Stub predict implementation for testing."""
        pass


class CustomModelTest(TestCase):
    """Tests the base custom model interface."""

    def test_model(self):
        """It saves and loads custom models end to end."""
        model = CustomModel("content")

        with tempfile.NamedTemporaryFile() as f:
            model.save(f.name)
            loaded_model = CustomModel.load(f.name)
            result = loaded_model.predict(inputs={"feature": np.array([1, 2])})
            self.assertEqual(result["response"].tolist(), [1, 2])
            self.assertEqual(result["content"].tolist(), ["content"])

    def test_model_load_not_implemented(self):
        """It raises when abstract methods remain unimplemented."""
        with self.assertRaises(TypeError):
            IncompleteModel()

        with self.assertRaises(NotImplementedError):
            IncompleteModel.load("path")
