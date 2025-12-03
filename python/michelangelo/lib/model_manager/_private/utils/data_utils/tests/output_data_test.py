from typing import Optional
from unittest import TestCase

import numpy as np

from michelangelo.lib.model_manager._private.utils.data_utils import (
    validate_output_data,
    validate_output_data_with_model_schema,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem


class OutputDataTest(TestCase):
    """Tests validation helpers for model output data."""

    def setUp(self):
        """Build reusable schema fixtures."""
        self.schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1]),
            ],
            output_schema=[
                ModelSchemaItem(name="a", data_type=DataType.FLOAT, shape=[1]),
                ModelSchemaItem(name="b", data_type=DataType.STRING, shape=[-1, 1]),
            ],
        )

    def assert_valid(self, result: tuple[bool, Exception]):
        """Assert that a validation result represents success."""
        is_valid, err = result
        self.assertTrue(is_valid)
        self.assertIsNone(err)

    def assert_invalid(
        self,
        result: tuple[bool, Exception],
        error_type: type[Exception],
        message: Optional[str] = None,
    ):
        """Assert that a validation result matches the expected failure."""
        is_valid, err = result
        self.assertFalse(is_valid)
        self.assertIsInstance(err, error_type)
        if message:
            self.assertIn(message, str(err))

    def test_validate_output_data_valid(self):
        """It approves output data with the expected shapes."""
        data = {"a": np.array([1]), "b": np.array([2])}
        self.assert_valid(validate_output_data(data))

    def test_validate_output_data_invalid(self):
        """It rejects malformed output payloads."""
        self.assert_invalid(
            validate_output_data(None),
            ValueError,
            "Output of the model cannot be empty",
        )
        self.assert_invalid(
            validate_output_data({}), ValueError, "Output of the model cannot be empty"
        )
        self.assert_invalid(
            validate_output_data("a"),
            TypeError,
            "Error validating model output data, "
            "data must dictionaries of numpy arrays",
        )
        self.assert_invalid(validate_output_data(["a"]), TypeError)
        self.assert_invalid(validate_output_data({"a": "b"}), TypeError)
        self.assert_invalid(
            validate_output_data({"a": np.array([1]), "b": "c"}), TypeError
        )
        self.assert_invalid(validate_output_data({1: np.array([1])}), TypeError)

    def test_validate_output_data_with_model_schema(self):
        """It validates output payloads against a model schema."""
        data = {"a": np.array([1]), "b": np.array([["a"], ["b"]])}
        self.assert_valid(validate_output_data_with_model_schema(data, self.schema))
        self.assert_invalid(
            validate_output_data_with_model_schema(
                data, self.schema, batch_inference=True
            ),
            ValueError,
            "Error",
        )

        data = {
            "a": np.array([[1], [2]]),
            "b": np.array([[["a"], ["b"]], [["c"], ["d"]]]),
        }
        self.assert_valid(
            validate_output_data_with_model_schema(
                data, self.schema, batch_inference=True
            )
        )

        data = {"a": np.array([1]), "b": np.array([["a"], ["b"]]), "c": np.array([1])}
        self.assert_invalid(
            validate_output_data_with_model_schema(data, self.schema),
            ValueError,
            "Error validating model output data. Data fields",
        )
