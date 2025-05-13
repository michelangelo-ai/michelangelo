from unittest import TestCase
from michelangelo.lib.model_manager.schema import (
    DataType,
    ModelSchemaItem,
)
from michelangelo.lib.model_manager._private.schema.triton import convert_schema_to_dict


class SchemaToDictTest(TestCase):
    def test_convert_schema_to_dict(self):
        schema = [
            ModelSchemaItem(
                name="ft1",
                data_type=DataType.INT,
                shape=[1],
            ),
            ModelSchemaItem(
                name="ft2",
                data_type=DataType.FLOAT,
                shape=[1, 2],
            ),
        ]

        schema_dict = convert_schema_to_dict(schema)

        expected_schema_dict = {
            "ft1": {
                "data_type": "INT32",
                "shape": "[ 1 ]",
            },
            "ft2": {
                "data_type": "FP32",
                "shape": "[ 1, 2 ]",
            },
        }

        self.assertEqual(schema_dict, expected_schema_dict)

    def test_convert_schema_to_dict_invalid_schema(self):
        invalid_schema = [
            ModelSchemaItem(
                name="ft1",
                data_type=DataType.UNKNOWN,
                shape=[],
            ),
            ModelSchemaItem(
                name="ft2",
                data_type=DataType.FLOAT,
            ),
        ]

        schema_dict = convert_schema_to_dict(invalid_schema)

        expected_schema_dict = {
            "ft1": {
                "data_type": None,
                "shape": "[ -1 ]",
            },
            "ft2": {
                "data_type": "FP32",
                "shape": "[ -1 ]",
            },
        }

        self.assertEqual(schema_dict, expected_schema_dict)
