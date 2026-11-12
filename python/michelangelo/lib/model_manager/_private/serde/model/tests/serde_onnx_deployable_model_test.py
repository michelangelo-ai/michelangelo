"""Tests for creating ONNX Runtime sessions from deployable packages."""

import os
import tempfile
from unittest import TestCase

import numpy as np
import pytest

from michelangelo.lib.model_manager._private.serde.model import (
    load_onnx_deployable_model,
)


class OnnxDeployableModelTest(TestCase):
    """Loading ``0/model.onnx``."""

    def setUp(self) -> None:
        """Create a scratch package directory."""
        self._tmp = tempfile.TemporaryDirectory()
        self.root = self._tmp.name

    def tearDown(self) -> None:
        """Remove the scratch directory."""
        self._tmp.cleanup()

    def test_missing_version_dir(self):
        """A package with no ``0`` directory is a FileNotFoundError."""
        with self.assertRaisesRegex(FileNotFoundError, "ONNX model file not found"):
            load_onnx_deployable_model(self.root)

    def test_missing_model_file(self):
        """A ``0`` directory with no model.onnx is a FileNotFoundError."""
        os.makedirs(os.path.join(self.root, "0"))
        with self.assertRaisesRegex(FileNotFoundError, "0/model.onnx"):
            load_onnx_deployable_model(self.root)

    def test_model_with_external_data_runs(self):
        """Weights stored in a sidecar file next to model.onnx are found."""
        onnx = pytest.importorskip("onnx")
        pytest.importorskip("onnxruntime")
        from onnx import TensorProto, helper, numpy_helper

        weight = numpy_helper.from_array(
            np.array([[2.0, 0.0], [0.0, 3.0]], dtype=np.float32), name="w"
        )
        graph = helper.make_graph(
            [helper.make_node("MatMul", ["x", "w"], ["y"])],
            "g",
            [helper.make_tensor_value_info("x", TensorProto.FLOAT, [1, 2])],
            [helper.make_tensor_value_info("y", TensorProto.FLOAT, [1, 2])],
            initializer=[weight],
        )
        model = helper.make_model(
            graph, opset_imports=[helper.make_opsetid("", 14)], ir_version=8
        )
        version_dir = os.path.join(self.root, "0")
        os.makedirs(version_dir)
        onnx.save_model(
            model,
            os.path.join(version_dir, "model.onnx"),
            save_as_external_data=True,
            all_tensors_to_one_file=True,
            location="weights.bin",
            size_threshold=0,
        )
        self.assertTrue(os.path.exists(os.path.join(version_dir, "weights.bin")))

        session = load_onnx_deployable_model(self.root)

        (out,) = session.run(None, {"x": np.array([[1.0, 1.0]], dtype=np.float32)})
        np.testing.assert_allclose(out, [[2.0, 3.0]])
