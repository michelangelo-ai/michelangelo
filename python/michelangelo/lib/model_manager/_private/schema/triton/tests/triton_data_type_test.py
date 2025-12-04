"""Tests for triton data type conversion."""

from unittest import TestCase

from michelangelo.lib.model_manager._private.schema.triton.data_type import (
    DATA_TYPE_MAPPING,
    convert_data_type,
)
from michelangelo.lib.model_manager.schema import DataType


class DataTypeTest(TestCase):
    """Tests conversion between internal and Triton data types."""

    def test_convert_data_type(self):
        """It maps known data types and returns None for unknown ones."""
        for data_type, triton_data_type in DATA_TYPE_MAPPING.items():
            self.assertEqual(convert_data_type(data_type), triton_data_type)

        for data_type in DataType:
            if data_type not in DATA_TYPE_MAPPING:
                self.assertIsNone(convert_data_type(data_type))
