"""Tests for torch model utilities."""

from unittest import TestCase

import torch

from michelangelo.lib.model_manager._private.utils.torch_utils.model import (
    is_state_dict,
    torch_dtype_to_data_type,
)
from michelangelo.lib.model_manager.schema import DataType


class IsStateDictTest(TestCase):
    """Tests for is_state_dict."""

    def test_state_dict_of_tensors(self):
        self.assertTrue(is_state_dict({"w": torch.zeros(2), "b": torch.zeros(1)}))

    def test_empty_dict(self):
        self.assertFalse(is_state_dict({}))

    def test_non_tensor_value(self):
        self.assertFalse(is_state_dict({"w": torch.zeros(2), "x": 5}))

    def test_not_a_dict(self):
        self.assertFalse(is_state_dict([torch.zeros(2)]))
        self.assertFalse(is_state_dict("nope"))


class TorchDtypeToDataTypeTest(TestCase):
    """Tests for torch_dtype_to_data_type."""

    def test_known_dtypes(self):
        cases = {
            torch.float32: DataType.FLOAT,
            torch.float64: DataType.DOUBLE,
            torch.int32: DataType.INT,
            torch.int16: DataType.SHORT,
            torch.int8: DataType.BYTE,
            torch.int64: DataType.LONG,
            torch.bool: DataType.BOOLEAN,
        }
        for dtype, expected in cases.items():
            self.assertEqual(torch_dtype_to_data_type(dtype), expected)

    def test_unsupported_dtype_raises(self):
        with self.assertRaisesRegex(ValueError, "Cannot convert torch.dtype"):
            torch_dtype_to_data_type(torch.float16)
