"""Tests for DataType enum."""

from unittest import TestCase

from michelangelo.lib.model_manager.schema.data_type import DataType


class DataTypeTest(TestCase):
    """Tests data type enumeration."""

    def test_data_type(self):
        """It iterates over DataType members without errors."""
        for data_type in DataType:
            self.assertIsInstance(data_type, DataType)

    def test_data_type_values_match_schema_proto(self):
        """It assigns enum values matching the schema protobuf definition.

        Only the Triton-relevant/OSS-public subset is defined here (see
        ``proto/api/v2/schema.proto``); the numeric values below intentionally
        skip the gaps reserved by that proto's own DataType enum.
        """
        expected_values = {
            "INVALID": 0,
            "UNKNOWN": 1,
            "BOOLEAN": 4,
            "STRING": 7,
            "BYTE": 15,
            "CHAR": 16,
            "SHORT": 17,
            "INT": 18,
            "LONG": 19,
            "FLOAT": 20,
            "DOUBLE": 21,
        }
        for name, value in expected_values.items():
            with self.subTest(name=name):
                self.assertEqual(DataType[name].value, value)
        self.assertEqual({dt.name for dt in DataType}, set(expected_values))
