from unittest import TestCase

from michelangelo.lib.model_manager.packager.custom_triton import CustomTritonPackager


class CustomTritonPackagerTest(TestCase):
    """Tests instantiation of the custom Triton packager."""

    def test_custom_triton_packager(self):
        """It creates a packager instance with default settings."""
        packager = CustomTritonPackager()
        self.assertIsNotNone(packager)
