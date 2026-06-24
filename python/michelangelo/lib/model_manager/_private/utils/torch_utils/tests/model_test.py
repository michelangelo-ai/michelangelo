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
        """A dict of tensors is a valid state_dict."""
        self.assertTrue(is_state_dict({"w": torch.zeros(2), "b": torch.zeros(1)}))

    def test_empty_dict(self):
        """An empty dict is not a valid state_dict."""
        self.assertFalse(is_state_dict({}))

    def test_non_tensor_value(self):
        """A dict with a non-tensor value is not a valid state_dict."""
        self.assertFalse(is_state_dict({"w": torch.zeros(2), "x": 5}))

    def test_not_a_dict(self):
        """A non-dict value is not a valid state_dict."""
        self.assertFalse(is_state_dict([torch.zeros(2)]))
        self.assertFalse(is_state_dict("nope"))


class TorchDtypeToDataTypeTest(TestCase):
    """Tests for torch_dtype_to_data_type."""

    def test_known_dtypes(self):
        """Known torch dtypes map to the correct DataType."""
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
        """An unsupported dtype raises ValueError."""
        with self.assertRaisesRegex(ValueError, "Cannot convert torch.dtype"):
            torch_dtype_to_data_type(torch.float16)


class TensorToNumpyTest(TestCase):
    """Tests for tensor_to_numpy."""

    def test_tensor_converted_to_numpy(self):
        """A torch.Tensor is detached and converted to numpy."""
        import numpy as np

        from michelangelo.lib.model_manager._private.utils.torch_utils.model import (
            tensor_to_numpy,
        )

        t = torch.tensor([1.0, 2.0, 3.0])
        result = tensor_to_numpy(t)
        self.assertIsInstance(result, np.ndarray)

    def test_non_tensor_passed_through(self):
        """A non-tensor value is returned unchanged."""
        from michelangelo.lib.model_manager._private.utils.torch_utils.model import (
            tensor_to_numpy,
        )

        self.assertEqual(tensor_to_numpy(42), 42)
        self.assertEqual(tensor_to_numpy("hello"), "hello")
