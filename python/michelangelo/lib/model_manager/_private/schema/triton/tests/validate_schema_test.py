"""Tests for schema validation."""

from unittest import TestCase

from michelangelo.lib.model_manager._private.schema.triton import (
    validate_model_schema,
    validate_model_schema_item,
)
from michelangelo.lib.model_manager.schema import (
    DataType,
    ModelSchema,
    ModelSchemaItem,
)


class ValidateSchemaTest(TestCase):
    """Tests Triton schema validation helpers."""

    def test_validate_model_schema_item_success(self):
        """It returns success for a valid schema item."""
        schema_item = ModelSchemaItem(
            name="ft1",
            data_type=DataType.INT,
            shape=[1],
        )

        is_valid, error = validate_model_schema_item(schema_item)
        self.assertTrue(is_valid)
        self.assertIsNone(error)

    def test_validate_model_schema_item_invalid_type(self):
        """It rejects schema items with unsupported data types."""
        schema_item = ModelSchemaItem(
            name="ft1",
            data_type=DataType.UNKNOWN,
            shape=[],
        )

        is_valid, error = validate_model_schema_item(schema_item)
        self.assertFalse(is_valid)
        self.assertIsInstance(error, ValueError)
        self.assertEqual(
            str(error),
            (
                "Invalid data type: DataType.UNKNOWN. Supported data types for "
                "Triton models: "
                "['BOOLEAN', 'BYTE', 'CHAR', 'SHORT', 'INT', 'LONG', 'FLOAT', "
                "'DOUBLE', 'STRING']"
            ),
        )

    def test_validate_model_schema_item_invalid_shape(self):
        """It rejects schema items without shapes."""
        schema_item = ModelSchemaItem(
            name="ft1",
            data_type=DataType.INT,
            shape=[],
        )

        is_valid, error = validate_model_schema_item(schema_item)
        self.assertFalse(is_valid)
        self.assertIsInstance(error, ValueError)
        self.assertEqual(
            str(error),
            "Shape must be provided for item: ModelSchemaItem(name='ft1', "
            "data_type=<DataType.INT: 18>, shape=[], optional=None)",
        )

        schema_item = ModelSchemaItem(
            name="ft1",
            data_type=DataType.INT,
        )
        is_valid, error = validate_model_schema_item(schema_item)
        self.assertFalse(is_valid)
        self.assertIsInstance(error, ValueError)
        self.assertEqual(
            str(error),
            "Shape must be provided for item: ModelSchemaItem(name='ft1', "
            "data_type=<DataType.INT: 18>, shape=None, optional=None)",
        )

    def test_validate_model_schema_success(self):
        """It accepts schemas with valid items."""
        model_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(
                    name="ft1",
                    data_type=DataType.INT,
                    shape=[1],
                ),
            ],
            feature_store_features_schema=[
                ModelSchemaItem(
                    name="ft2",
                    data_type=DataType.FLOAT,
                    shape=[1, 2],
                ),
            ],
            output_schema=[
                ModelSchemaItem(
                    name="ft3",
                    data_type=DataType.STRING,
                    shape=[1],
                ),
            ],
        )

        is_valid, error = validate_model_schema(model_schema)
        self.assertTrue(is_valid)
        self.assertIsNone(error)

        model_schema = ModelSchema(
            input_schema=None,
            output_schema=[
                ModelSchemaItem(
                    name="ft3",
                    data_type=DataType.STRING,
                    shape=[1],
                ),
            ],
        )
        is_valid, error = validate_model_schema(model_schema)
        self.assertTrue(is_valid)
        self.assertIsNone(error)

    def test_validate_model_schema_error(self):
        """It surfaces the first invalid input schema item error."""
        model_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(
                    name="ft1",
                    data_type=DataType.UNKNOWN,
                    shape=[],
                ),
            ],
        )

        is_valid, error = validate_model_schema(model_schema)
        self.assertFalse(is_valid)
        self.assertIsInstance(error, ValueError)

    def test_validate_model_schema_output_schema_error(self):
        """It rejects output schema items missing a shape."""
        model_schema = ModelSchema(
            output_schema=[
                ModelSchemaItem(
                    name="ft1",
                    data_type=DataType.INT,
                    shape=[],
                ),
            ],
        )
        is_valid, error = validate_model_schema(model_schema)
        self.assertFalse(is_valid)
        self.assertIsInstance(error, ValueError)

    def test_validate_model_schema_output_schema_with_optional(self):
        """It rejects optional output schema items."""
        model_schema = ModelSchema(
            output_schema=[
                ModelSchemaItem(
                    name="ft1",
                    data_type=DataType.INT,
                    shape=[1],
                    optional=True,
                ),
            ],
        )
        is_valid, error = validate_model_schema(model_schema)
        self.assertFalse(is_valid)
        self.assertIsInstance(error, ValueError)
        self.assertEqual(
            str(error),
            (
                "Optional is not allowed for output schema. "
                "Please remove the optional flag from the schema item: "
                "ModelSchemaItem(name='ft1', data_type=<DataType.INT: 18>, shape=[1], "
                "optional=True)"
            ),
        )
