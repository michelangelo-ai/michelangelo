"""Tests for michelangelo.lib.shared.utils.model_metadata.e2e_schema."""

import unittest

from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.lib.model_manager.schema.feature_schema import FeatureSchema
from michelangelo.lib.model_manager.schema.feature_schema_item import FeatureSchemaItem
from michelangelo.lib.shared.utils.model_metadata.e2e_schema import (
    feature_schema_item_to_model_schema_item,
    fuse_e2e_schema,
)


class FeatureSchemaItemToModelSchemaItemTest(unittest.TestCase):
    def test_converts_name_shape_and_data_type(self) -> None:
        item = FeatureSchemaItem(name="f", data_type=DataType.DOUBLE, shape=[4])
        out = feature_schema_item_to_model_schema_item(item)
        self.assertEqual(out.name, "f")
        self.assertEqual(out.shape, [4])
        self.assertEqual(out.data_type, DataType.DOUBLE)


class FuseE2eSchemaTest(unittest.TestCase):
    def test_feature_schema_none_returns_model_schema_unchanged(self) -> None:
        model_schema = ModelSchema(input_schema=[ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1])])
        out = fuse_e2e_schema(None, model_schema)
        self.assertIs(out, model_schema)

    def test_input_is_union_minus_derived(self) -> None:
        feature_schema = FeatureSchema(
            input_schema=[FeatureSchemaItem(name="raw_a", data_type=DataType.FLOAT, shape=[1])],
            derived_features_schema=[FeatureSchemaItem(name="derived_a", data_type=DataType.FLOAT, shape=[1])],
        )
        model_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="derived_a", data_type=DataType.FLOAT, shape=[1]),
                ModelSchemaItem(name="model_only", data_type=DataType.INT, shape=[1]),
            ],
            output_schema=[ModelSchemaItem(name="out", data_type=DataType.FLOAT, shape=[1])],
        )
        out = fuse_e2e_schema(feature_schema, model_schema)
        self.assertEqual([item.name for item in out.input_schema], ["raw_a", "model_only"])
        self.assertEqual([item.name for item in out.output_schema], ["out"])

    def test_feature_side_wins_on_duplicate_input_name(self) -> None:
        feature_schema = FeatureSchema(
            input_schema=[FeatureSchemaItem(name="x", data_type=DataType.DOUBLE, shape=[9])],
        )
        model_schema = ModelSchema(
            input_schema=[ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1])],
        )
        out = fuse_e2e_schema(feature_schema, model_schema)
        self.assertEqual(len(out.input_schema), 1)
        self.assertEqual(out.input_schema[0].data_type, DataType.DOUBLE)
        self.assertEqual(out.input_schema[0].shape, [9])

    def test_feature_store_features_schema_is_unioned(self) -> None:
        feature_schema = FeatureSchema(
            feature_store_features_schema=[FeatureSchemaItem(name="palette_a", data_type=DataType.FLOAT, shape=[1])],
        )
        model_schema = ModelSchema(
            feature_store_features_schema=[ModelSchemaItem(name="palette_b", data_type=DataType.FLOAT, shape=[1])],
        )
        out = fuse_e2e_schema(feature_schema, model_schema)
        self.assertEqual(
            sorted(item.name for item in out.feature_store_features_schema),
            ["palette_a", "palette_b"],
        )

    def test_empty_feature_schema_yields_model_schema_input(self) -> None:
        feature_schema = FeatureSchema()
        model_schema = ModelSchema(
            input_schema=[ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1])],
            output_schema=[ModelSchemaItem(name="y", data_type=DataType.FLOAT, shape=[1])],
        )
        out = fuse_e2e_schema(feature_schema, model_schema)
        self.assertEqual([item.name for item in out.input_schema], ["x"])
        self.assertEqual([item.name for item in out.output_schema], ["y"])

    def test_feature_schema_input_item_matching_derived_name_is_excluded(self) -> None:
        feature_schema = FeatureSchema(
            input_schema=[
                FeatureSchemaItem(name="derived_a", data_type=DataType.FLOAT, shape=[1]),
                FeatureSchemaItem(name="raw_b", data_type=DataType.FLOAT, shape=[1]),
            ],
            derived_features_schema=[FeatureSchemaItem(name="derived_a", data_type=DataType.FLOAT, shape=[1])],
        )
        model_schema = ModelSchema(output_schema=[ModelSchemaItem(name="out", data_type=DataType.FLOAT, shape=[1])])
        out = fuse_e2e_schema(feature_schema, model_schema)
        self.assertEqual([item.name for item in out.input_schema], ["raw_b"])

    def test_duplicate_names_within_feature_input_schema_are_deduped(self) -> None:
        feature_schema = FeatureSchema(
            input_schema=[
                FeatureSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1]),
                FeatureSchemaItem(name="x", data_type=DataType.DOUBLE, shape=[2]),
            ],
        )
        model_schema = ModelSchema()
        out = fuse_e2e_schema(feature_schema, model_schema)
        self.assertEqual(len(out.input_schema), 1)
        self.assertEqual(out.input_schema[0].data_type, DataType.FLOAT)
