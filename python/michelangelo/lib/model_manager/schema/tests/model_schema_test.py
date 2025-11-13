from unittest import TestCase
from michelangelo.lib.model_manager.schema import (
    ModelSchema,
    ModelSchemaItem,
    DataType,
)


class ModelSchemaTest(TestCase):
    def test_model_schema(self):
        schema = ModelSchema()
        self.assertEqual(schema.input_schema, [])
        self.assertEqual(schema.feature_store_features_schema, [])

        schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="input1"),
                ModelSchemaItem(name="input2"),
            ],
            feature_store_features_schema=[
                ModelSchemaItem(name="feature1"),
                ModelSchemaItem(name="feature2"),
            ],
        )

        self.assertEqual(len(schema.input_schema), 2)
        self.assertEqual(len(schema.feature_store_features_schema), 2)
        self.assertEqual(schema.input_schema[0].name, "input1")
        self.assertEqual(schema.input_schema[0].data_type, DataType.UNKNOWN)
        self.assertEqual(schema.input_schema[1].name, "input2")
        self.assertEqual(schema.input_schema[1].data_type, DataType.UNKNOWN)
        self.assertEqual(schema.feature_store_features_schema[0].name, "feature1")
        self.assertEqual(schema.feature_store_features_schema[0].data_type, DataType.UNKNOWN)
        self.assertEqual(schema.feature_store_features_schema[1].name, "feature2")
        self.assertEqual(schema.feature_store_features_schema[1].data_type, DataType.UNKNOWN)

        schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="input1", data_type=DataType.FLOAT),
                ModelSchemaItem(name="input2", data_type=DataType.STRING),
            ],
            feature_store_features_schema=[
                ModelSchemaItem(name="feature1", data_type=DataType.DOUBLE),
                ModelSchemaItem(name="feature2", data_type=DataType.INT),
            ],
        )

        self.assertEqual(len(schema.input_schema), 2)
        self.assertEqual(len(schema.feature_store_features_schema), 2)
        self.assertEqual(schema.input_schema[0].name, "input1")
        self.assertEqual(schema.input_schema[0].data_type, DataType.FLOAT)
        self.assertEqual(schema.input_schema[1].name, "input2")
        self.assertEqual(schema.input_schema[1].data_type, DataType.STRING)
        self.assertEqual(schema.feature_store_features_schema[0].name, "feature1")
        self.assertEqual(schema.feature_store_features_schema[0].data_type, DataType.DOUBLE)
        self.assertEqual(schema.feature_store_features_schema[1].name, "feature2")
        self.assertEqual(schema.feature_store_features_schema[1].data_type, DataType.INT)

        schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="input1", data_type=DataType.FLOAT, shape=[10, 5]),
                ModelSchemaItem(name="input2", data_type=DataType.STRING),
            ],
            feature_store_features_schema=[
                ModelSchemaItem(name="feature1", data_type=DataType.LONG, shape=[-1, 1]),
                ModelSchemaItem(name="feature2", data_type=DataType.DOUBLE),
            ],
        )

        self.assertEqual(len(schema.input_schema), 2)
        self.assertEqual(len(schema.feature_store_features_schema), 2)
        self.assertEqual(schema.input_schema[0].name, "input1")
        self.assertEqual(schema.input_schema[0].data_type, DataType.FLOAT)
        self.assertEqual(schema.input_schema[0].shape, [10, 5])
        self.assertEqual(schema.input_schema[1].name, "input2")
        self.assertEqual(schema.input_schema[1].data_type, DataType.STRING)
        self.assertIsNone(schema.input_schema[1].shape)
        self.assertEqual(schema.feature_store_features_schema[0].name, "feature1")
        self.assertEqual(schema.feature_store_features_schema[0].data_type, DataType.LONG)
        self.assertEqual(schema.feature_store_features_schema[0].shape, [-1, 1])
        self.assertEqual(schema.feature_store_features_schema[1].name, "feature2")
        self.assertEqual(schema.feature_store_features_schema[1].data_type, DataType.DOUBLE)
        self.assertIsNone(schema.feature_store_features_schema[1].shape)
