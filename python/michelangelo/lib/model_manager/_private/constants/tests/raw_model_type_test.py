from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager._private.constants import RawModelType


class RawModelTypeTest(TestCase):
    def test_raw_model_type(self):
        self.assertEqual(RawModelType.CUSTOM_PYTHON, "custom-python")
        self.assertEqual(RawModelType.HUGGINGFACE, "huggingface")
