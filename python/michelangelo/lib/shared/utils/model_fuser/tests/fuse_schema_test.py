"""Unit tests for ``lib.shared.utils.model_fuser.fuse_schema``."""

from __future__ import annotations

import unittest

from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.lib.shared.utils.model_fuser.fuse_schema import (
    fuse_input_schema,
    fuse_model_schema,
)


class FuseInputSchemaTest(unittest.TestCase):
    """Tests for ``fuse_input_schema``."""

    def test_empty_schemas(self) -> None:
        """``None`` and empty schemas both fuse to an empty input list."""
        self.assertEqual(fuse_input_schema(None, None), [])
        self.assertEqual(fuse_input_schema(ModelSchema(), ModelSchema()), [])

    def test_union_preserves_order_tx_then_pred(self) -> None:
        """Fused inputs are ordered transform-first, then predictor."""
        tx = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="a", data_type=DataType.FLOAT, shape=[2, 3])
            ]
        )
        pred = ModelSchema(
            input_schema=[ModelSchemaItem(name="b", data_type=DataType.INT, shape=[1])]
        )
        out = fuse_input_schema(tx, pred)
        self.assertEqual([item.name for item in out], ["a", "b"])
        self.assertEqual(out[0].shape, [2, 3])
        self.assertEqual(out[0].data_type, DataType.FLOAT)
        self.assertEqual(out[1].shape, [1])
        self.assertEqual(out[1].data_type, DataType.INT)

    def test_tx_wins_on_duplicate_key(self) -> None:
        """When both stages define an input with the same name, tx wins."""
        tx = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1])
            ]
        )
        pred = ModelSchema(
            input_schema=[ModelSchemaItem(name="x", data_type=DataType.INT, shape=[2])]
        )
        out = fuse_input_schema(tx, pred)
        self.assertEqual(len(out), 1)
        self.assertEqual(out[0].data_type, DataType.FLOAT)
        self.assertEqual(out[0].shape, [1])

    def test_excludes_predictor_inputs_that_are_tx_outputs(self) -> None:
        """Predictor inputs produced by the transform are not user-facing."""
        tx = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="in", data_type=DataType.FLOAT, shape=[1])
            ],
            output_schema=[
                ModelSchemaItem(name="emb", data_type=DataType.FLOAT, shape=[8])
            ],
        )
        pred = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="emb", data_type=DataType.FLOAT, shape=[8])
            ],
            output_schema=[],
        )
        out = fuse_input_schema(tx, pred)
        self.assertEqual([item.name for item in out], ["in"])

    def test_item_missing_field_passed_through(self) -> None:
        """Items with a ``None`` shape or dtype are passed through as-is."""
        for field in ("shape", "data_type"):
            with self.subTest(field=field):
                kwargs = {"name": "a", "data_type": DataType.FLOAT, "shape": [2]}
                kwargs[field] = None
                schema = ModelSchema(input_schema=[ModelSchemaItem(**kwargs)])
                out = fuse_input_schema(schema, None)
                self.assertEqual(len(out), 1)
                self.assertIsNone(getattr(out[0], field))


class FuseModelSchemaTest(unittest.TestCase):
    """Tests for ``fuse_model_schema``."""

    def test_input_matches_fuse_input_schema_same_item_references(self) -> None:
        """The fused input schema reuses ``fuse_input_schema``'s items."""
        tx_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="u", data_type=DataType.FLOAT, shape=[1])
            ],
            output_schema=[
                ModelSchemaItem(name="v", data_type=DataType.FLOAT, shape=[2])
            ],
        )
        pred_schema = ModelSchema(
            input_schema=[ModelSchemaItem(name="w", data_type=DataType.INT, shape=[1])],
            output_schema=[
                ModelSchemaItem(name="o", data_type=DataType.FLOAT, shape=[1])
            ],
        )
        fused = fuse_model_schema(tx_schema, pred_schema)
        expected_items = fuse_input_schema(tx_schema, pred_schema)
        self.assertEqual(len(fused.input_schema), len(expected_items))
        for fused_item, expected in zip(fused.input_schema, expected_items):
            self.assertIs(fused_item, expected)

    def test_output_schema_from_predictor(self) -> None:
        """The fused output schema is the predictor's, unchanged."""
        tx_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="tx_in_a", data_type=DataType.FLOAT, shape=[1]),
                ModelSchemaItem(name="tx_in_b", data_type=DataType.INT, shape=[1]),
            ],
            output_schema=[
                ModelSchemaItem(name="tx_out", data_type=DataType.FLOAT, shape=[3])
            ],
        )
        predictor_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="pred_in", data_type=DataType.FLOAT, shape=[2]),
                ModelSchemaItem(name="tx_out", data_type=DataType.FLOAT, shape=[3]),
            ],
            output_schema=[
                ModelSchemaItem(name="output", data_type=DataType.FLOAT, shape=[1])
            ],
        )
        fused = fuse_model_schema(tx_schema, predictor_schema)
        input_names = [item.name for item in fused.input_schema]
        self.assertIn("tx_in_a", input_names)
        self.assertIn("tx_in_b", input_names)
        self.assertIn("pred_in", input_names)
        self.assertNotIn("tx_out", input_names)
        self.assertEqual([item.name for item in fused.output_schema], ["output"])

    def test_empty_predictor_returns_empty_outputs(self) -> None:
        """With no predictor, the fused schema has an empty output schema."""
        tx_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1])
            ],
            output_schema=[
                ModelSchemaItem(name="y", data_type=DataType.FLOAT, shape=[1])
            ],
        )
        fused = fuse_model_schema(tx_schema, None)
        self.assertEqual([item.name for item in fused.input_schema], ["x"])
        self.assertEqual(len(fused.output_schema), 0)

    def test_both_schemas_none(self) -> None:
        """With no schemas at all, the fused schema is empty on both sides."""
        fused = fuse_model_schema(None, None)
        self.assertEqual(fused.input_schema, [])
        self.assertEqual(fused.output_schema, [])

    def test_output_items_are_shallow_copy_of_predictor_list(self) -> None:
        """The output list is a new list, but items are shared references."""
        out_item = ModelSchemaItem(name="logit", data_type=DataType.FLOAT, shape=[32])
        predictor_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1])
            ],
            output_schema=[out_item],
        )
        fused = fuse_model_schema(None, predictor_schema)
        self.assertEqual(len(fused.output_schema), 1)
        self.assertIs(fused.output_schema[0], out_item)
        self.assertIsNot(fused.output_schema, predictor_schema.output_schema)
