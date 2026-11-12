"""Tests for the private model loading and execution helpers."""

from __future__ import annotations

import os
import tempfile
import unittest
from types import SimpleNamespace
from typing import NamedTuple
from unittest.mock import MagicMock, patch

import numpy as np
import torch

from michelangelo.lib.inference._private.lib.model_inference import (
    _execute_torch,
    _is_custom_python,
    _load_custom_python_model,
    _load_deployable_model,
    _load_torch_deployable_model,
    _numpy_to_model_input,
    _torch_model_accepts_dict_input,
)
from michelangelo.lib.model_manager.constants import RawModelType, TritonBackendType
from michelangelo.lib.model_manager.schema import (
    DataType,
    ModelSchema,
    ModelSchemaItem,
)

_PRIV = "michelangelo.lib.inference._private.lib.model_inference"
_SERDE = "michelangelo.lib.model_manager.serde.model"


def _mi(**kwargs):
    """Build a minimal ModelInference-like namespace for testing private functions."""
    defaults = {
        "model_type": None,
        "model_path": "/p",
        "model_name": None,
        "model": None,
        "model_schema": None,
        "batch_inference": False,
        "_torch_model_accepts_dict": False,
        "device": None,
        "_onnx_input_dtypes": {},
    }
    defaults.update(kwargs)
    return SimpleNamespace(**defaults)


class NumpyToModelInputTest(unittest.TestCase):
    """Numpy To Model Input Test tests."""

    def test_numeric_array_returns_tensor(self):
        """Numeric array returns tensor."""
        arr = np.array([1.0, 2.0], dtype=np.float32)
        result = _numpy_to_model_input(arr)
        self.assertIsInstance(result, torch.Tensor)

    def test_unicode_array_returns_list(self):
        """Unicode array returns list."""
        arr = np.array(["a", "b"], dtype="U5")
        self.assertIsInstance(_numpy_to_model_input(arr), list)

    def test_bytes_array_returns_list(self):
        """Bytes array returns list."""
        arr = np.array([b"x"], dtype="S5")
        self.assertIsInstance(_numpy_to_model_input(arr), list)

    def test_object_array_returns_list(self):
        """Object array returns list."""
        arr = np.array(["x"], dtype=object)
        self.assertIsInstance(_numpy_to_model_input(arr), list)

    def test_with_device(self):
        """With device."""
        arr = np.array([1.0], dtype=np.float32)
        result = _numpy_to_model_input(arr, device=torch.device("cpu"))
        self.assertEqual(result.device.type, "cpu")

    def test_non_contiguous_array(self):
        """Non contiguous array."""
        arr = np.array([[1, 2, 3], [4, 5, 6]], dtype=np.float32)[:, ::2]
        self.assertFalse(arr.flags["C_CONTIGUOUS"])
        result = _numpy_to_model_input(arr)
        self.assertIsInstance(result, torch.Tensor)


class TorchModelAcceptsDictInputTest(unittest.TestCase):
    """Torch Model Accepts Dict Input Test tests."""

    def test_scripted_dict_model_detected(self):
        """Scripted dict model detected."""

        class DictModel(torch.nn.Module):
            """Dict Model tests."""

            def forward(self, inputs: dict[str, torch.Tensor]) -> torch.Tensor:
                """Forward."""
                return inputs["x"]

        scripted = torch.jit.script(DictModel())
        schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1])
            ],
            output_schema=[],
        )
        self.assertTrue(_torch_model_accepts_dict_input(scripted, schema))

    def test_positional_model_returns_false(self):
        """Positional model returns false."""

        class PosModel(torch.nn.Module):
            """Pos Model tests."""

            def forward(self, x: torch.Tensor) -> torch.Tensor:
                """Forward."""
                return x

        scripted = torch.jit.script(PosModel())
        schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1])
            ],
            output_schema=[],
        )
        self.assertFalse(_torch_model_accepts_dict_input(scripted, schema))

    def test_none_schema_returns_false(self):
        """None schema returns false."""
        model = MagicMock()
        model._c._get_method.side_effect = Exception("no method")
        model.side_effect = TypeError("positional")
        self.assertFalse(_torch_model_accepts_dict_input(model, None))

    def test_empty_schema_returns_false(self):
        """Empty schema returns false."""
        model = MagicMock()
        model._c._get_method.side_effect = Exception("no method")
        model.side_effect = TypeError("positional")
        self.assertFalse(
            _torch_model_accepts_dict_input(
                model, ModelSchema(input_schema=[], output_schema=[])
            )
        )

    def test_fallback_probe_returns_true_when_probe_succeeds(self):
        """Fallback probe call succeeds → model accepts dict input."""
        model = MagicMock()
        model._c._get_method.side_effect = Exception("no method")
        model.return_value = torch.tensor([[1.0]])  # probe call succeeds
        schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1])
            ],
            output_schema=[],
        )
        self.assertTrue(_torch_model_accepts_dict_input(model, schema))

    def test_fallback_probe_runtime_error_returns_false(self):
        """RuntimeError from probe call → not dict input."""
        model = MagicMock()
        model._c._get_method.side_effect = Exception("no method")
        model.side_effect = RuntimeError("expected Tensor, got dict")
        schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1])
            ],
            output_schema=[],
        )
        self.assertFalse(_torch_model_accepts_dict_input(model, schema))

    def test_fallback_probe_with_no_shape(self):
        """Schema item with no shape defaults to [1] for probe."""
        model = MagicMock()
        model._c._get_method.side_effect = Exception("no method")
        model.side_effect = TypeError("positional")
        schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=None)
            ],
            output_schema=[],
        )
        self.assertFalse(_torch_model_accepts_dict_input(model, schema))

    def test_fallback_probe_with_string_type(self):
        """String-type schema item gets a list probe value."""
        model = MagicMock()
        model._c._get_method.side_effect = Exception("no method")
        model.return_value = torch.tensor([[1.0]])
        schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="text", data_type=DataType.STRING, shape=[1])
            ],
            output_schema=[],
        )
        self.assertTrue(_torch_model_accepts_dict_input(model, schema))


class IsCustomPythonTest(unittest.TestCase):
    """Is Custom Python Test tests."""

    def test_explicit_custom_python_type(self):
        """Explicit custom python type."""
        mi = _mi(model_type=RawModelType.CUSTOM_PYTHON)
        self.assertTrue(_is_custom_python(mi))

    def test_explicit_torch_type(self):
        """Explicit torch type."""
        mi = _mi(model_type=RawModelType.TORCH)
        self.assertFalse(_is_custom_python(mi))

    def test_local_path_with_config_pbtxt(self):
        """Local path with config pbtxt."""
        with tempfile.TemporaryDirectory() as d:
            open(os.path.join(d, "config.pbtxt"), "w").close()
            mi = _mi(model_path=d)
            self.assertFalse(_is_custom_python(mi))

    def test_local_path_without_markers(self):
        """Local path without markers."""
        with tempfile.TemporaryDirectory() as d:
            mi = _mi(model_path=d)
            self.assertFalse(_is_custom_python(mi))

    def test_local_path_with_type_yaml_custom_python(self):
        """Local path with type yaml custom python."""
        with tempfile.TemporaryDirectory() as d:
            meta = os.path.join(d, "metadata")
            os.makedirs(meta)
            open(os.path.join(meta, "type.yaml"), "w").close()
            with patch(
                f"{_SERDE}.get_raw_model_type", return_value=RawModelType.CUSTOM_PYTHON
            ):
                mi = _mi(model_path=d)
                self.assertTrue(_is_custom_python(mi))

    def test_local_path_with_type_yaml_torch(self):
        """Local path with type yaml torch."""
        with tempfile.TemporaryDirectory() as d:
            meta = os.path.join(d, "metadata")
            os.makedirs(meta)
            open(os.path.join(meta, "type.yaml"), "w").close()
            with patch(f"{_SERDE}.get_raw_model_type", return_value=RawModelType.TORCH):
                mi = _mi(model_path=d)
                self.assertFalse(_is_custom_python(mi))


class LoadCustomPythonModelTest(unittest.TestCase):
    """Load Custom Python Model Test tests."""

    @patch(f"{_SERDE}.load_model_schema", return_value=MagicMock())
    @patch(f"{_SERDE}.load_raw_model", return_value=MagicMock())
    @patch(f"{_PRIV}.inject_model_defs")
    def test_existing_path_no_type_yaml(self, mock_inject, _mock_load, _mock_schema):
        """Existing path no type yaml."""
        with tempfile.TemporaryDirectory() as d:
            mi = _mi(model_path=d)
            _load_custom_python_model(mi)
            mock_inject.assert_called_once_with(d)
            self.assertFalse(mi.batch_inference)

    @patch(f"{_SERDE}.load_model_schema", return_value=MagicMock())
    @patch(f"{_SERDE}.load_raw_model", return_value=MagicMock())
    @patch(f"{_PRIV}.inject_model_defs")
    def test_reads_batch_inference_from_type_yaml(
        self, _mock_inject, _mock_load, _mock_schema
    ):
        """Reads batch inference from type yaml."""
        with tempfile.TemporaryDirectory() as d:
            meta = os.path.join(d, "metadata")
            os.makedirs(meta)
            with open(os.path.join(meta, "type.yaml"), "w") as f:
                f.write("batch_inference: true\n")
            mi = _mi(model_path=d)
            _load_custom_python_model(mi)
            self.assertTrue(mi.batch_inference)


class LoadDeployableModelTest(unittest.TestCase):
    """Load Deployable Model Test tests."""

    def test_onnx_routes_to_onnx_loader(self):
        """Onnx routes to onnx loader."""
        mi = _mi(model_path="/p")
        with (
            patch(f"{_PRIV}._load_onnx_deployable_model") as mock_onnx,
            patch(f"{_PRIV}._load_torch_deployable_model") as mock_torch,
            patch(
                f"{_PRIV}.infer_deployable_model_package_type",
                return_value=TritonBackendType.ONNX,
            ),
        ):
            _load_deployable_model(mi)
        mock_onnx.assert_called_once_with(mi)
        mock_torch.assert_not_called()

    def test_torch_routes_to_torch_loader(self):
        """Torch routes to torch loader."""
        mi = _mi(model_path="/p")
        with (
            patch(f"{_PRIV}._load_onnx_deployable_model") as mock_onnx,
            patch(f"{_PRIV}._load_torch_deployable_model") as mock_torch,
            patch(
                f"{_PRIV}.infer_deployable_model_package_type",
                return_value=RawModelType.TORCH,
            ),
        ):
            _load_deployable_model(mi)
        mock_torch.assert_called_once_with(mi)
        mock_onnx.assert_not_called()

    def test_lightning_routes_to_torch_loader(self):
        """Lightning routes to torch loader."""
        mi = _mi(model_path="/p")
        with (
            patch(f"{_PRIV}._load_torch_deployable_model") as mock_torch,
            patch(
                f"{_PRIV}.infer_deployable_model_package_type",
                return_value=RawModelType.LIGHTNING,
            ),
        ):
            _load_deployable_model(mi)
        mock_torch.assert_called_once_with(mi)

    def test_unsupported_backend_raises(self):
        """Unsupported backend raises."""
        mi = _mi(model_path="/p")
        with (
            patch(
                f"{_PRIV}.infer_deployable_model_package_type",
                return_value="unsupported",
            ),
            self.assertRaisesRegex(ValueError, "Unsupported deployable backend"),
        ):
            _load_deployable_model(mi)


class LoadTorchDeployableModelTest(unittest.TestCase):
    """Load Torch Deployable Model Test tests."""

    @patch(f"{_SERDE}.load_deployable_model_schema")
    @patch(f"{_SERDE}.load_deployable_model")
    @patch(f"{_PRIV}._torch_model_accepts_dict_input", return_value=False)
    @patch("torch.cuda.is_available", return_value=False)
    def test_cpu_path(self, mock_cuda, mock_dict, mock_load, mock_schema):
        """Cpu path."""
        mock_model = MagicMock()
        mock_model.to.return_value = mock_model
        mock_load.return_value = mock_model
        mock_schema.return_value = MagicMock()
        mi = _mi(model_path="/p")
        _load_torch_deployable_model(mi)
        self.assertTrue(mi.batch_inference)
        self.assertEqual(mi.device.type, "cpu")

    @patch("torch.jit.set_fusion_strategy")
    @patch(f"{_SERDE}.load_deployable_model_schema")
    @patch(f"{_SERDE}.load_deployable_model")
    @patch(f"{_PRIV}._torch_model_accepts_dict_input", return_value=False)
    @patch("torch.cuda.is_available", return_value=True)
    def test_cuda_disables_fusion(
        self, mock_cuda, mock_dict, mock_load, mock_schema, mock_fusion
    ):
        """Cuda disables fusion."""
        mock_model = MagicMock()
        mock_model.to.return_value = mock_model
        mock_load.return_value = mock_model
        mock_schema.return_value = MagicMock()
        mi = _mi(model_path="/p")
        _load_torch_deployable_model(mi)
        mock_fusion.assert_called_once_with([("STATIC", 0), ("DYNAMIC", 0)])


class ExecuteTorchTest(unittest.TestCase):
    """Execute Torch Test tests."""

    def _schema(self, inputs, outputs):
        return ModelSchema(
            input_schema=[
                ModelSchemaItem(name=n, data_type=DataType.FLOAT, shape=[1])
                for n in inputs
            ],
            output_schema=[
                ModelSchemaItem(name=n, data_type=DataType.FLOAT, shape=[1])
                for n in outputs
            ],
        )

    def test_single_tensor_output(self):
        """Single tensor output."""
        mock_model = MagicMock(return_value=torch.tensor([[1.0]]))
        mi = _mi(
            model_schema=self._schema(["x"], ["out"]),
            model=mock_model,
            device=torch.device("cpu"),
            _torch_model_accepts_dict=False,
        )
        result = _execute_torch(mi, {"x": np.array([[1.0]], dtype=np.float32)})
        self.assertIn("out", result)

    def test_dict_input_model(self):
        """Dict input model."""
        mock_model = MagicMock(return_value=torch.tensor([[2.0]]))
        mi = _mi(
            model_schema=self._schema(["x"], ["out"]),
            model=mock_model,
            device=torch.device("cpu"),
            _torch_model_accepts_dict=True,
        )
        result = _execute_torch(mi, {"x": np.array([[1.0]], dtype=np.float32)})
        self.assertIn("out", result)

    def test_multiple_inputs(self):
        """Multiple inputs."""
        mock_model = MagicMock(return_value=torch.tensor([[3.0]]))
        mi = _mi(
            model_schema=self._schema(["a", "b"], ["out"]),
            model=mock_model,
            device=torch.device("cpu"),
            _torch_model_accepts_dict=False,
        )
        result = _execute_torch(
            mi,
            {
                "a": np.array([[1.0]], dtype=np.float32),
                "b": np.array([[2.0]], dtype=np.float32),
            },
        )
        self.assertIn("out", result)

    def test_named_tuple_output(self):
        """Named tuple output."""

        class Out(NamedTuple):
            """Out tests."""

            logits: torch.Tensor

        mock_model = MagicMock(return_value=Out(logits=torch.tensor([[1.0]])))
        mi = _mi(
            model_schema=ModelSchema(
                input_schema=[
                    ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1])
                ],
                output_schema=[
                    ModelSchemaItem(name="logits", data_type=DataType.FLOAT, shape=[1])
                ],
            ),
            model=mock_model,
            device=torch.device("cpu"),
            _torch_model_accepts_dict=False,
        )
        result = _execute_torch(mi, {"x": np.array([[1.0]], dtype=np.float32)})
        self.assertIn("logits", result)
