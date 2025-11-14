from unittest import TestCase
from michelangelo.lib.model_manager.schema.data_type import DataType


class DataTypeTest(TestCase):
    def test_data_type(self):
        for data_type in DataType:
            self.assertIsInstance(data_type, DataType)
