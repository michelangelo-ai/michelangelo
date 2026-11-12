"""Tests for onnx_tensor_type_to_np_dtype."""

from __future__ import annotations

import unittest

import numpy as np

from michelangelo.lib.model_manager.utils.onnx.data_type import (
    ONNX_TENSOR_TYPE_TO_NP_DTYPE,
    onnx_tensor_type_to_np_dtype,
)


class OnnxTensorTypeToNpDtypeTest(unittest.TestCase):
    """Tests for the ONNX tensor-type to numpy dtype mapping."""

    def test_float_maps_to_float32(self):
        """Test that tensor(float) maps to float32."""
        self.assertEqual(
            onnx_tensor_type_to_np_dtype("tensor(float)"), np.dtype(np.float32)
        )

    def test_float16_maps_to_float16(self):
        """Test that tensor(float16) maps to float16."""
        self.assertEqual(
            onnx_tensor_type_to_np_dtype("tensor(float16)"), np.dtype(np.float16)
        )

    def test_double_maps_to_float64(self):
        """Test that tensor(double) maps to float64."""
        self.assertEqual(
            onnx_tensor_type_to_np_dtype("tensor(double)"), np.dtype(np.float64)
        )

    def test_int64_maps_to_int64(self):
        """Test that tensor(int64) maps to int64."""
        self.assertEqual(
            onnx_tensor_type_to_np_dtype("tensor(int64)"), np.dtype(np.int64)
        )

    def test_bool_maps_to_bool(self):
        """Test that tensor(bool) maps to bool."""
        self.assertEqual(
            onnx_tensor_type_to_np_dtype("tensor(bool)"), np.dtype(np.bool_)
        )

    def test_unknown_returns_none(self):
        """Test that unmapped types return None."""
        self.assertIsNone(onnx_tensor_type_to_np_dtype("tensor(string)"))
        self.assertIsNone(onnx_tensor_type_to_np_dtype("not-a-tensor-type"))

    def test_constant_covers_all_numeric_int_and_bool_types(self):
        """Test the mapping covers every numeric, integer and bool type."""
        expected = {
            "tensor(float)",
            "tensor(float16)",
            "tensor(double)",
            "tensor(int8)",
            "tensor(int16)",
            "tensor(int32)",
            "tensor(int64)",
            "tensor(uint8)",
            "tensor(uint16)",
            "tensor(uint32)",
            "tensor(uint64)",
            "tensor(bool)",
        }
        self.assertEqual(set(ONNX_TENSOR_TYPE_TO_NP_DTYPE.keys()), expected)
