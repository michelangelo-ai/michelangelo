from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager.schema import (
    DataType,
    ModelSchema,
    ModelSchemaItem,
)
from uber.ai.michelangelo.sdk.model_manager._private.schema.triton import convert_model_schema


class ModelSchemaTest(TestCase):
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
                "input1": {"data_type": "FP32", "shape": "[ 1 ]"},
                "input2": {"data_type": "INT32", "shape": "[ 1, 2 ]"},
                "@palette:feature1": {"data_type": "STRING", "shape": "[ 1 ]"},
            },
        )

        self.assertEqual(
            output_schema,
            {
                "output1": {"data_type": "INT32", "shape": "[ 1, 2 ]"},
                "output2": {"data_type": "STRING", "shape": "[ 1 ]"},
            },
        )
