from unittest import TestCase
from michelangelo.lib.model_manager.schema import DataType
from michelangelo.lib.model_manager._private.schema.triton import convert_data_type
from michelangelo.lib.model_manager._private.schema.triton import DATA_TYPE_MAPPING


class DataTypeTest(TestCase):
    def test_convert_data_type(self):
        for data_type, triton_data_type in DATA_TYPE_MAPPING.items():
            self.assertEqual(convert_data_type(data_type), triton_data_type)

        for data_type in DataType:
            if data_type not in DATA_TYPE_MAPPING:
                self.assertIsNone(convert_data_type(data_type))
