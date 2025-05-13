from unittest import TestCase
from michelangelo.lib.model_manager.schema import (
    ModelSchema,
    ModelSchemaItem,
    DataType,
)
from michelangelo.lib.model_manager._private.schema.triton import (
    validate_model_schema,
    validate_model_schema_item,
)


class ValidateSchemaTest(TestCase):
    def test_validate_model_schema_item_success(self):
        schema_item = ModelSchemaItem(
            name="ft1",
            data_type=DataType.INT,
            shape=[1],
        )

        is_valid, error = validate_model_schema_item(schema_item)
        self.assertTrue(is_valid)
        self.assertIsNone(error)

    def test_validate_model_schema_item_invalid_type(self):
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
                "Invalid data type: DataType.UNKNOWN. Supported data types for Triton models: "
                "['BOOLEAN', 'BYTE', 'CHAR', 'SHORT', 'INT', 'LONG', 'FLOAT', 'DOUBLE', 'STRING', 'NUMERIC']"
            ),
        )

    def test_validate_model_schema_item_invalid_shape(self):
        schema_item = ModelSchemaItem(
            name="ft1",
            data_type=DataType.INT,
            shape=[],
        )

        is_valid, error = validate_model_schema_item(schema_item)
        self.assertFalse(is_valid)
        self.assertIsInstance(error, ValueError)
        self.assertEqual(str(error), "Shape must be provided for item: ModelSchemaItem(name='ft1', data_type=<DataType.INT: 18>, shape=[])")

        schema_item = ModelSchemaItem(
            name="ft1",
            data_type=DataType.INT,
        )
        is_valid, error = validate_model_schema_item(schema_item)
        self.assertFalse(is_valid)
        self.assertIsInstance(error, ValueError)
        self.assertEqual(str(error), "Shape must be provided for item: ModelSchemaItem(name='ft1', data_type=<DataType.INT: 18>, shape=None)")

    def test_validate_model_schema_success(self):
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
