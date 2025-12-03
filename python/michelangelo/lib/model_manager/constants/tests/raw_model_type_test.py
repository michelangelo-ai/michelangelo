from unittest import TestCase

from michelangelo.lib.model_manager.constants import RawModelType


class RawModelTypeTest(TestCase):
    """Tests the raw model type enum values."""

    def test_raw_model_type(self):
        """It exposes user friendly string representations."""
        self.assertEqual(RawModelType.CUSTOM_PYTHON, "custom-python")
        self.assertEqual(RawModelType.HUGGINGFACE, "huggingface")
        self.assertEqual(RawModelType.TORCH, "torch")
