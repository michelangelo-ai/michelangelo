from unittest import TestCase
from michelangelo.lib.model_manager.packager.custom_triton import CustomTritonPackager


class CustomTritonPackagerTest(TestCase):
    def test_custom_triton_packager(self):
        packager = CustomTritonPackager()
        self.assertIsNotNone(packager)