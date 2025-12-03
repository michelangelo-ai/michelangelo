from unittest import TestCase

from michelangelo.lib.model_manager.schema.data_type import DataType


class DataTypeTest(TestCase):
    """Tests data type enumeration."""

    def test_data_type(self):
        """It iterates over DataType members without errors."""
        for data_type in DataType:
            self.assertIsInstance(data_type, DataType)
