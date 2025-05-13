import numpy as np
from unittest import TestCase
from typing import Optional
from michelangelo.lib.model_manager.schema import ModelSchemaItem, DataType
from michelangelo.lib.model_manager._private.utils.data_utils.numpy_data import (
    validate_numpy_data,
    validate_numpy_data_with_model_schema,
    validate_numpy_data_record_with_model_schema,
    validate_data_type,
    validate_shape,
)


class NumpyDataTest(TestCase):
    def setUp(self):
        self.schema_items = [
            ModelSchemaItem(name="a", data_type=DataType.FLOAT, shape=[1]),
            ModelSchemaItem(name="b", data_type=DataType.STRING, shape=[-1, 1]),
        ]

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

    def test_validate_numpy_data_valid(self):
        data = [{"a": np.array([1]), "b": np.array([2])}]
        self.assertValid(validate_numpy_data(data))

    def test_validate_numpy_data_invalid(self):
        self.assertInvalid(validate_numpy_data("a"), TypeError)
        self.assertInvalid(validate_numpy_data(["a"]), TypeError)
        self.assertInvalid(validate_numpy_data([{"a": "b"}]), TypeError)
        self.assertInvalid(validate_numpy_data([{"a": np.array([1]), "b": "c"}]), TypeError)
        self.assertInvalid(validate_numpy_data([{1: np.array([1])}]), TypeError)

    def test_validate_numpy_data_with_model_schema(self):
        data = [{"a": np.array([1]), "b": np.array([["a"], ["b"]])}, {"a": np.array([1.0]), "b": np.array([["a"]])}]
        self.assertValid(validate_numpy_data_with_model_schema(data, self.schema_items))
        data = [{"a": np.array([1]), "b": np.array([["a"], ["b"]])}, {"a": np.array([1.0])}]
        self.assertInvalid(validate_numpy_data_with_model_schema(data, self.schema_items), ValueError, "Data fields")

    def test_validate_numpy_data_record_with_model_schema(self):
        record = {"a": np.array([1]), "b": np.array([["a"], ["b"]])}
        self.assertValid(validate_numpy_data_record_with_model_schema(record, self.schema_items))
        self.assertInvalid(
            validate_numpy_data_record_with_model_schema(record, self.schema_items, batch_inference=True),
            ValueError,
            "Found mismatching number of dimensions",
        )
        record = {"a": np.array([[1], [2]]), "b": np.array([[["a"], ["b"]], [["c"], ["d"]]])}
        self.assertValid(validate_numpy_data_record_with_model_schema(record, self.schema_items, batch_inference=True))

        record = {"a": np.array([1]), "b": np.array([["a"]]), "c": np.array([1.0])}
        self.assertInvalid(validate_numpy_data_record_with_model_schema(record, self.schema_items), ValueError, "Data fields")
        record = {"a": np.array([1])}
        self.assertInvalid(validate_numpy_data_record_with_model_schema(record, self.schema_items), ValueError, "Data fields")
        record = {"a": np.array([1]), "b": np.array(["a"])}
        self.assertInvalid(validate_numpy_data_record_with_model_schema(record, self.schema_items), ValueError, "Found mismatching number of dimensions")
        record = {"a": np.array(["a"]), "b": np.array([["a"]])}
        self.assertInvalid(validate_numpy_data_record_with_model_schema(record, self.schema_items), TypeError, "Found incompatible data type")

    def test_validate_data_type(self):
        self.assertValid(validate_data_type("name", np.full([1], True), DataType.BOOLEAN))
        self.assertInvalid(validate_data_type("name", np.array([1.0]), DataType.BOOLEAN), TypeError)
        self.assertValid(validate_data_type("name", np.array([1]), DataType.INT))
        self.assertValid(validate_data_type("name", np.array([1]), DataType.SHORT))
        self.assertValid(validate_data_type("name", np.array([1]), DataType.BYTE))
        self.assertValid(validate_data_type("name", np.array([1]), DataType.CHAR))
        self.assertInvalid(validate_data_type("name", np.array([1.0]), DataType.INT), TypeError)
        self.assertInvalid(validate_data_type("name", np.array([1.0]), DataType.SHORT), TypeError)
        self.assertInvalid(validate_data_type("name", np.array([1.0]), DataType.BYTE), TypeError)
        self.assertInvalid(validate_data_type("name", np.array([1.0]), DataType.CHAR), TypeError)
        self.assertValid(validate_data_type("name", np.array([1.0]), DataType.FLOAT))
        self.assertValid(validate_data_type("name", np.array([1.0]), DataType.DOUBLE))
        self.assertValid(validate_data_type("name", np.array([1.0]), DataType.NUMERIC))
        self.assertInvalid(validate_data_type("name", np.array(["a"]), DataType.FLOAT), TypeError)
        self.assertInvalid(validate_data_type("name", np.array(["a"]), DataType.DOUBLE), TypeError)
        self.assertInvalid(validate_data_type("name", np.array(["a"]), DataType.NUMERIC), TypeError)
        self.assertValid(validate_data_type("name", np.array(["a"]), DataType.STRING))
        self.assertValid(validate_data_type("name", np.array([b"a"]), DataType.STRING))
        self.assertInvalid(validate_data_type("name", np.array([1]), DataType.STRING), TypeError)
        self.assertInvalid(validate_data_type("name", np.array([1 + 2j]), DataType.STRING), TypeError, "Data contains")

    def test_validate_shape(self):
        self.assertValid(validate_shape("name", np.array([1, 2]), [2]))
        self.assertValid(validate_shape("name", np.array([1, 2]), [-1]))
        self.assertInvalid(validate_shape("name", np.array([1, 2]), [1, 2]), ValueError, "Found mismatching number")
        self.assertInvalid(validate_shape("name", np.array([1, 2]), [3]), ValueError, "Found mismatching dimensions")
        self.assertValid(validate_shape("name", np.array([[1, 2], [3, 4]]), [2, 2]))
        self.assertValid(validate_shape("name", np.array([[1, 2], [3, 4]]), [-1, 2]))
        self.assertInvalid(validate_shape("name", np.array([[1, 2], [3, 4]]), [-1, 3]), ValueError, "Found mismatching dimensions")
        self.assertValid(validate_shape("name", np.array([[1, 2], [3, 4]]), [2], batch_inference=True))
        self.assertInvalid(
            validate_shape("name", np.array([1, 2]), [2], batch_inference=True), ValueError, "Note: batch inference is enabled for the model"
        )
