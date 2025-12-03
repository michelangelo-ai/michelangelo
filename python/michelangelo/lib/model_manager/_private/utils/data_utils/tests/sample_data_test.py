from typing import Optional
from unittest import TestCase

import numpy as np

from michelangelo.lib.model_manager._private.utils.data_utils import (
    validate_sample_data,
    validate_sample_data_with_model_schema,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem


class TestSampleData(TestCase):
    """Tests sample data validation helpers."""

    def setUp(self):
        """Create schema fixtures for sample data tests."""
        self.schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="a", data_type=DataType.FLOAT, shape=[1]),
                ModelSchemaItem(name="b", data_type=DataType.STRING, shape=[-1, 1]),
            ],
            output_schema=[
                ModelSchemaItem(name="c", data_type=DataType.FLOAT, shape=[1]),
            ],
        )

    def assert_valid(self, result: tuple[bool, Exception]):
        """Assert that validation result is successful."""
        is_valid, err = result
        self.assertTrue(is_valid)
        self.assertIsNone(err)

    def assert_invalid(
        self,
        result: tuple[bool, Exception],
        error_type: type[Exception],
        message: Optional[str] = None,
    ):
        """Assert that validation result is a failure with expected error type."""
        is_valid, err = result
        self.assertFalse(is_valid)
        self.assertIsInstance(err, error_type)
        if message:
            self.assertIn(message, str(err))

    def test_validate_sample_data_valid(self):
        """It validates correct sample data structures."""
        data = [{"a": np.array([1]), "b": np.array([2])}]
        self.assert_valid(validate_sample_data(data))

    def test_validate_sample_data_invalid(self):
        """It invalidates incorrect sample data structures."""
        self.assert_invalid(
            validate_sample_data([]), ValueError, "Sample data is required"
        )
        self.assert_invalid(
            validate_sample_data(None), ValueError, "Sample data is required"
        )
        self.assert_invalid(
            validate_sample_data("a"),
            TypeError,
            "Error validating sample data, "
            "data must be a list of dictionaries of numpy arrays",
        )
        self.assert_invalid(validate_sample_data(["a"]), TypeError)
        self.assert_invalid(validate_sample_data([{"a": "b"}]), TypeError)
        self.assert_invalid(
            validate_sample_data([{"a": np.array([1]), "b": "c"}]), TypeError
        )
        self.assert_invalid(validate_sample_data([{1: np.array([1])}]), TypeError)

    def test_validate_sample_data_with_model_schema(self):
        """It validates sample data against a model schema."""
        data = [
            {"a": np.array([1]), "b": np.array([["a"], ["b"]])},
            {"a": np.array([1.0]), "b": np.array([["a"]])},
        ]
        self.assert_valid(validate_sample_data_with_model_schema(data, self.schema))
        self.assert_invalid(
            validate_sample_data_with_model_schema(
                data, self.schema, batch_inference=True
            ),
            ValueError,
            "Error",
        )

        data = [
            {"a": np.array([[1], [2]]), "b": np.array([[["a"], ["b"]]])},
            {"a": np.array([[1.0]]), "b": np.array([[["a"]]])},
        ]
        self.assert_valid(
            validate_sample_data_with_model_schema(
                data, self.schema, batch_inference=True
            )
        )

        data = [
            {"a": np.array([1]), "b": np.array([["a"], ["b"]])},
            {"a": np.array([1.0])},
        ]
        self.assert_invalid(
            validate_sample_data_with_model_schema(data, self.schema),
            ValueError,
            "Error validating sample data with model input schema. Data fields",
        )
