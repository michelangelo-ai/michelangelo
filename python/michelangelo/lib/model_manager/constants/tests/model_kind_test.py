"""Tests for ModelKind constants."""

from unittest import TestCase

from michelangelo.lib.model_manager.constants import ModelKind


class ModelKindTest(TestCase):
    """Tests the model kind enum values."""

    def test_model_kind(self):
        """It exposes user friendly string representations."""
        self.assertEqual(ModelKind.CUSTOM, "custom")
        self.assertEqual(ModelKind.REGRESSION, "regression")
        self.assertEqual(ModelKind.BINARY_CLASSIFICATION, "binary-classification")
        self.assertEqual(
            ModelKind.MULTICLASS_CLASSIFICATION, "multiclass-classification"
        )
