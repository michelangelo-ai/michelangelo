from typing import Optional
from unittest import TestCase

import numpy as np

from michelangelo.lib.model_manager._private.utils.data_utils.numpy_data import (
    validate_data_type,
    validate_numpy_data,
    validate_numpy_data_record_with_model_schema,
    validate_numpy_data_with_model_schema,
    validate_shape,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchemaItem


class NumpyDataTest(TestCase):
    """Tests numpy data validation helpers."""

    def setUp(self):
        """Prepare schema fixtures shared across tests."""
        self.schema_items = [
            ModelSchemaItem(name="a", data_type=DataType.FLOAT, shape=[1]),
            ModelSchemaItem(name="b", data_type=DataType.STRING, shape=[-1, 1]),
        ]

    def assert_valid(self, result: tuple[bool, Exception]):
        """Assert that validation result reports success."""
        is_valid, err = result
        self.assertTrue(is_valid)
        self.assertIsNone(err)

    def assert_invalid(
        self,
        result: tuple[bool, Exception],
        error_type: type[Exception],
        message: Optional[str] = None,
    ):
        """Assert that validation result reports the expected failure."""
        is_valid, err = result
        self.assertFalse(is_valid)
        self.assertIsInstance(err, error_type)
        if message:
            self.assertIn(message, str(err))

    def test_validate_numpy_data_valid(self):
        """It passes validation when the numpy data matches expectations."""
        data = [{"a": np.array([1]), "b": np.array([2])}]
        self.assert_valid(validate_numpy_data(data))

    def test_validate_numpy_data_invalid(self):
        """It fails validation for malformed numpy inputs."""
        self.assert_invalid(validate_numpy_data("a"), TypeError)
        self.assert_invalid(validate_numpy_data(["a"]), TypeError)
        self.assert_invalid(validate_numpy_data([{"a": "b"}]), TypeError)
        self.assert_invalid(
            validate_numpy_data([{"a": np.array([1]), "b": "c"}]), TypeError
        )
        self.assert_invalid(validate_numpy_data([{1: np.array([1])}]), TypeError)

    def test_validate_numpy_data_with_model_schema(self):
        """It validates numpy data against the model schema."""
        data = [
            {"a": np.array([1]), "b": np.array([["a"], ["b"]])},
            {"a": np.array([1.0]), "b": np.array([["a"]])},
        ]
        self.assert_valid(
            validate_numpy_data_with_model_schema(data, self.schema_items)
        )
        data = [
            {"a": np.array([1]), "b": np.array([["a"], ["b"]])},
            {"a": np.array([1.0])},
        ]
        self.assert_invalid(
            validate_numpy_data_with_model_schema(data, self.schema_items),
            ValueError,
            "Data fields",
        )

    def test_validate_numpy_data_record_with_model_schema(self):
        """It validates individual records when using the model schema."""
        record = {"a": np.array([1]), "b": np.array([["a"], ["b"]])}
        self.assert_valid(
            validate_numpy_data_record_with_model_schema(record, self.schema_items)
        )
        self.assert_invalid(
            validate_numpy_data_record_with_model_schema(
                record, self.schema_items, batch_inference=True
            ),
            ValueError,
            "Found mismatching number of dimensions",
        )
        record = {
            "a": np.array([[1], [2]]),
            "b": np.array([[["a"], ["b"]], [["c"], ["d"]]]),
        }
        self.assert_valid(
            validate_numpy_data_record_with_model_schema(
                record, self.schema_items, batch_inference=True
            )
        )

        record = {"a": np.array([1]), "b": np.array([["a"]]), "c": np.array([1.0])}
        self.assert_invalid(
            validate_numpy_data_record_with_model_schema(record, self.schema_items),
            ValueError,
            "Data fields",
        )
        record = {"a": np.array([1])}
        self.assert_invalid(
            validate_numpy_data_record_with_model_schema(record, self.schema_items),
            ValueError,
            "Data fields",
        )
        record = {"a": np.array([1]), "b": np.array(["a"])}
        self.assert_invalid(
            validate_numpy_data_record_with_model_schema(record, self.schema_items),
            ValueError,
            "Found mismatching number of dimensions",
        )
        record = {"a": np.array(["a"]), "b": np.array([["a"]])}
        self.assert_invalid(
            validate_numpy_data_record_with_model_schema(record, self.schema_items),
            TypeError,
            "Found incompatible data type",
        )

    def test_validate_data_type(self):
        """It checks numpy dtypes against the data type enum."""
        self.assert_valid(
            validate_data_type("name", np.full([1], True), DataType.BOOLEAN)
        )
        self.assert_invalid(
            validate_data_type("name", np.array([1.0]), DataType.BOOLEAN), TypeError
        )
        self.assert_valid(validate_data_type("name", np.array([1]), DataType.INT))
        self.assert_valid(validate_data_type("name", np.array([1]), DataType.SHORT))
        self.assert_valid(validate_data_type("name", np.array([1]), DataType.BYTE))
        self.assert_valid(validate_data_type("name", np.array([1]), DataType.CHAR))
        self.assert_invalid(
            validate_data_type("name", np.array([1.0]), DataType.INT), TypeError
        )
        self.assert_invalid(
            validate_data_type("name", np.array([1.0]), DataType.SHORT), TypeError
        )
        self.assert_invalid(
            validate_data_type("name", np.array([1.0]), DataType.BYTE), TypeError
        )
        self.assert_invalid(
            validate_data_type("name", np.array([1.0]), DataType.CHAR), TypeError
        )
        self.assert_valid(validate_data_type("name", np.array([1.0]), DataType.FLOAT))
        self.assert_valid(validate_data_type("name", np.array([1.0]), DataType.DOUBLE))
        self.assert_invalid(
            validate_data_type("name", np.array(["a"]), DataType.FLOAT), TypeError
        )
        self.assert_invalid(
            validate_data_type("name", np.array(["a"]), DataType.DOUBLE), TypeError
        )
        self.assert_valid(validate_data_type("name", np.array(["a"]), DataType.STRING))
        self.assert_valid(validate_data_type("name", np.array([b"a"]), DataType.STRING))
        self.assert_invalid(
            validate_data_type("name", np.array([1]), DataType.STRING), TypeError
        )
        self.assert_invalid(
            validate_data_type("name", np.array([1 + 2j]), DataType.STRING),
            TypeError,
            "Data contains",
        )

    def test_validate_shape(self):
        """It validates array shapes against expected dimensions."""
        self.assert_valid(validate_shape("name", np.array([1, 2]), [2]))
        self.assert_valid(validate_shape("name", np.array([1, 2]), [-1]))
        self.assert_invalid(
            validate_shape("name", np.array([1, 2]), [1, 2]),
            ValueError,
            "Found mismatching number",
        )
        self.assert_invalid(
            validate_shape("name", np.array([1, 2]), [3]),
            ValueError,
            "Found mismatching dimensions",
        )
        self.assert_valid(validate_shape("name", np.array([[1, 2], [3, 4]]), [2, 2]))
        self.assert_valid(validate_shape("name", np.array([[1, 2], [3, 4]]), [-1, 2]))
        self.assert_invalid(
            validate_shape("name", np.array([[1, 2], [3, 4]]), [-1, 3]),
            ValueError,
            "Found mismatching dimensions",
        )
        self.assert_valid(
            validate_shape(
                "name", np.array([[1, 2], [3, 4]]), [2], batch_inference=True
            )
        )
        self.assert_invalid(
            validate_shape("name", np.array([1, 2]), [2], batch_inference=True),
            ValueError,
            "Note: batch inference is enabled for the model",
        )
