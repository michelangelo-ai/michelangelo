from unittest import TestCase
from michelangelo.lib.model_manager._private.constants import Placeholder


class PlaceholderTest(TestCase):
    def test_placeholder(self):
        self.assertEqual(Placeholder.MODEL_NAME, "$MODEL_NAME")
