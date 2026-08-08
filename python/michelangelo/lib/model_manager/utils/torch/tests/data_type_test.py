"""Tests for data_type_to_torch_dtype and torch_dtype_to_data_type."""

from unittest import TestCase

import torch

from michelangelo.lib.model_manager.schema import DataType
from michelangelo.lib.model_manager.utils.torch.data_type import (
    data_type_to_torch_dtype,
    torch_dtype_to_data_type,
)


class DataTypeToTorchDtypeTest(TestCase):
    """Tests for data_type_to_torch_dtype mapping."""

    def test_float_maps_to_float32(self):
        """It maps DataType.FLOAT to torch.float32."""
        self.assertEqual(data_type_to_torch_dtype(DataType.FLOAT), torch.float32)

    def test_double_maps_to_float64(self):
        """It maps DataType.DOUBLE to torch.float64."""
        self.assertEqual(data_type_to_torch_dtype(DataType.DOUBLE), torch.float64)

    def test_int_maps_to_int32(self):
        """It maps DataType.INT to torch.int32."""
        self.assertEqual(data_type_to_torch_dtype(DataType.INT), torch.int32)

    def test_short_maps_to_int16(self):
        """It maps DataType.SHORT to torch.int16."""
        self.assertEqual(data_type_to_torch_dtype(DataType.SHORT), torch.int16)

    def test_byte_maps_to_int8(self):
        """It maps DataType.BYTE to torch.int8."""
        self.assertEqual(data_type_to_torch_dtype(DataType.BYTE), torch.int8)

    def test_long_maps_to_int64(self):
        """It maps DataType.LONG to torch.int64."""
        self.assertEqual(data_type_to_torch_dtype(DataType.LONG), torch.int64)

    def test_boolean_maps_to_bool(self):
        """It maps DataType.BOOLEAN to torch.bool."""
        self.assertEqual(data_type_to_torch_dtype(DataType.BOOLEAN), torch.bool)

    def test_unsupported_data_type_raises_value_error(self):
        """It raises ValueError for a DataType with no torch.dtype mapping."""
        with self.assertRaises(ValueError) as ctx:
            data_type_to_torch_dtype(DataType.STRING)
        self.assertIn("Cannot convert data type", str(ctx.exception))
        self.assertIn("STRING", str(ctx.exception))

    def test_unknown_raises_value_error(self):
        """It raises ValueError for DataType.UNKNOWN."""
        with self.assertRaises(ValueError) as ctx:
            data_type_to_torch_dtype(DataType.UNKNOWN)
        self.assertIn("Cannot convert data type", str(ctx.exception))

    def test_invalid_raises_value_error(self):
        """It raises ValueError for DataType.INVALID."""
        with self.assertRaises(ValueError):
            data_type_to_torch_dtype(DataType.INVALID)


class TorchDtypeToDataTypeTest(TestCase):
    """Tests for torch_dtype_to_data_type mapping."""

    def test_supported_dtypes(self):
        """It maps each supported torch.dtype back to its DataType."""
        cases = [
            (torch.float32, DataType.FLOAT),
            (torch.float64, DataType.DOUBLE),
            (torch.int32, DataType.INT),
            (torch.int16, DataType.SHORT),
            (torch.int8, DataType.BYTE),
            (torch.int64, DataType.LONG),
            (torch.bool, DataType.BOOLEAN),
        ]
        for dtype, expected in cases:
            with self.subTest(dtype=dtype):
                self.assertEqual(torch_dtype_to_data_type(dtype), expected)

    def test_float16_raises(self):
        """It raises ValueError for torch.float16 (not yet supported)."""
        with self.assertRaises(ValueError) as ctx:
            torch_dtype_to_data_type(torch.float16)
        self.assertIn("float16", str(ctx.exception))

    def test_bfloat16_raises(self):
        """It raises ValueError for torch.bfloat16 (not yet supported)."""
        with self.assertRaises(ValueError) as ctx:
            torch_dtype_to_data_type(torch.bfloat16)
        self.assertIn("bfloat16", str(ctx.exception))

    def test_unsupported_dtype_raises(self):
        """It raises ValueError for a dtype with no DataType equivalent."""
        with self.assertRaises(ValueError):
            torch_dtype_to_data_type(torch.complex64)
