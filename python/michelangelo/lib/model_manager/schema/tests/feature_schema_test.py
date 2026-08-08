"""Tests for FeatureSchema."""

from unittest import TestCase

from michelangelo.lib.model_manager.schema import (
    DataType,
    FeatureSchema,
    FeatureSchemaItem,
)


class FeatureSchemaTest(TestCase):
    """Tests default behaviours of the feature schema dataclasses."""

    def test_feature_schema_defaults(self):
        """It populates schema collections with sensible defaults."""
        schema = FeatureSchema()
        self.assertEqual(schema.input_schema, [])
        self.assertEqual(schema.feature_store_features_schema, [])
        self.assertEqual(schema.derived_features_schema, [])

    def test_feature_schema_with_items(self):
        """It stores items across all three schema collections."""
        schema = FeatureSchema(
            input_schema=[
                FeatureSchemaItem(name="input1", data_type=DataType.FLOAT, shape=[1]),
            ],
            feature_store_features_schema=[
                FeatureSchemaItem(name="feature1", data_type=DataType.DOUBLE),
            ],
            derived_features_schema=[
                FeatureSchemaItem(name="derived1", data_type=DataType.INT),
            ],
        )

        self.assertEqual(len(schema.input_schema), 1)
        self.assertEqual(schema.input_schema[0].name, "input1")
        self.assertEqual(schema.input_schema[0].data_type, DataType.FLOAT)
        self.assertEqual(schema.input_schema[0].shape, [1])
        self.assertEqual(schema.feature_store_features_schema[0].name, "feature1")
        self.assertEqual(schema.derived_features_schema[0].name, "derived1")

    def test_feature_schema_item_defaults(self):
        """FeatureSchemaItem defaults to DataType.UNKNOWN and no shape."""
        item = FeatureSchemaItem(name="input1")
        self.assertEqual(item.data_type, DataType.UNKNOWN)
        self.assertIsNone(item.shape)
