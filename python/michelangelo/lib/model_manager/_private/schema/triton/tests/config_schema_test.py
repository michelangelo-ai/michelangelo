from unittest import TestCase
from michelangelo.lib.model_manager.schema import (
    DataType,
    ModelSchema,
    ModelSchemaItem,
)
from michelangelo.lib.model_manager._private.schema.triton import convert_model_schema, convert_schema_to_dict


class ConfigSchemaTest(TestCase):
    def test_convert_model_schema(self):
        model_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="input1", data_type=DataType.FLOAT, shape=[1]),
                ModelSchemaItem(name="input2", data_type=DataType.INT, shape=[1, 2]),
            ],
            feature_store_features_schema=[
                ModelSchemaItem(name="@palette:feature1", data_type=DataType.STRING, shape=[1]),
            ],
            output_schema=[
                ModelSchemaItem(name="output1", data_type=DataType.INT, shape=[1, 2]),
                ModelSchemaItem(name="output2", data_type=DataType.STRING, shape=[1]),
            ],
        )

        input_schema, output_schema = convert_model_schema(model_schema)

        self.assertEqual(
            input_schema,
            {
                "input1": {"data_type": "FP32", "shape": "[ 1 ]", "optional": None},
                "input2": {"data_type": "INT32", "shape": "[ 1, 2 ]", "optional": None},
                "@palette:feature1": {"data_type": "STRING", "shape": "[ 1 ]", "optional": None},
            },
        )

        self.assertEqual(
            output_schema,
            {
                "output1": {"data_type": "INT32", "shape": "[ 1, 2 ]", "optional": None},
                "output2": {"data_type": "STRING", "shape": "[ 1 ]", "optional": None},
            },
        )

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
                "optional": None,
            },
            "ft2": {
                "data_type": "FP32",
                "shape": "[ 1, 2 ]",
                "optional": None,
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
                "optional": None,
            },
            "ft2": {
                "data_type": "FP32",
                "shape": "[ -1 ]",
                "optional": None,
            },
        }

        self.assertEqual(schema_dict, expected_schema_dict)