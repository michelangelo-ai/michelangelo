"""Tests for TritonBackendType constants."""

from unittest import TestCase

from michelangelo.lib.model_manager._private.constants import TritonBackendType


class TritonBackendTypeTest(TestCase):
    """Tests the Triton backend type constants."""

    def test_triton_backend_type_constants(self):
        """It exposes expected names for each backend."""
        self.assertEqual(TritonBackendType.PYTHON, "python")
        self.assertEqual(TritonBackendType.TORCH, "pytorch")
        self.assertEqual(TritonBackendType.TENSORRT, "tensorrt")
        self.assertEqual(TritonBackendType.ONNX, "onnxruntime")
