from unittest import TestCase

import yaml

from michelangelo.lib.model_manager._private.schema.common import (
    dict_to_schema,
    schema_to_dict,
    schema_to_yaml,
)
from michelangelo.lib.model_manager.schema import (
    DataType,
    ModelSchema,
    ModelSchemaItem,
)


class SerdeTest(TestCase):
    """Tests serialization helpers for model schema objects."""

    def setUp(self):
        """Build shared schema fixtures."""
        self.schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(
                    name="input1", data_type=DataType.FLOAT, shape=[-1, -1]
                ),
                ModelSchemaItem(name="input2", data_type=DataType.INT, shape=[-1]),
            ],
            feature_store_features_schema=[
                ModelSchemaItem(
                    name="feature1", data_type=DataType.FLOAT, shape=[1, 2]
                ),
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
        """It converts schema objects to YAML."""
        schema_yaml = schema_to_yaml(self.schema)
        self.assertEqual(yaml.safe_load(schema_yaml), self.schema_dict)

    def test_schema_to_dict(self):
        """It converts schema objects to dictionaries."""
        schema_dict = schema_to_dict(self.schema)
        self.assertEqual(schema_dict, self.schema_dict)

    def test_dict_to_schema(self):
        """It materializes schema objects from dictionaries."""
        schema = dict_to_schema(self.schema_dict)
        self.assertEqual(schema, self.schema)
