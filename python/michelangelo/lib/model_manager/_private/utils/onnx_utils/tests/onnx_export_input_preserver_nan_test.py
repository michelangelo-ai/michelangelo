"""Tests that the ONNX input preserver stays finite when an input has NaN or inf.

Most tests only exercise ``torch``. The export-level test at the bottom needs
``onnx`` and ``onnxruntime`` and is skipped when either is missing.
"""

from __future__ import annotations

from unittest import TestCase

import pytest
import torch

from michelangelo.lib.model_manager._private.utils.onnx_utils.onnx_export_helpers import (  # noqa: E501
    onnx_export_attach_inputs_to_output,
    onnx_export_input_preserver,
)


def _preserver(*tensors: torch.Tensor) -> torch.Tensor:
    return onnx_export_input_preserver(
        tensors, ref_dtype=torch.float32, ref_device=torch.device("cpu")
    )


class OnnxExportInputPreserverNonFiniteTest(TestCase):
    """The preserver term is exactly zero for non-finite floating-point inputs."""

    def test_nan_input_gives_zero(self):
        """A NaN in one row does not make the preserver NaN."""
        x = torch.tensor([[1.0, float("nan")], [2.0, 3.0]])
        self.assertEqual(_preserver(x).item(), 0.0)

    def test_inf_inputs_give_zero(self):
        """Positive and negative infinity do not make the preserver NaN."""
        x = torch.tensor([[float("inf"), float("-inf")], [2.0, 3.0]])
        self.assertEqual(_preserver(x).item(), 0.0)

    def test_half_precision_nan_gives_zero(self):
        """Non-float32 floating-point inputs are sanitized too."""
        x = torch.tensor([float("nan"), 1.0], dtype=torch.float16)
        self.assertEqual(_preserver(x).item(), 0.0)

    def test_integer_inputs_unaffected(self):
        """Integer inputs are used as-is and give zero."""
        x = torch.tensor([[1, 2], [3, 4]], dtype=torch.int64)
        result = _preserver(x)
        self.assertEqual(result.item(), 0.0)
        self.assertEqual(result.dtype, torch.float32)

    def test_mixed_inputs_with_nan_give_zero(self):
        """A NaN float input next to an integer input still gives zero."""
        f = torch.tensor([float("nan")])
        i = torch.tensor([5], dtype=torch.int32)
        self.assertEqual(_preserver(f, i).item(), 0.0)

    def test_finite_inputs_unchanged(self):
        """Finite inputs keep a zero preserver of the requested dtype and shape."""
        result = _preserver(torch.randn(3, 4), torch.randn(2))
        self.assertEqual(result.item(), 0.0)
        self.assertEqual(result.shape, ())
        self.assertEqual(result.dtype, torch.float32)

    def test_input_tensor_not_modified(self):
        """Sanitizing does not mutate the caller's tensor."""
        x = torch.tensor([float("nan"), 1.0])
        _preserver(x)
        self.assertTrue(torch.isnan(x[0]))


class OnnxExportAttachInputsNonFiniteTest(TestCase):
    """Attaching inputs to an output keeps a finite output finite."""

    def test_tensor_output_stays_finite_with_nan_row(self):
        """The repro: NaN in one input row must not change any output row."""
        x = torch.tensor([[1.0, float("nan")], [2.0, 3.0]])
        out = torch.ones(2, 1)
        result = onnx_export_attach_inputs_to_output(out, (x,))
        self.assertEqual(result.flatten().tolist(), [1.0, 1.0])

    def test_dict_output_stays_finite_with_inf_row(self):
        """The dict branch also stays finite when an input holds infinity."""
        x = torch.tensor([[float("inf")], [2.0]])
        out = {"score": torch.ones(2, 1)}
        result = onnx_export_attach_inputs_to_output(out, (x,))
        self.assertEqual(result["score"].flatten().tolist(), [1.0, 1.0])

    def test_finite_inputs_leave_output_identical(self):
        """For finite inputs the output values are unchanged."""
        out = torch.tensor([[0.25], [0.75]])
        result = onnx_export_attach_inputs_to_output(out, (torch.randn(2, 3),))
        self.assertTrue(torch.equal(result, out))


class OnnxExportInputPreserverGraphTest(TestCase):
    """Export-level check that the input survives in the ONNX graph."""

    def test_nan_input_kept_as_graph_input(self):
        """The input stays in the exported graph and the output is finite."""
        onnx_pkg = pytest.importorskip("onnx")
        ort = pytest.importorskip("onnxruntime")
        import os
        import tempfile

        class _Model(torch.nn.Module):
            def forward(self, a, unused):
                return onnx_export_attach_inputs_to_output(a + 1.0, (a, unused))

        a = torch.ones(2, 1)
        unused = torch.ones(2, 1)
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "m.onnx")
            torch.onnx.export(
                _Model(),
                (a, unused),
                path,
                input_names=["a", "unused"],
                dynamo=False,
            )
            graph_inputs = [i.name for i in onnx_pkg.load(path).graph.input]
            self.assertIn("unused", graph_inputs)
            sess = ort.InferenceSession(path)
            bad = torch.tensor([[1.0], [float("nan")]]).numpy()
            (res,) = sess.run(None, {"a": a.numpy(), "unused": bad})
            self.assertEqual(res.flatten().tolist(), [2.0, 2.0])
