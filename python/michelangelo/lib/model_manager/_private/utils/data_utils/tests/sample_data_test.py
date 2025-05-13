import numpy as np
from unittest import TestCase
from typing import Optional
from uber.ai.michelangelo.sdk.model_manager.schema import ModelSchema, ModelSchemaItem, DataType
from uber.ai.michelangelo.sdk.model_manager._private.utils.data_utils import (
    validate_sample_data,
    validate_sample_data_with_model_schema,
)


class TestSampleData(TestCase):
    def setUp(self):
        self.schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="a", data_type=DataType.FLOAT, shape=[1]),
                ModelSchemaItem(name="b", data_type=DataType.STRING, shape=[-1, 1]),
            ],
            output_schema=[
                ModelSchemaItem(name="c", data_type=DataType.FLOAT, shape=[1]),
            ],
        )

    def assertValid(self, result: tuple[bool, Exception]):
        is_valid, err = result
        self.assertTrue(is_valid)
        self.assertIsNone(err)

    def assertInvalid(self, result: tuple[bool, Exception], ErrorType: type[Exception], message: Optional[str] = None):
        is_valid, err = result
        self.assertFalse(is_valid)
        self.assertIsInstance(err, ErrorType)
        if message:
            self.assertIn(message, str(err))

    def test_validate_sample_data_valid(self):
        data = [{"a": np.array([1]), "b": np.array([2])}]
        self.assertValid(validate_sample_data(data))

    def test_validate_sample_data_invalid(self):
        self.assertInvalid(validate_sample_data([]), ValueError, "Sample data is required")
        self.assertInvalid(validate_sample_data(None), ValueError, "Sample data is required")
        self.assertInvalid(validate_sample_data("a"), TypeError, "Error validating sample data, data must be a list of dictionaries of numpy arrays")
        self.assertInvalid(validate_sample_data(["a"]), TypeError)
        self.assertInvalid(validate_sample_data([{"a": "b"}]), TypeError)
        self.assertInvalid(validate_sample_data([{"a": np.array([1]), "b": "c"}]), TypeError)
        self.assertInvalid(validate_sample_data([{1: np.array([1])}]), TypeError)

    def test_validate_sample_data_with_model_schema(self):
        data = [{"a": np.array([1]), "b": np.array([["a"], ["b"]])}, {"a": np.array([1.0]), "b": np.array([["a"]])}]
        self.assertValid(validate_sample_data_with_model_schema(data, self.schema))
        self.assertInvalid(validate_sample_data_with_model_schema(data, self.schema, batch_inference=True), ValueError, "Error")

        data = [{"a": np.array([[1], [2]]), "b": np.array([[["a"], ["b"]]])}, {"a": np.array([[1.0]]), "b": np.array([[["a"]]])}]
        self.assertValid(validate_sample_data_with_model_schema(data, self.schema, batch_inference=True))

        data = [{"a": np.array([1]), "b": np.array([["a"], ["b"]])}, {"a": np.array([1.0])}]
        self.assertInvalid(
            validate_sample_data_with_model_schema(data, self.schema), ValueError, "Error validating sample data with model input schema. Data fields"
        )
