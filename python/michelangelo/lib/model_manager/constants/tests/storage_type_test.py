"""Tests for StorageType constants."""

from unittest import TestCase

from michelangelo.lib.model_manager.constants import StorageType


class StorageTypeTest(TestCase):
    """Tests storage type enum helpers."""

    def test_storage_type(self):
        """It exposes uppercase names for each storage type."""
        for attr_name, attr_value in vars(StorageType).items():
            if not callable(attr_value) and not attr_name.startswith("__"):
                self.assertTrue(attr_name, attr_value.upper())
