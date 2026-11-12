"""Tests for the ONNX deployable model helpers."""

from __future__ import annotations

import os
import tempfile
import unittest
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

import numpy as np

from michelangelo.lib.constants.sentinel import INT32_SENTINEL
from michelangelo.lib.inference._private.lib.tensorrt_onnx import (
    _execute_onnx,
    _load_onnx_deployable_model,
    _prepare_onnx_input,
)
from michelangelo.lib.model_manager.schema.data_type import DataType
from michelangelo.lib.model_manager.schema.model_schema import ModelSchema
from michelangelo.lib.model_manager.schema.model_schema_item import ModelSchemaItem


class OnnxInferenceMethodsTest(unittest.TestCase):
    """Tests for the ONNX loading and execution helpers."""

    def _make_onnx_pkg(self, directory):
        """Build a minimal ONNX deployable package layout (stub model.onnx, mocked at load time)."""  # noqa: E501
        pkg = os.path.join(directory, "onnx_pkg")
        os.makedirs(os.path.join(pkg, "0"))
        # Write a placeholder file; actual loading is mocked in tests.
        with open(os.path.join(pkg, "0", "model.onnx"), "wb") as f:
            f.write(b"ONNX_STUB")
        with open(os.path.join(pkg, "config.pbtxt"), "w") as f:
            f.write(
                'name:"m"\nbackend:"onnxruntime"\nmax_batch_size:256\n'
                'input:[{name:"x",data_type:TYPE_FP32,dims:[1]}]\n'
                'output:[{name:"y",data_type:TYPE_FP32,dims:[1]}]\n'
            )
        return pkg

    def _mi(self, **kw):
        defaults = {
            "model_path": None,
            "model": None,
            "model_schema": None,
            "batch_inference": False,
            "_onnx_input_dtypes": {},
        }
        defaults.update(kw)
        ns = SimpleNamespace(**defaults)
        return ns

    # ------------------------------------------------------------------
    # _load_onnx_deployable_model
    # ------------------------------------------------------------------

    def test_load_onnx_missing_file_raises(self):
        """Load onnx missing file raises."""
        with tempfile.TemporaryDirectory() as d:
            pkg = os.path.join(d, "pkg")
            os.makedirs(os.path.join(pkg, "0"))
            with open(os.path.join(pkg, "config.pbtxt"), "w") as f:
                f.write('name:"m"\nbackend:"onnxruntime"\nmax_batch_size:256\n')
            mi = self._mi(model_path=pkg)
            with self.assertRaisesRegex(FileNotFoundError, "ONNX model file not found"):
                _load_onnx_deployable_model(mi)

    def test_load_onnx_ort_path(self):
        """Load onnx ort path."""
        with tempfile.TemporaryDirectory() as d:
            pkg = self._make_onnx_pkg(d)
            mi = self._mi(model_path=pkg)
            mock_session = MagicMock()
            mock_session.get_inputs.return_value = []
            with patch(
                "michelangelo.lib.model_manager.serde.model.load_deployable_model",
                return_value=mock_session,
            ):
                _load_onnx_deployable_model(mi)
            self.assertIsNotNone(mi.model)
            self.assertTrue(mi.batch_inference)

    # ------------------------------------------------------------------
    # _prepare_onnx_input
    # ------------------------------------------------------------------

    def test_prepare_casts_dtype(self):
        """Prepare casts dtype."""
        mi = SimpleNamespace()
        arr = np.array([[1, 2]], dtype=np.int32)
        out = _prepare_onnx_input(mi, arr, np.dtype(np.int64))
        self.assertEqual(out.dtype, np.int64)

    def test_prepare_makes_contiguous(self):
        """Prepare makes contiguous."""
        mi = SimpleNamespace()
        base = np.array([[1.0, 2.0], [3.0, 4.0]], dtype=np.float32)
        nc = base[:, 0:1]
        self.assertFalse(nc.flags["C_CONTIGUOUS"])
        out = _prepare_onnx_input(mi, nc, None)
        self.assertTrue(out.flags["C_CONTIGUOUS"])

    def test_prepare_makes_writeable(self):
        """Prepare makes writeable."""
        mi = SimpleNamespace()
        arr = np.array([[1.0]], dtype=np.float32)
        arr.setflags(write=False)
        out = _prepare_onnx_input(mi, arr, None)
        self.assertTrue(out.flags.writeable)

    def test_prepare_no_op_when_already_correct(self):
        """Prepare no op when already correct."""
        mi = SimpleNamespace()
        arr = np.array([[1.0]], dtype=np.float32)
        out = _prepare_onnx_input(mi, arr, np.dtype(np.float32))
        self.assertTrue(out.flags["C_CONTIGUOUS"])
        self.assertTrue(out.flags.writeable)

    def test_prepare_int_cast_maps_nan_to_int32_sentinel(self):
        # pyarrow_to_numpy's uniform-nested fast path yields float64 NaN at null
        # slots; the exported PadOrCrop1D only recognizes INT32_SENTINEL there.
        """Prepare int cast maps nan to int32 sentinel."""
        mi = SimpleNamespace()
        arr = np.array([[1.0, np.nan, 3.0], [np.nan, 5.0, 6.0]], dtype=np.float64)
        out = _prepare_onnx_input(mi, arr, np.dtype(np.int64))
        self.assertEqual(out.dtype, np.int64)
        self.assertEqual(out[0].tolist(), [1, INT32_SENTINEL, 3])
        self.assertEqual(out[1].tolist(), [INT32_SENTINEL, 5, 6])

    def test_prepare_int_cast_maps_nan_to_int32_sentinel_int32_target(self):
        """Prepare int cast maps nan to int32 sentinel int32 target."""
        mi = SimpleNamespace()
        arr = np.array([[np.nan, 2.0]], dtype=np.float64)
        out = _prepare_onnx_input(mi, arr, np.dtype(np.int32))
        self.assertEqual(out.dtype, np.int32)
        self.assertEqual(out[0].tolist(), [INT32_SENTINEL, 2])

    def test_prepare_int_cast_finite_floats_round_trip(self):
        """Prepare int cast finite floats round trip."""
        mi = SimpleNamespace()
        arr = np.array([[1.0, 2.0, 3.0]], dtype=np.float64)
        out = _prepare_onnx_input(mi, arr, np.dtype(np.int64))
        self.assertEqual(out[0].tolist(), [1, 2, 3])

    def test_prepare_float_target_keeps_nan(self):
        # Float targets must keep NaN semantics (FLOAT_SENTINEL) — no int
        # sentinel mapping.
        """Prepare float target keeps nan."""
        mi = SimpleNamespace()
        arr = np.array([[np.nan, 2.0]], dtype=np.float64)
        out = _prepare_onnx_input(mi, arr, np.dtype(np.float32))
        self.assertEqual(out.dtype, np.float32)
        self.assertTrue(np.isnan(out[0, 0]))
        self.assertEqual(out[0, 1], 2.0)

    def test_prepare_int_target_from_int_array_no_nan_handling(self):
        # Already-integer inputs pass through the existing cast path untouched.
        """Prepare int target from int array no nan handling."""
        mi = SimpleNamespace()
        arr = np.array([[5, 6]], dtype=np.int32)
        out = _prepare_onnx_input(mi, arr, np.dtype(np.int64))
        self.assertEqual(out.dtype, np.int64)
        self.assertEqual(out[0].tolist(), [5, 6])

    # ------------------------------------------------------------------
    # _execute_onnx
    # ------------------------------------------------------------------

    def test_execute_onnx_ort_path(self):
        """Execute onnx ort path."""
        schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1])
            ],
            output_schema=[
                ModelSchemaItem(name="y", data_type=DataType.FLOAT, shape=[1])
            ],
        )
        mock_session = MagicMock()
        mock_session.run.return_value = [np.array([[2.0]], dtype=np.float32)]
        mi = SimpleNamespace(
            model_schema=schema,
            _onnx_input_dtypes={"x": np.dtype(np.float32)},
            model=mock_session,
        )
        out = _execute_onnx(mi, {"x": np.array([[1.0]], dtype=np.float32)})
        self.assertIn("y", out)

    def test_load_onnx_ort_path_reads_input_dtypes(self):
        """Input dtypes come from the session's declared input types."""
        with tempfile.TemporaryDirectory() as d:
            pkg = self._make_onnx_pkg(d)
            mi = self._mi(model_path=pkg)
            mock_session = MagicMock()
            mock_session.get_inputs.return_value = [
                SimpleNamespace(name="x", type="tensor(float)"),
                SimpleNamespace(name="y", type="tensor(string)"),
            ]
            with patch(
                "michelangelo.lib.model_manager.serde.model.load_deployable_model",
                return_value=mock_session,
            ):
                _load_onnx_deployable_model(mi)
            self.assertEqual(mi._onnx_input_dtypes["x"], np.dtype(np.float32))
            self.assertIsNone(mi._onnx_input_dtypes["y"])
            self.assertEqual(mi.model_schema.input_schema[0].name, "x")
