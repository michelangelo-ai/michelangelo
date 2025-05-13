import yaml
from unittest import TestCase
from michelangelo.lib.model_manager.schema import (
    ModelSchema,
    ModelSchemaItem,
    DataType,
)
from michelangelo.lib.model_manager._private.schema.common import schema_to_yaml, schema_to_dict


class SerdeTest(TestCase):
    def setUp(self):
        self.schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="input1", data_type=DataType.FLOAT, shape=[-1, -1]),
                ModelSchemaItem(name="input2", data_type=DataType.INT, shape=[-1]),
            ],
            feature_store_features_schema=[
                ModelSchemaItem(name="feature1", data_type=DataType.FLOAT, shape=[1, 2]),
                ModelSchemaItem(name="feature2", data_type=DataType.INT),
            ],
            output_schema=[
                ModelSchemaItem(name="output1", data_type=DataType.STRING, shape=[6]),
                ModelSchemaItem(name="output2"),
            ],
        )
        self.schema_dict = {
            "input_schema": [
                {"name": "input1", "data_type": "float", "shape": [-1, -1]},
                {"name": "input2", "data_type": "int", "shape": [-1]},
            ],
            "feature_store_features_schema": [
                {"name": "feature1", "data_type": "float", "shape": [1, 2]},
                {"name": "feature2", "data_type": "int"},
            ],
            "output_schema": [
                {"name": "output1", "data_type": "string", "shape": [6]},
                {"name": "output2", "data_type": "unknown"},
            ],
        }

    def test_schema_to_yaml(self):
        schema_yaml = schema_to_yaml(self.schema)
        self.assertEqual(yaml.safe_load(schema_yaml), self.schema_dict)

    def test_schema_to_dict(self):
        schema_dict = schema_to_dict(self.schema)
        self.assertEqual(schema_dict, self.schema_dict)
