"""Unit tests for ``...tabular_assembler._private.schema.output_schema``."""

from __future__ import annotations

import unittest

from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.workflow.tasks.tabular_assembler._private.schema.output_schema import (  # noqa: E501
    reorder_output_schema,
)


def _make_schema(
    output_names: list[str], input_names: list[str] | None = None
) -> ModelSchema:
    input_schema = [
        ModelSchemaItem(name=n, data_type=DataType.FLOAT, shape=[1])
        for n in (input_names or ["x"])
    ]
    return ModelSchema(
        input_schema=input_schema,
        output_schema=[
            ModelSchemaItem(name=n, data_type=DataType.FLOAT, shape=[1])
            for n in output_names
        ],
    )


class ReorderOutputSchemaTest(unittest.TestCase):
    """Tests for ``reorder_output_schema``."""

    def test_none_field_order_returns_schema_unchanged(self):
        """``field_order=None`` returns the exact same schema instance."""
        schema = _make_schema(["a", "b", "c"])
        result = reorder_output_schema(schema, None)
        self.assertIs(result, schema)

    def test_reorders_output_to_match_field_order(self):
        """Output fields are reordered to match ``field_order``."""
        schema = _make_schema(["a", "b", "c"])
        result = reorder_output_schema(schema, ["c", "a", "b"])
        self.assertEqual([item.name for item in result.output_schema], ["c", "a", "b"])

    def test_field_order_missing_from_schema_is_skipped(self):
        """Names in ``field_order`` that aren't in the schema are silently skipped."""
        schema = _make_schema(["a", "b"])
        result = reorder_output_schema(schema, ["b", "ghost", "a"])
        self.assertEqual([item.name for item in result.output_schema], ["b", "a"])

    def test_uncovered_schema_fields_appended_at_end(self):
        """Output fields absent from ``field_order`` are appended at the end."""
        schema = _make_schema(["a", "b", "c"])
        result = reorder_output_schema(schema, ["b"])
        self.assertEqual([item.name for item in result.output_schema], ["b", "a", "c"])

    def test_input_schema_preserved_unchanged(self):
        """``input_schema`` is untouched by output reordering."""
        schema = _make_schema(["a", "b"], input_names=["x", "y"])
        result = reorder_output_schema(schema, ["b", "a"])
        self.assertEqual([item.name for item in result.input_schema], ["x", "y"])

    def test_empty_field_order_appends_all_fields(self):
        """An empty (but non-``None``) ``field_order`` keeps the original order."""
        schema = _make_schema(["a", "b"])
        result = reorder_output_schema(schema, [])
        self.assertEqual([item.name for item in result.output_schema], ["a", "b"])

    def test_empty_output_schema_with_field_order(self):
        """A ``field_order`` naming fields absent from an empty schema is a no-op."""
        schema = _make_schema([])
        result = reorder_output_schema(schema, ["a"])
        self.assertEqual(result.output_schema, [])

    def test_empty_output_schema_with_empty_field_order(self):
        """An empty schema with an empty ``field_order`` stays empty."""
        schema = _make_schema([])
        result = reorder_output_schema(schema, [])
        self.assertEqual(result.output_schema, [])

    def test_returns_new_model_schema_object(self):
        """A reordered schema is a new object, not the input mutated in place."""
        schema = _make_schema(["a", "b"])
        result = reorder_output_schema(schema, ["b", "a"])
        self.assertIsNot(result, schema)

    def test_input_schema_is_shallow_copy_not_same_reference(self):
        """``result.input_schema`` is a new list, even though items are shared."""
        schema = _make_schema(["a"])
        result = reorder_output_schema(schema, ["a"])
        self.assertIsNot(result.input_schema, schema.input_schema)

    def test_output_schema_is_new_list_not_same_reference(self):
        """``result.output_schema`` is a new list, even though items are shared."""
        schema = _make_schema(["a"])
        result = reorder_output_schema(schema, ["a"])
        self.assertIsNot(result.output_schema, schema.output_schema)

    def test_all_field_order_items_in_schema_covers_everything(self):
        """A ``field_order`` covering every field fully determines the result order."""
        schema = _make_schema(["a", "b", "c"])
        result = reorder_output_schema(schema, ["c", "b", "a"])
        self.assertEqual([item.name for item in result.output_schema], ["c", "b", "a"])

    def test_duplicate_field_order_preserves_duplicates(self):
        """Duplicates in ``field_order`` produce duplicates in output (no dedup)."""
        schema = _make_schema(["a", "b"])
        result = reorder_output_schema(schema, ["b", "b", "a"])
        self.assertEqual([item.name for item in result.output_schema], ["b", "b", "a"])

    def test_output_items_are_same_object_references(self):
        """Reordering moves the existing item objects; it doesn't copy them."""
        schema = _make_schema(["a", "b"])
        result = reorder_output_schema(schema, ["b", "a"])
        self.assertIs(result.output_schema[0], schema.output_schema[1])
        self.assertIs(result.output_schema[1], schema.output_schema[0])

    def test_preserves_output_item_attributes(self):
        """Reordered items keep their original data_type/shape."""
        schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1])
            ],
            output_schema=[
                ModelSchemaItem(name="out", data_type=DataType.INT, shape=[32])
            ],
        )
        result = reorder_output_schema(schema, ["out"])
        self.assertEqual(result.output_schema[0].data_type, DataType.INT)
        self.assertEqual(result.output_schema[0].shape, [32])


if __name__ == "__main__":
    unittest.main()
