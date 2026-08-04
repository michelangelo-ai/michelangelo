"""Tests for the shared PyTorch-to-ONNX export entry point.

``export_torch_to_onnx`` is the single implementation used by both the
model manager packager (non-fused models) and the model fuser (fused
models), consolidated from what used to be two independently-drifting
inline implementations (mirroring internal's ``ea07350f69c``). Fuser- and
packager-specific integration coverage lives in
``model_fuser/tests/fuse_test.py``'s ``FuseModelsToOnnxTest`` and
``packager/torch_triton/tests/onnx_conversion_test.py`` respectively; this
file is the single source of truth for export-path unit coverage.
"""

from __future__ import annotations

import os
import tempfile
from unittest import TestCase, mock

import numpy as np
import pytorch_lightning as pl
import torch
import torch.nn as nn

from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.lib.model_manager.utils.onnx import torch_onnx
from michelangelo.lib.model_manager.utils.onnx.torch_onnx import (
    export_torch_to_onnx,
    prepare_sample_inputs,
)

_TORCH_ONNX_MODULE = "michelangelo.lib.model_manager.utils.onnx.torch_onnx"


class _SimpleModel(nn.Module):
    """Single-tensor-input model used for the non-tuple-wrapper (packager) path."""

    def __init__(self, in_dim: int = 4, out_dim: int = 2) -> None:
        super().__init__()
        self.lin = nn.Linear(in_dim, out_dim)

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        return self.lin(x)


class _DictModel(nn.Module):
    """Dict-input model used for the tuple-wrapper (fuser) path."""

    def forward(self, inputs: dict[str, torch.Tensor]) -> torch.Tensor:
        return inputs["a"] + inputs["b"]


def _schema() -> ModelSchema:
    return ModelSchema(
        input_schema=[ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[4])],
        output_schema=[ModelSchemaItem(name="y", data_type=DataType.FLOAT, shape=[2])],
    )


class PrepareSampleInputsTest(TestCase):
    """Tests for ``prepare_sample_inputs``."""

    def test_tensor_inputs(self):
        """Tensor inputs."""
        data = {"x": torch.randn(1, 4)}
        result = prepare_sample_inputs(["x"], data)
        self.assertEqual(len(result), 1)
        self.assertIsInstance(result[0], torch.Tensor)

    def test_numpy_inputs(self):
        """Numpy inputs."""
        data = {"x": np.random.default_rng().standard_normal((1, 4)).astype(np.float32)}
        result = prepare_sample_inputs(["x"], data)
        self.assertEqual(len(result), 1)
        self.assertIsInstance(result[0], torch.Tensor)

    def test_missing_key_raises(self):
        """Missing key raises."""
        with self.assertRaisesRegex(ValueError, "missing required input"):
            prepare_sample_inputs(["x"], {"wrong": torch.randn(1, 4)})

    def test_bad_type_raises(self):
        """Bad type raises."""
        with self.assertRaisesRegex(TypeError, "must be torch.Tensor or numpy.ndarray"):
            prepare_sample_inputs(["x"], {"x": [1.0, 2.0]})

    def test_expands_batch_dim(self):
        """Expands batch dim."""
        data = {"x": torch.randn(1, 4)}
        result = prepare_sample_inputs(["x"], data)
        self.assertEqual(result[0].shape[0], 2)


class ExportTorchToOnnxTest(TestCase):
    """Tests for ``export_torch_to_onnx``."""

    def test_export_simple_model(self):
        """Export simple model."""
        with tempfile.TemporaryDirectory() as d:
            dest = os.path.join(d, "model.onnx")
            model = _SimpleModel()
            model.eval()
            export_torch_to_onnx(
                model=model,
                dest_path=dest,
                sample_inputs=(torch.randn(2, 4),),
                input_names=["x"],
                output_names=["y"],
            )
            self.assertTrue(os.path.isfile(dest))

    def test_export_with_schema_shape_forcing(self):
        """Export with schema shape forcing."""
        with tempfile.TemporaryDirectory() as d:
            dest = os.path.join(d, "model.onnx")
            model = _SimpleModel()
            model.eval()
            export_torch_to_onnx(
                model=model,
                dest_path=dest,
                sample_inputs=(torch.randn(2, 4),),
                input_names=["x"],
                output_names=["y"],
                model_schemas=[_schema()],
            )
            self.assertTrue(os.path.isfile(dest))

    def test_export_creates_dest_dir(self):
        """Export creates dest dir."""
        with tempfile.TemporaryDirectory() as d:
            dest = os.path.join(d, "subdir", "model.onnx")
            model = _SimpleModel()
            model.eval()
            export_torch_to_onnx(
                model=model,
                dest_path=dest,
                sample_inputs=(torch.randn(2, 4),),
                input_names=["x"],
                output_names=["y"],
            )
            self.assertTrue(os.path.isfile(dest))

    def test_export_with_dynamic_batching_disabled(self):
        """Export with dynamic batching disabled."""
        with (
            tempfile.TemporaryDirectory() as d,
            mock.patch("torch.onnx.export") as export_mock,
        ):
            dest = os.path.join(d, "model.onnx")
            model = _SimpleModel()
            model.eval()
            export_torch_to_onnx(
                model=model,
                dest_path=dest,
                sample_inputs=(torch.randn(2, 4),),
                input_names=["x"],
                output_names=["y"],
                enable_dynamic_batching=False,
            )
            self.assertIsNone(export_mock.call_args.kwargs.get("dynamic_axes"))

    def test_export_with_external_data_passed_when_supported(self):
        """``external_data=True`` reaches export when torch supports it."""
        with (
            tempfile.TemporaryDirectory() as d,
            mock.patch("torch.onnx.export") as export_mock,
            mock.patch(
                f"{_TORCH_ONNX_MODULE}.torch_export_supports_external_data",
                return_value=True,
            ),
        ):
            dest = os.path.join(d, "model.onnx")
            model = _SimpleModel()
            model.eval()
            export_torch_to_onnx(
                model=model,
                dest_path=dest,
                sample_inputs=(torch.randn(2, 4),),
                input_names=["x"],
                output_names=["y"],
                external_data=True,
            )
            self.assertTrue(export_mock.call_args.kwargs.get("external_data"))

    def test_export_omits_external_data_when_not_supported(self):
        """``external_data`` is omitted from export kwargs on older torch versions."""
        with (
            tempfile.TemporaryDirectory() as d,
            mock.patch("torch.onnx.export") as export_mock,
            mock.patch(
                f"{_TORCH_ONNX_MODULE}.torch_export_supports_external_data",
                return_value=False,
            ),
        ):
            dest = os.path.join(d, "model.onnx")
            model = _SimpleModel()
            model.eval()
            export_torch_to_onnx(
                model=model,
                dest_path=dest,
                sample_inputs=(torch.randn(2, 4),),
                input_names=["x"],
                output_names=["y"],
                external_data=True,
            )
            self.assertNotIn("external_data", export_mock.call_args.kwargs)

    def test_export_with_tuple_wrapper(self):
        """Export with tuple wrapper."""
        with tempfile.TemporaryDirectory() as d:
            dest = os.path.join(d, "model.onnx")
            model = _DictModel()
            model.eval()
            export_torch_to_onnx(
                model=model,
                dest_path=dest,
                sample_inputs=(torch.randn(2, 4), torch.randn(2, 4)),
                input_names=["a", "b"],
                output_names=["y"],
                use_tuple_wrapper=True,
                input_key_order=["a", "b"],
            )
            self.assertTrue(os.path.isfile(dest))

    def test_tuple_wrapper_without_key_order_raises(self):
        """Tuple wrapper without key order raises."""
        with tempfile.TemporaryDirectory() as d:
            dest = os.path.join(d, "model.onnx")
            model = _DictModel()
            model.eval()
            with self.assertRaisesRegex(ValueError, "input_key_order is required"):
                export_torch_to_onnx(
                    model=model,
                    dest_path=dest,
                    sample_inputs=(torch.randn(2, 4), torch.randn(2, 4)),
                    input_names=["a", "b"],
                    output_names=["y"],
                    use_tuple_wrapper=True,
                )

    def test_export_with_none_schemas(self):
        """Export with none schemas."""
        with tempfile.TemporaryDirectory() as d:
            dest = os.path.join(d, "model.onnx")
            model = _SimpleModel()
            model.eval()
            export_torch_to_onnx(
                model=model,
                dest_path=dest,
                sample_inputs=(torch.randn(2, 4),),
                input_names=["x"],
                output_names=["y"],
                model_schemas=None,
            )
            self.assertTrue(os.path.isfile(dest))

    def test_export_with_empty_output_names(self):
        """Export with empty output names."""
        with tempfile.TemporaryDirectory() as d:
            dest = os.path.join(d, "model.onnx")
            model = _SimpleModel()
            model.eval()
            export_torch_to_onnx(
                model=model,
                dest_path=dest,
                sample_inputs=(torch.randn(2, 4),),
                input_names=["x"],
                output_names=[],
            )
            self.assertTrue(os.path.isfile(dest))

    def test_export_with_lightning_module(self):
        """Export with lightning module invokes the jit-scripting guard."""

        class _LitModel(pl.LightningModule):
            def __init__(self):
                super().__init__()
                self.lin = nn.Linear(4, 2)

            def forward(self, x: torch.Tensor) -> torch.Tensor:
                return self.lin(x)

        with (
            tempfile.TemporaryDirectory() as d,
            mock.patch(
                f"{_TORCH_ONNX_MODULE}._lightning_jit_scripting_guard",
                wraps=torch_onnx._lightning_jit_scripting_guard,
            ) as guard_mock,
        ):
            dest = os.path.join(d, "model.onnx")
            model = _LitModel()
            model.eval()
            export_torch_to_onnx(
                model=model,
                dest_path=dest,
                sample_inputs=(torch.randn(2, 4),),
                input_names=["x"],
                output_names=["y"],
                is_lightning_module=True,
            )
            self.assertTrue(os.path.isfile(dest))
            guard_mock.assert_called_once()

    def test_dynamo_export_retries_legacy_on_onnxscript_error(self):
        """Dynamo export retries legacy on onnxscript error."""
        with tempfile.TemporaryDirectory() as d:
            dest = os.path.join(d, "model.onnx")
            model = _DictModel()
            model.eval()

            def _export_side_effect(*_args, **kwargs):
                if kwargs.get("dynamo") is True:
                    raise RuntimeError("onnxscript required")
                return None

            fake_sig = mock.MagicMock()
            fake_sig.parameters = {"dynamo": 1, "dynamic_shapes": 1}
            with (
                mock.patch(
                    "torch.onnx.export", side_effect=_export_side_effect
                ) as export_mock,
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.onnx_dynamo_exporter_dependencies_available",
                    return_value=True,
                ),
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.onnx_dynamo_dynamic_shapes_for_tuple_arg",
                    return_value=({"0": mock.MagicMock()},),
                ),
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.inspect.signature", return_value=fake_sig
                ),
            ):
                export_torch_to_onnx(
                    model=model,
                    dest_path=dest,
                    sample_inputs=(torch.randn(2, 4), torch.randn(2, 4)),
                    input_names=["a", "b"],
                    output_names=["y"],
                    use_tuple_wrapper=True,
                    input_key_order=["a", "b"],
                )
            self.assertEqual(export_mock.call_count, 2)
            self.assertFalse(export_mock.call_args_list[1].kwargs.get("dynamo"))

    def test_dynamo_export_non_recoverable_error_reraised(self):
        """Dynamo export non recoverable error reraised."""
        with tempfile.TemporaryDirectory() as d:
            dest = os.path.join(d, "model.onnx")
            model = _DictModel()
            model.eval()

            def _export_side_effect(*_args, **kwargs):
                if kwargs.get("dynamo") is True:
                    raise RuntimeError("totally unrelated error")
                return None

            fake_sig = mock.MagicMock()
            fake_sig.parameters = {"dynamo": 1, "dynamic_shapes": 1}
            with (
                mock.patch("torch.onnx.export", side_effect=_export_side_effect),
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.onnx_dynamo_exporter_dependencies_available",
                    return_value=True,
                ),
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.onnx_dynamo_dynamic_shapes_for_tuple_arg",
                    return_value=({"0": mock.MagicMock()},),
                ),
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.inspect.signature", return_value=fake_sig
                ),
                self.assertRaises(RuntimeError),
            ):
                export_torch_to_onnx(
                    model=model,
                    dest_path=dest,
                    sample_inputs=(torch.randn(2, 4), torch.randn(2, 4)),
                    input_names=["a", "b"],
                    output_names=["y"],
                    use_tuple_wrapper=True,
                    input_key_order=["a", "b"],
                )

    def test_dynamo_export_success_with_dynamic_shapes_tuple_wrapper(self):
        """Dynamo export success with dynamic shapes tuple wrapper."""
        with tempfile.TemporaryDirectory() as d:
            dest = os.path.join(d, "model.onnx")
            model = _DictModel()
            model.eval()
            fake_ds = ({"0": mock.MagicMock()},)
            fake_sig = mock.MagicMock()
            fake_sig.parameters = {"dynamo": 1, "dynamic_shapes": 1}
            with (
                mock.patch("torch.onnx.export") as export_mock,
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.onnx_dynamo_exporter_dependencies_available",
                    return_value=True,
                ),
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.onnx_dynamo_dynamic_shapes_for_tuple_arg",
                    return_value=fake_ds,
                ),
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.inspect.signature", return_value=fake_sig
                ),
            ):
                export_torch_to_onnx(
                    model=model,
                    dest_path=dest,
                    sample_inputs=(torch.randn(2, 4), torch.randn(2, 4)),
                    input_names=["a", "b"],
                    output_names=["y"],
                    use_tuple_wrapper=True,
                    input_key_order=["a", "b"],
                )
            export_mock.assert_called_once()
            self.assertTrue(export_mock.call_args.kwargs.get("dynamo"))
            self.assertEqual(
                export_mock.call_args.kwargs.get("dynamic_shapes"), fake_ds
            )

    def test_export_with_schema_shape_forcing_failure_logs_warning(self):
        """Export with schema shape forcing failure logs warning."""
        with tempfile.TemporaryDirectory() as d:
            dest = os.path.join(d, "model.onnx")
            model = _SimpleModel()
            model.eval()
            with (
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.force_onnx_io_shapes_from_schema",
                    side_effect=Exception("bad onnx"),
                ),
                self.assertLogs(_TORCH_ONNX_MODULE, level="WARNING"),
            ):
                export_torch_to_onnx(
                    model=model,
                    dest_path=dest,
                    sample_inputs=(torch.randn(2, 4),),
                    input_names=["x"],
                    output_names=["y"],
                    model_schemas=[_schema()],
                )

    def test_dynamo_with_tuple_wrapper_dynamic_axes_fallback(self):
        """Dynamo available, dynamic_shapes returns None -> use dynamic_axes."""
        with tempfile.TemporaryDirectory() as d:
            dest = os.path.join(d, "model.onnx")
            model = _DictModel()
            model.eval()
            fake_sig = mock.MagicMock()
            fake_sig.parameters = {"dynamo": 1, "dynamic_shapes": 1}
            with (
                mock.patch("torch.onnx.export") as export_mock,
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.onnx_dynamo_exporter_dependencies_available",
                    return_value=True,
                ),
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.onnx_dynamo_dynamic_shapes_for_tuple_arg",
                    return_value=None,
                ),
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.inspect.signature", return_value=fake_sig
                ),
            ):
                export_torch_to_onnx(
                    model=model,
                    dest_path=dest,
                    sample_inputs=(torch.randn(2, 4), torch.randn(2, 4)),
                    input_names=["a", "b"],
                    output_names=["y"],
                    use_tuple_wrapper=True,
                    input_key_order=["a", "b"],
                )
            self.assertTrue(export_mock.call_args.kwargs.get("dynamo"))
            self.assertIsNotNone(export_mock.call_args.kwargs.get("dynamic_axes"))

    def test_dynamo_non_tuple_wrapper_uses_dynamic_axes(self):
        """When dynamo is available but no tuple wrapper, use dynamic_axes."""
        with tempfile.TemporaryDirectory() as d:
            dest = os.path.join(d, "model.onnx")
            model = _SimpleModel()
            model.eval()
            fake_sig = mock.MagicMock()
            fake_sig.parameters = {"dynamo": 1}
            with (
                mock.patch("torch.onnx.export") as export_mock,
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.onnx_dynamo_exporter_dependencies_available",
                    return_value=True,
                ),
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.inspect.signature", return_value=fake_sig
                ),
            ):
                export_torch_to_onnx(
                    model=model,
                    dest_path=dest,
                    sample_inputs=(torch.randn(2, 4),),
                    input_names=["x"],
                    output_names=["y"],
                )
            self.assertTrue(export_mock.call_args.kwargs.get("dynamo"))
            self.assertIsNotNone(export_mock.call_args.kwargs.get("dynamic_axes"))

    def test_legacy_export_sets_opset_version(self):
        """When dynamo is not available, opset_version defaults to OPSET_VERSION=14."""
        with tempfile.TemporaryDirectory() as d:
            dest = os.path.join(d, "model.onnx")
            model = _SimpleModel()
            model.eval()
            fake_sig = mock.MagicMock()
            fake_sig.parameters = {}
            with (
                mock.patch("torch.onnx.export") as export_mock,
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.onnx_dynamo_exporter_dependencies_available",
                    return_value=False,
                ),
                mock.patch(
                    f"{_TORCH_ONNX_MODULE}.inspect.signature", return_value=fake_sig
                ),
            ):
                export_torch_to_onnx(
                    model=model,
                    dest_path=dest,
                    sample_inputs=(torch.randn(2, 4),),
                    input_names=["x"],
                    output_names=["y"],
                )
            self.assertEqual(export_mock.call_args.kwargs.get("opset_version"), 14)
