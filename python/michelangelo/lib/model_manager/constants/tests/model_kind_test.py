from unittest import TestCase
from michelangelo.lib.model_manager.constants import ModelKind


class ModelKindTest(TestCase):
    def test_model_kind(self):
        for attr_name, attr_value in vars(ModelKind).items():
            if not callable(attr_value) and not attr_name.startswith("__"):
                self.assertTrue(attr_name, attr_value.upper())
