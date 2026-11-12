"""Tests for ModelInference."""

from __future__ import annotations

import os
import tempfile
import unittest
import unittest.mock
from typing import NamedTuple
from unittest.mock import MagicMock, patch

import numpy as np
import torch

from michelangelo.lib.inference._private.lib.model_inference import (
    _is_custom_python,
)
from michelangelo.lib.inference.lib.model_inference import (
    ModelInference,
    _numpy_to_model_input,
    _torch_model_accepts_dict_input,
)
from michelangelo.lib.inference.lib.tests.fixtures.model import TorchModel
from michelangelo.lib.model_manager.constants import RawModelType, TritonBackendType
from michelangelo.lib.model_manager.packager.custom_triton.custom_triton_packager import (  # noqa: E501
    CustomTritonPackager,
)
from michelangelo.lib.model_manager.schema import (
    DataType,
    ModelSchema,
    ModelSchemaItem,
)

_FIXTURES = "michelangelo.lib.inference.lib.tests.fixtures"


class ModelInferenceTest(unittest.TestCase):
    """Model Inference Test tests."""

    def setUp(self):
        """Build the shared model schema and sample data."""
        self.model_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(
                    name="input",
                    data_type=DataType.INT,
                    shape=[1],
                ),
            ],
            output_schema=[
                ModelSchemaItem(
                    name="response",
                    data_type=DataType.INT,
                    shape=[1],
                ),
            ],
        )
        self.sample_data = [{"input": np.array([1])}]
        self.batch_sample_data = [{"input": np.array([[1], [2]])}]

    def create_model_package(
        self, directory: str, custom_batch_processing: bool = False
    ):
        """Create model package."""
        src_model_path = os.path.join(directory, "model")
        dest_model_path = os.path.join(directory, "model_package")
        os.makedirs(src_model_path)
        os.makedirs(dest_model_path)

        with open(os.path.join(src_model_path, "test_file.txt"), "w") as f:
            f.write("test_content")

        packager = CustomTritonPackager(custom_batch_processing)

        return packager.create_raw_model_package(
            model_path=src_model_path,
            model_class=f"{_FIXTURES}.predict.Predict",
            model_schema=self.model_schema,
            sample_data=self.batch_sample_data
            if custom_batch_processing
            else self.sample_data,
            dest_model_path=dest_model_path,
            include_import_prefixes=[_FIXTURES],
        )

    def test_load_model_with_model_path(self):
        """Load model with model path."""
        with tempfile.TemporaryDirectory() as temp_dir:
            raw_model_package = self.create_model_package(temp_dir)
            model_inference = ModelInference(model_path=raw_model_package)
            model_inference.load_model()
            self.assertIsNotNone(model_inference.model)
            self.assertFalse(model_inference.batch_inference)

    def test_load_model(self):
        """Load model."""
        with tempfile.TemporaryDirectory() as temp_dir:
            raw_model_package = self.create_model_package(temp_dir)
            model_inference = ModelInference(model_path=raw_model_package)
            model_inference.load_model()
            self.assertIsNotNone(model_inference.model)
            self.assertEqual(model_inference.model_type, RawModelType.CUSTOM_PYTHON)
            self.assertFalse(model_inference.batch_inference)

    def test_load_model_with_model_type(self):
        """Load model with model type."""
        with tempfile.TemporaryDirectory() as temp_dir:
            raw_model_package = self.create_model_package(temp_dir)
            model_inference = ModelInference(
                model_path=raw_model_package, model_type=RawModelType.CUSTOM_PYTHON
            )
            model_inference.load_model()
            self.assertIsNotNone(model_inference.model)
            self.assertFalse(model_inference.batch_inference)

    def test_load_model_with_batch_inference(self):
        """Load model with batch inference."""
        with tempfile.TemporaryDirectory() as temp_dir:
            raw_model_package = self.create_model_package(
                temp_dir, custom_batch_processing=True
            )
            model_inference = ModelInference(model_path=raw_model_package)
            model_inference.load_model()
            self.assertIsNotNone(model_inference.model)
            self.assertTrue(model_inference.batch_inference)

    def test_predict(self):
        """Predict."""
        with tempfile.TemporaryDirectory() as temp_dir:
            raw_model_package = self.create_model_package(temp_dir)
            model_inference = ModelInference(model_path=raw_model_package)
            model_inference.load_model()
            prediction = model_inference.execute({"input": np.array([1])})
            self.assertIsNotNone(prediction)
            self.assertIn("response", prediction)

    def test_model_inference_missing_fields(self):
        """Model inference missing fields."""
        with self.assertRaisesRegex(ValueError, "model_path must be provided"):
            ModelInference()

    def test_model_name_is_kept(self):
        """Model name is kept."""
        model_inference = ModelInference(model_path="/tmp/pkg", model_name="my_model")
        self.assertEqual(model_inference.model_name, "my_model")

    def test_predict_with_model_not_loaded_error(self):
        """Predict with model not loaded error."""
        model_inference = ModelInference(model_path="/tmp/pkg")
        with self.assertRaises(ValueError):
            model_inference.execute({"input": np.array([1])})

    # ==================== Torch Model Tests ====================

    def create_torch_deployable_model_package(self, directory: str):
        """Create a torch deployable model package for testing."""
        dest_model_path = os.path.join(directory, "torch_deployable_package")
        os.makedirs(dest_model_path)
        model_version_dir = os.path.join(dest_model_path, "0")
        os.makedirs(model_version_dir)

        # Create and save a TorchScript model
        model = TorchModel(input_dim=1, output_dim=1)
        scripted_model = torch.jit.script(model)
        torch.jit.save(scripted_model, os.path.join(model_version_dir, "model.pt"))

        # Create config.pbtxt
        config_content = """name: "test_torch_model"
backend: "pytorch"
max_batch_size: 256
input : [
  {
    name: "input",
    data_type: TYPE_FP32,
    dims: [ 1 ]
  }
]
output: [
  {
    name: "output",
    data_type: TYPE_FP32,
    dims: [ 1 ]
  }
]
"""
        with open(os.path.join(dest_model_path, "config.pbtxt"), "w") as f:
            f.write(config_content)

        return dest_model_path

    def test_load_torch_deployable_model(self):
        """Test loading a torch deployable model package."""
        with tempfile.TemporaryDirectory() as temp_dir:
            torch_deployable_package = self.create_torch_deployable_model_package(
                temp_dir
            )
            model_inference = ModelInference(model_path=torch_deployable_package)
            model_inference.load_model()

            self.assertIsNotNone(model_inference.model)
            self.assertEqual(model_inference.model_type, RawModelType.TORCH)
            self.assertTrue(model_inference.batch_inference)

    def test_execute_torch_model(self):
        """Test inference execution with a torch deployable model."""
        with tempfile.TemporaryDirectory() as temp_dir:
            torch_deployable_package = self.create_torch_deployable_model_package(
                temp_dir
            )
            model_inference = ModelInference(model_path=torch_deployable_package)
            model_inference.load_model()

            # Single-tensor model: probe detects positional args, not dict
            self.assertFalse(model_inference._torch_model_accepts_dict)

            # Execute inference with batched input
            prediction = model_inference.execute(
                {"input": np.array([[1.0]], dtype=np.float32)}
            )

            self.assertIsNotNone(prediction)
            self.assertIn("output", prediction)
            self.assertIsInstance(prediction["output"], np.ndarray)

    def test_torch_deployable_is_not_custom_python(self):
        """A local torch deployable package is not custom-python, so it routes to the deployable path."""  # noqa: E501
        with tempfile.TemporaryDirectory() as temp_dir:
            torch_deployable_package = self.create_torch_deployable_model_package(
                temp_dir
            )
            model_inference = ModelInference(model_path=torch_deployable_package)

            self.assertFalse(_is_custom_python(model_inference))

    def test_load_unsupported_backend_raises_error(self):
        """Test that a deployable model with an unsupported backend raises a clear error."""  # noqa: E501
        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "unsupported_deployable_package")
            os.makedirs(dest_model_path)

            # Create config.pbtxt with unsupported backend (e.g., tensorrt)
            config_content = """name: "test_model"
backend: "tensorrt"
max_batch_size: 256
"""
            with open(os.path.join(dest_model_path, "config.pbtxt"), "w") as f:
                f.write(config_content)

            model_inference = ModelInference(model_path=dest_model_path)

            with self.assertRaisesRegex(ValueError, "Unsupported deployable backend"):
                model_inference.load_model()

    def test_is_custom_python_local_path_without_markers_returns_false(self):
        """A local path with neither config.pbtxt nor metadata/type.yaml is not custom-python."""  # noqa: E501
        with tempfile.TemporaryDirectory() as temp_dir:
            model_inference = ModelInference(model_path=temp_dir)
            self.assertFalse(_is_custom_python(model_inference))

    def test_load_lightning_deployable_model(self):
        """A lightning model packages as a pytorch-backend deployable, so it loads as torch."""  # noqa: E501
        with tempfile.TemporaryDirectory() as temp_dir:
            torch_deployable_package = self.create_torch_deployable_model_package(
                temp_dir
            )
            model_inference = ModelInference(
                model_path=torch_deployable_package,
                model_type=RawModelType.LIGHTNING,
            )
            model_inference.load_model()

            self.assertIsNotNone(model_inference.model)
            self.assertEqual(model_inference.model_type, RawModelType.TORCH)
            self.assertTrue(model_inference.batch_inference)

    def test_execute_lightning_model(self):
        """Test inference execution with a lightning deployable model."""
        with tempfile.TemporaryDirectory() as temp_dir:
            torch_deployable_package = self.create_torch_deployable_model_package(
                temp_dir
            )
            model_inference = ModelInference(
                model_path=torch_deployable_package,
                model_type=RawModelType.LIGHTNING,
            )
            model_inference.load_model()

            prediction = model_inference.execute(
                {"input": np.array([[1.0]], dtype=np.float32)}
            )

            self.assertIsNotNone(prediction)
            self.assertIn("output", prediction)
            self.assertIsInstance(prediction["output"], np.ndarray)

    def test_execute_torch_model_multiple_inputs(self):
        """Test torch model execution with multiple inputs."""

        # Create a model with 2 inputs
        class MultiInputTorchModel(torch.nn.Module):
            """Multi Input Torch Model tests."""

            def __init__(self):
                super().__init__()
                self.linear1 = torch.nn.Linear(1, 1)
                self.linear2 = torch.nn.Linear(1, 1)

            def forward(self, x1, x2):
                """Forward."""
                return self.linear1(x1) + self.linear2(x2)

        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "torch_deployable_package")
            os.makedirs(dest_model_path)
            model_version_dir = os.path.join(dest_model_path, "0")
            os.makedirs(model_version_dir)

            model = MultiInputTorchModel()
            scripted_model = torch.jit.script(model)
            torch.jit.save(scripted_model, os.path.join(model_version_dir, "model.pt"))

            config_content = """name: "test_torch_model"
backend: "pytorch"
max_batch_size: 256
input : [
  {
    name: "input1",
    data_type: TYPE_FP32,
    dims: [ 1 ]
  },
  {
    name: "input2",
    data_type: TYPE_FP32,
    dims: [ 1 ]
  }
]
output: [
  {
    name: "output",
    data_type: TYPE_FP32,
    dims: [ 1 ]
  }
]
"""
            with open(os.path.join(dest_model_path, "config.pbtxt"), "w") as f:
                f.write(config_content)

            model_inference = ModelInference(model_path=dest_model_path)
            model_inference.load_model()

            prediction = model_inference.execute(
                {
                    "input1": np.array([[1.0]], dtype=np.float32),
                    "input2": np.array([[2.0]], dtype=np.float32),
                }
            )

            self.assertIsNotNone(prediction)
            self.assertIn("output", prediction)

    def test_execute_torch_model_multiple_outputs(self):
        """Test torch model execution with multiple outputs."""

        # Create a model with 2 outputs
        class MultiOutputTorchModel(torch.nn.Module):
            """Multi Output Torch Model tests."""

            def __init__(self):
                super().__init__()
                self.linear = torch.nn.Linear(1, 2)

            def forward(self, x):
                """Forward."""
                out = self.linear(x)
                return out[:, 0:1], out[:, 1:2]

        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "torch_deployable_package")
            os.makedirs(dest_model_path)
            model_version_dir = os.path.join(dest_model_path, "0")
            os.makedirs(model_version_dir)

            model = MultiOutputTorchModel()
            scripted_model = torch.jit.script(model)
            torch.jit.save(scripted_model, os.path.join(model_version_dir, "model.pt"))

            config_content = """name: "test_torch_model"
backend: "pytorch"
max_batch_size: 256
input : [
  {
    name: "input",
    data_type: TYPE_FP32,
    dims: [ 1 ]
  }
]
output: [
  {
    name: "output1",
    data_type: TYPE_FP32,
    dims: [ 1 ]
  },
  {
    name: "output2",
    data_type: TYPE_FP32,
    dims: [ 1 ]
  }
]
"""
            with open(os.path.join(dest_model_path, "config.pbtxt"), "w") as f:
                f.write(config_content)

            model_inference = ModelInference(model_path=dest_model_path)
            model_inference.load_model()

            prediction = model_inference.execute(
                {"input": np.array([[1.0]], dtype=np.float32)}
            )

            self.assertIsNotNone(prediction)
            self.assertIn("output1", prediction)
            self.assertIn("output2", prediction)

    def test_execute_unsupported_model_type(self):
        """Test execute raises error for unsupported model type."""
        model_inference = ModelInference(
            model_path="/tmp/pkg", model_type="unsupported_type"
        )
        # Mock model and model_type to bypass load_model
        model_inference.model = "dummy_model"
        model_inference.model_type = "unsupported_type"

        with self.assertRaisesRegex(
            ValueError, "Execution for 'unsupported_type' is not supported"
        ):
            model_inference.execute({"input": np.array([1])})

    def test_execute_dispatches_through_subclass_overrides(self):
        """execute() must go through self, so subclass overrides win.

        Subclasses can override _execute_torch to feed the model by keyword
        instead of positionally. Calling the module-level function directly
        bypasses the override and transposes typed arguments, since input_schema
        is alphabetical while forward() takes its List[str] arguments first.
        """

        class _OverridingInference(ModelInference):
            """_ Overriding Inference tests."""

            def _execute_torch(self, data):
                return {"who": "subclass_torch"}

            def _execute_onnx(self, data):
                return {"who": "subclass_onnx"}

        for model_type, expected in (
            (RawModelType.TORCH, "subclass_torch"),
            (RawModelType.LIGHTNING, "subclass_torch"),
            (TritonBackendType.ONNX, "subclass_onnx"),
        ):
            with self.subTest(model_type=model_type):
                model_inference = _OverridingInference(
                    model_path="/tmp/pkg", model_type=model_type
                )
                model_inference.model = "dummy_model"
                self.assertEqual(
                    model_inference.execute({"input": np.array([1])})["who"], expected
                )

    def test_load_torch_model_with_string_schema_succeeds(self):
        """Test that loading a torch model with STRING schema does not raise error."""
        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "torch_deployable_package")
            os.makedirs(dest_model_path)
            model_version_dir = os.path.join(dest_model_path, "0")
            os.makedirs(model_version_dir)

            # Create a dummy model
            model = TorchModel()
            scripted_model = torch.jit.script(model)
            torch.jit.save(scripted_model, os.path.join(model_version_dir, "model.pt"))

            # Schema declares STRING type
            config_content = """name: "test_model"
backend: "pytorch"
max_batch_size: 256
input : [
  {
    name: "input",
    data_type: TYPE_STRING,
    dims: [ -1 ]
  }
]
output: [
  {
    name: "output",
    data_type: TYPE_FP32,
    dims: [ 1 ]
  }
]
"""
            with open(os.path.join(dest_model_path, "config.pbtxt"), "w") as f:
                f.write(config_content)

            model_inference = ModelInference(model_path=dest_model_path)

            # Should succeed — string type validation is not done at load time
            model_inference.load_model()
            self.assertIsNotNone(model_inference.model)

    def test_execute_torch_model_with_string_data_converts_to_list(self):
        """Test that executing with string data converts it to a list for the model."""
        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "torch_deployable_package")
            os.makedirs(dest_model_path)
            model_version_dir = os.path.join(dest_model_path, "0")
            os.makedirs(model_version_dir)

            # Create a dummy model
            model = TorchModel()
            scripted_model = torch.jit.script(model)
            torch.jit.save(scripted_model, os.path.join(model_version_dir, "model.pt"))

            # Schema declares FP32
            config_content = """name: "test_model"
backend: "pytorch"
max_batch_size: 256
input : [
  {
    name: "input",
    data_type: TYPE_FP32,
    dims: [ 1 ]
  }
]
output: [
  {
    name: "output",
    data_type: TYPE_FP32,
    dims: [ 1 ]
  }
]
"""
            with open(os.path.join(dest_model_path, "config.pbtxt"), "w") as f:
                f.write(config_content)

            model_inference = ModelInference(model_path=dest_model_path)
            model_inference.load_model()

            # Send string data — should be converted to list and passed to model
            # The model itself may reject it (RuntimeError), but we no
            # longer raise ValueError
            string_data = np.array(["hello", "world"], dtype=object)

            with self.assertRaises(RuntimeError):
                model_inference.execute({"input": string_data})

    def test_torch_model_accepts_dict_input_returns_false_for_positional_model(self):
        """Test _torch_model_accepts_dict_input returns False for a single-tensor (positional) model."""  # noqa: E501
        with tempfile.TemporaryDirectory() as temp_dir:
            torch_deployable_package = self.create_torch_deployable_model_package(
                temp_dir
            )
            model_inference = ModelInference(model_path=torch_deployable_package)
            model_inference.load_model()
            schema = ModelSchema(
                input_schema=[
                    ModelSchemaItem(name="input", data_type=DataType.FLOAT, shape=[1])
                ],
                output_schema=[
                    ModelSchemaItem(name="output", data_type=DataType.FLOAT, shape=[1])
                ],
            )
            self.assertFalse(
                _torch_model_accepts_dict_input(model_inference.model, schema)
            )

    def test_torch_model_accepts_dict_input_returns_false_for_empty_schema(self):
        """Test _torch_model_accepts_dict_input returns False when schema has no input_schema."""  # noqa: E501
        model = TorchModel()
        model.eval()
        empty_schema = ModelSchema(input_schema=[], output_schema=[])
        self.assertFalse(_torch_model_accepts_dict_input(model, empty_schema))

    def test_torch_model_accepts_dict_input_returns_false_for_none_schema(self):
        """Test _torch_model_accepts_dict_input returns False when model_schema is None."""  # noqa: E501
        model = TorchModel()
        model.eval()
        self.assertFalse(_torch_model_accepts_dict_input(model, None))

    def test_execute_torch_model_dict_input(self):
        """Test inference with a TorchScript model that accepts dict[str, Tensor] (e.g. fused model)."""  # noqa: E501

        # Dict-accepting module: forward takes a single dict
        class DictInputModel(torch.nn.Module):
            """Dict Input Model tests."""

            def __init__(self):
                super().__init__()
                self.linear = torch.nn.Linear(1, 1)

            def forward(self, inputs: dict[str, torch.Tensor]) -> torch.Tensor:
                """Forward."""
                x = inputs["input"]
                return self.linear(x)

        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "torch_deployable_package")
            os.makedirs(dest_model_path)
            model_version_dir = os.path.join(dest_model_path, "0")
            os.makedirs(model_version_dir)

            model = DictInputModel()
            sample_input = {"input": torch.zeros(1, 1, dtype=torch.float32)}
            traced = torch.jit.trace(model, sample_input)
            torch.jit.save(traced, os.path.join(model_version_dir, "model.pt"))

            config_content = """name: "test_torch_model"
backend: "pytorch"
max_batch_size: 256
input : [
  {
    name: "input",
    data_type: TYPE_FP32,
    dims: [ 1 ]
  }
]
output: [
  {
    name: "output",
    data_type: TYPE_FP32,
    dims: [ 1 ]
  }
]
"""
            with open(os.path.join(dest_model_path, "config.pbtxt"), "w") as f:
                f.write(config_content)

            model_inference = ModelInference(model_path=dest_model_path)
            model_inference.load_model()

            self.assertTrue(
                model_inference._torch_model_accepts_dict,
                "Dict-input model should be detected as accepting dict",
            )

            prediction = model_inference.execute(
                {"input": np.array([[1.0]], dtype=np.float32)}
            )
            self.assertIsNotNone(prediction)
            self.assertIn("output", prediction)
            self.assertIsInstance(prediction["output"], np.ndarray)

    def test_load_torch_model_sets_device(self):
        """Test that loading torch model sets self.device."""
        with tempfile.TemporaryDirectory() as temp_dir:
            torch_deployable_package = self.create_torch_deployable_model_package(
                temp_dir
            )
            model_inference = ModelInference(model_path=torch_deployable_package)

            # Verify device is None before load
            self.assertIsNone(model_inference.device)

            model_inference.load_model()

            # Verify device is set after load
            self.assertIsNotNone(model_inference.device)
            self.assertIsInstance(model_inference.device, torch.device)
            # Device should be cuda if available, else cpu
            expected_device = "cuda" if torch.cuda.is_available() else "cpu"
            self.assertEqual(model_inference.device.type, expected_device)

    def test_load_torch_model_moves_to_device(self):
        """Test that loading torch model calls model.to(device)."""
        with tempfile.TemporaryDirectory() as temp_dir:
            torch_deployable_package = self.create_torch_deployable_model_package(
                temp_dir
            )
            model_inference = ModelInference(model_path=torch_deployable_package)
            model_inference.load_model()

            # Verify model parameters are on the correct device
            expected_device = "cuda" if torch.cuda.is_available() else "cpu"
            for param in model_inference.model.parameters():
                self.assertEqual(param.device.type, expected_device)

    def test_load_lightning_model_sets_device(self):
        """Test that loading lightning model sets self.device."""
        with tempfile.TemporaryDirectory() as temp_dir:
            torch_deployable_package = self.create_torch_deployable_model_package(
                temp_dir
            )
            model_inference = ModelInference(
                model_path=torch_deployable_package,
                model_type=RawModelType.LIGHTNING,
            )

            # Verify device is None before load
            self.assertIsNone(model_inference.device)

            model_inference.load_model()

            # Verify device is set after load
            self.assertIsNotNone(model_inference.device)
            self.assertIsInstance(model_inference.device, torch.device)
            expected_device = "cuda" if torch.cuda.is_available() else "cpu"
            self.assertEqual(model_inference.device.type, expected_device)

    def test_custom_python_model_device_remains_none(self):
        """Test that custom python models do not set device."""
        with tempfile.TemporaryDirectory() as temp_dir:
            raw_model_package = self.create_model_package(temp_dir)
            model_inference = ModelInference(model_path=raw_model_package)

            # Verify device is None before load
            self.assertIsNone(model_inference.device)

            model_inference.load_model()

            # Verify device remains None for custom python models
            self.assertIsNone(model_inference.device)

    def test_load_torch_model_disables_jit_fusion_on_cuda(self):
        """Test that JIT fusion is disabled when CUDA is available."""
        mock_model = unittest.mock.MagicMock()
        mock_model.to.return_value = mock_model

        with (
            patch(
                "michelangelo.lib.model_manager.serde.model.load_deployable_model",
                return_value=mock_model,
            ),
            patch(
                "michelangelo.lib.model_manager.serde.model.load_deployable_model_schema",
                return_value=unittest.mock.MagicMock(),
            ),
            patch(
                "michelangelo.lib.inference._private.lib.model_inference._torch_model_accepts_dict_input",
                return_value=False,
            ),
            patch("torch.cuda.is_available", return_value=True),
            patch("torch.jit.set_fusion_strategy") as mock_set_fusion,
            tempfile.TemporaryDirectory() as temp_dir,
        ):
            torch_deployable_package = self.create_torch_deployable_model_package(
                temp_dir
            )
            model_inference = ModelInference(model_path=torch_deployable_package)
            model_inference.load_model()

        mock_set_fusion.assert_called_once_with([("STATIC", 0), ("DYNAMIC", 0)])

    def test_load_torch_model_no_jit_fusion_call_on_cpu(self):
        """Test that JIT fusion strategy is not set when loading on CPU."""
        with tempfile.TemporaryDirectory() as temp_dir:
            torch_deployable_package = self.create_torch_deployable_model_package(
                temp_dir
            )
            model_inference = ModelInference(model_path=torch_deployable_package)

            with (
                patch("torch.cuda.is_available", return_value=False),
                patch("torch.jit.set_fusion_strategy") as mock_set_fusion,
            ):
                model_inference.load_model()

            mock_set_fusion.assert_not_called()


class TestNumpyToModelInput(unittest.TestCase):
    """Test the _numpy_to_model_input helper function with device support."""

    def test_numeric_array_no_device(self):
        """Test converting numeric array without device (default CPU)."""
        arr = np.array([[1.0, 2.0], [3.0, 4.0]], dtype=np.float32)
        result = _numpy_to_model_input(arr, device=None)

        self.assertIsInstance(result, torch.Tensor)
        self.assertEqual(result.device.type, "cpu")
        self.assertTrue(torch.equal(result, torch.tensor([[1.0, 2.0], [3.0, 4.0]])))

    def test_numeric_array_cpu_device(self):
        """Test converting numeric array to CPU device."""
        arr = np.array([[1.0, 2.0]], dtype=np.float32)
        device = torch.device("cpu")
        result = _numpy_to_model_input(arr, device=device)

        self.assertIsInstance(result, torch.Tensor)
        self.assertEqual(result.device.type, "cpu")
        self.assertTrue(torch.equal(result, torch.tensor([[1.0, 2.0]])))

    @unittest.skipUnless(torch.cuda.is_available(), "CUDA not available")
    def test_numeric_array_cuda_device(self):
        """Test converting numeric array to CUDA device."""
        arr = np.array([[1.0, 2.0]], dtype=np.float32)
        device = torch.device("cuda")
        result = _numpy_to_model_input(arr, device=device)

        self.assertIsInstance(result, torch.Tensor)
        self.assertEqual(result.device.type, "cuda")
        self.assertTrue(torch.equal(result.cpu(), torch.tensor([[1.0, 2.0]])))

    def test_string_array_unicode_no_device(self):
        """Test string array (Unicode) returns list, ignores device."""
        arr = np.array(["hello", "world"], dtype="U10")
        result = _numpy_to_model_input(arr, device=None)

        self.assertIsInstance(result, list)
        self.assertEqual(result, ["hello", "world"])

    def test_string_array_unicode_with_device(self):
        """Test string array (Unicode) returns list even when device is specified."""
        arr = np.array(["foo", "bar"], dtype="U5")
        device = torch.device("cpu")
        result = _numpy_to_model_input(arr, device=device)

        self.assertIsInstance(result, list)
        self.assertEqual(result, ["foo", "bar"])

    def test_string_array_bytes_no_device(self):
        """Test string array (bytes) returns list."""
        arr = np.array([b"hello", b"world"], dtype="S10")
        result = _numpy_to_model_input(arr, device=None)

        self.assertIsInstance(result, list)
        self.assertEqual(result, [b"hello", b"world"])

    def test_string_array_object_dtype(self):
        """Test object dtype array returns list."""
        arr = np.array(["a", "b", "c"], dtype=object)
        result = _numpy_to_model_input(arr, device=None)

        self.assertIsInstance(result, list)
        self.assertEqual(result, ["a", "b", "c"])

    def test_integer_array_with_device(self):
        """Test integer array conversion with device."""
        arr = np.array([1, 2, 3, 4], dtype=np.int64)
        device = torch.device("cpu")
        result = _numpy_to_model_input(arr, device=device)

        self.assertIsInstance(result, torch.Tensor)
        self.assertEqual(result.device.type, "cpu")
        self.assertTrue(
            torch.equal(result, torch.tensor([1, 2, 3, 4], dtype=torch.int64))
        )

    def test_non_contiguous_array(self):
        """Test non-contiguous array is converted to contiguous before tensor creation."""  # noqa: E501
        arr = np.array([[1, 2, 3], [4, 5, 6]], dtype=np.float32)
        # Create non-contiguous view
        non_contiguous = arr[:, ::2]  # Skip every other column
        self.assertFalse(non_contiguous.flags["C_CONTIGUOUS"])

        result = _numpy_to_model_input(non_contiguous, device=None)

        self.assertIsInstance(result, torch.Tensor)
        expected = torch.tensor([[1.0, 3.0], [4.0, 6.0]], dtype=torch.float32)
        self.assertTrue(torch.equal(result, expected))


class _TwoFieldOutput(NamedTuple):
    """_ Two Field Output tests."""

    logits: torch.Tensor
    probs: torch.Tensor


class TestTorchModelAcceptsDictSchemaInspection(unittest.TestCase):
    """Tests for the new schema-inspection fast path in _torch_model_accepts_dict_input."""  # noqa: E501

    def _make_schema(self) -> ModelSchema:
        return ModelSchema(
            input_schema=[
                ModelSchemaItem(name="input", data_type=DataType.FLOAT, shape=[1])
            ],
            output_schema=[
                ModelSchemaItem(name="output", data_type=DataType.FLOAT, shape=[1])
            ],
        )

    def test_schema_inspection_detects_dict_model(self):
        """Schema inspection returns True for a scripted dict-input model without a probe call."""  # noqa: E501

        class DictModel(torch.nn.Module):
            """Dict Model tests."""

            def forward(self, inputs: dict[str, torch.Tensor]) -> torch.Tensor:
                """Forward."""
                return inputs["input"]

        scripted = torch.jit.script(DictModel())
        schema = self._make_schema()
        # Schema inspection path: "Dict(str, Tensor)" will be in the forward
        # schema string.
        result = _torch_model_accepts_dict_input(scripted, schema)
        self.assertTrue(result)

    def test_schema_inspection_rejects_positional_model(self):
        """Schema inspection returns False for a positional-arg scripted model."""

        class PositionalModel(torch.nn.Module):
            """Positional Model tests."""

            def forward(self, x: torch.Tensor) -> torch.Tensor:
                """Forward."""
                return x * 2

        scripted = torch.jit.script(PositionalModel())
        schema = self._make_schema()
        result = _torch_model_accepts_dict_input(scripted, schema)
        self.assertFalse(result)

    def test_fallback_probe_call_when_schema_inspection_raises(self):
        """When _c._get_method raises, falls through to probe-call fallback."""
        mock_model = unittest.mock.MagicMock()
        mock_model._c._get_method.side_effect = Exception("no method")
        # Probe call: model(probe) should raise TypeError to signal "not dict"
        mock_model.side_effect = TypeError("positional")

        schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[1])
            ],
            output_schema=[],
        )
        result = _torch_model_accepts_dict_input(mock_model, schema)
        self.assertFalse(result)


class TestExecuteTorchNamedTupleOutputPaths(unittest.TestCase):
    """Tests for the NamedTuple-like output branches of _execute_torch."""

    def _write_config(self, dest: str, inputs: list, outputs: list) -> None:
        input_block = "\n".join(
            f'  {{\n    name: "{n}",\n    data_type: TYPE_FP32,\n    dims: [ 1 ]\n  }}'
            for n in inputs
        )
        output_block = "\n".join(
            f'  {{\n    name: "{n}",\n    data_type: TYPE_FP32,\n    dims: [ 1 ]\n  }}'
            for n in outputs
        )
        config = f'name: "test"\nbackend: "pytorch"\nmax_batch_size: 256\ninput : [\n{input_block}\n]\noutput: [\n{output_block}\n]\n'  # noqa: E501
        with open(os.path.join(dest, "config.pbtxt"), "w") as f:
            f.write(config)

    def _save_scripted(self, model: torch.nn.Module, dest: str) -> None:
        model_version_dir = os.path.join(dest, "0")
        os.makedirs(model_version_dir)
        scripted = torch.jit.script(model)
        torch.jit.save(scripted, os.path.join(model_version_dir, "model.pt"))

    # ------------------------------------------------------------------
    # Branch 1: hasattr(outputs, "_fields") is True
    # ------------------------------------------------------------------

    def test_scripted_named_tuple_output_mapped_by_field_name(self):
        """_fields branch: output is mapped by name, not position."""

        class NamedTupleScriptedModel(torch.nn.Module):
            """Named Tuple Scripted Model tests."""

            def forward(self, x: torch.Tensor) -> _TwoFieldOutput:
                """Forward."""
                return _TwoFieldOutput(logits=x * 2, probs=x * 3)

        with tempfile.TemporaryDirectory() as tmp:
            dest = os.path.join(tmp, "pkg")
            os.makedirs(dest)
            self._save_scripted(NamedTupleScriptedModel(), dest)
            # output_schema lists "probs" first, "logits" second —
            # opposite of NamedTuple order.
            # Field-name mapping must still assign the correct tensors.
            self._write_config(dest, ["x"], ["probs", "logits"])

            mi = ModelInference(model_path=dest)
            mi.load_model()

            result = mi.execute({"x": np.array([[1.0]], dtype=np.float32)})

            self.assertIn("logits", result)
            self.assertIn("probs", result)
            np.testing.assert_allclose(result["logits"], [[2.0]], rtol=1e-5)
            np.testing.assert_allclose(result["probs"], [[3.0]], rtol=1e-5)

    # ------------------------------------------------------------------
    # Branch 2: no _fields → positional mapping using output_schema order
    # (assembler guarantees output_schema order matches NamedTuple field order)
    # ------------------------------------------------------------------

    def test_traced_tuple_output_mapped_positionally(self):
        """Plain tuple output (traced model): positional mapping against output_schema."""  # noqa: E501

        class TwoOutputModel(torch.nn.Module):
            """Two Output Model tests."""

            def forward(self, x: torch.Tensor):
                """Forward."""
                return x * 10, x * 20

        with tempfile.TemporaryDirectory() as tmp:
            dest = os.path.join(tmp, "pkg")
            os.makedirs(dest)
            self._save_scripted(TwoOutputModel(), dest)
            self._write_config(dest, ["x"], ["first", "second"])

            mi = ModelInference(model_path=dest)
            mi.load_model()

            result = mi.execute({"x": np.array([[1.0]], dtype=np.float32)})

            np.testing.assert_allclose(result["first"], [[10.0]], rtol=1e-4)
            np.testing.assert_allclose(result["second"], [[20.0]], rtol=1e-4)


def _build_onnx_deployable_package(
    directory: str,
    input_specs: list[tuple[str, str, list[int]]],
    output_specs: list[tuple[str, str, list[int]]],
    onnx_builder,
) -> str:
    """Build a minimal Triton ONNX deployable package layout.

    Args:
        directory: Parent temp dir.
        input_specs: List of (name, triton_dtype, dims).
        output_specs: List of (name, triton_dtype, dims).
        onnx_builder: Callable accepting (onnx_path) that writes the ONNX file.

    Returns:
        Path to the deployable package root.
    """
    dest = os.path.join(directory, "onnx_deployable_package")
    os.makedirs(dest)
    version_dir = os.path.join(dest, "0")
    os.makedirs(version_dir)

    onnx_path = os.path.join(version_dir, "model.onnx")
    onnx_builder(onnx_path)

    input_block = ",\n".join(
        f'  {{\n    name: "{n}",\n    data_type: TYPE_{dtype},\n    dims: [ {", ".join(str(d) for d in dims)} ]\n  }}'  # noqa: E501
        for n, dtype, dims in input_specs
    )
    output_block = ",\n".join(
        f'  {{\n    name: "{n}",\n    data_type: TYPE_{dtype},\n    dims: [ {", ".join(str(d) for d in dims)} ]\n  }}'  # noqa: E501
        for n, dtype, dims in output_specs
    )
    config = f'name: "test_onnx_model"\nbackend: "onnxruntime"\nmax_batch_size: 256\ninput : [\n{input_block}\n]\noutput: [\n{output_block}\n]\n'  # noqa: E501
    with open(os.path.join(dest, "config.pbtxt"), "w") as f:
        f.write(config)
    return dest


def _export_simple_linear_onnx(
    onnx_path: str, in_dim: int = 1, out_dim: int = 1
) -> None:
    """Export a tiny torch.nn.Linear model to ONNX at the given path."""
    model = TorchModel(input_dim=in_dim, output_dim=out_dim)
    model.eval()
    sample = torch.zeros(2, in_dim, dtype=torch.float32)
    with torch.no_grad():
        torch.onnx.export(
            model,
            (sample,),
            onnx_path,
            input_names=["input"],
            output_names=["output"],
            dynamic_axes={"input": {0: "b"}, "output": {0: "b"}},
            do_constant_folding=False,
        )


def _export_multi_io_onnx(onnx_path: str) -> None:
    """Export a 2-input, 2-output ONNX model where outputs are name-swapped."""

    class MultiIO(torch.nn.Module):
        """Multi I O tests."""

        def forward(self, a, b):
            """Forward."""
            return a * 10, b * 20

    model = MultiIO()
    model.eval()
    sample = (
        torch.zeros(2, 1, dtype=torch.float32),
        torch.zeros(2, 1, dtype=torch.float32),
    )
    with torch.no_grad():
        torch.onnx.export(
            model,
            sample,
            onnx_path,
            input_names=["a", "b"],
            output_names=["out_a", "out_b"],
            dynamic_axes={
                "a": {0: "b"},
                "b": {0: "b"},
                "out_a": {0: "b"},
                "out_b": {0: "b"},
            },
            do_constant_folding=False,
        )


def _export_int_input_onnx(onnx_path: str) -> None:
    """Export a model whose input is int64; lets us test dtype casting."""

    class IntIn(torch.nn.Module):
        """Int In tests."""

        def forward(self, x):
            """Forward."""
            return (x * 2).to(torch.float32)

    model = IntIn()
    model.eval()
    sample = torch.zeros(2, 1, dtype=torch.int64)
    with torch.no_grad():
        torch.onnx.export(
            model,
            (sample,),
            onnx_path,
            input_names=["x"],
            output_names=["y"],
            dynamic_axes={"x": {0: "b"}, "y": {0: "b"}},
            do_constant_folding=False,
        )


class OnnxModelInferenceTest(unittest.TestCase):
    """End-to-end tests for the ONNX path in ModelInference."""

    def test_load_onnx_deployable_model(self):
        """Load onnx deployable model."""
        with tempfile.TemporaryDirectory() as temp_dir:
            pkg = _build_onnx_deployable_package(
                temp_dir,
                input_specs=[("input", "FP32", [1])],
                output_specs=[("output", "FP32", [1])],
                onnx_builder=_export_simple_linear_onnx,
            )
            mi = ModelInference(model_path=pkg)
            mi.load_model()

            self.assertEqual(mi.model_type, TritonBackendType.ONNX)
            self.assertIsNotNone(mi.model)
            self.assertTrue(mi.batch_inference)
            # _onnx_input_dtypes populated from session inputs
            self.assertIn("input", mi._onnx_input_dtypes)
            self.assertEqual(mi._onnx_input_dtypes["input"], np.dtype(np.float32))

    def test_execute_onnx_simple_model(self):
        """Execute onnx simple model."""
        with tempfile.TemporaryDirectory() as temp_dir:
            pkg = _build_onnx_deployable_package(
                temp_dir,
                input_specs=[("input", "FP32", [1])],
                output_specs=[("output", "FP32", [1])],
                onnx_builder=_export_simple_linear_onnx,
            )
            mi = ModelInference(model_path=pkg)
            mi.load_model()

            out = mi.execute({"input": np.array([[1.0], [2.0]], dtype=np.float32)})

            self.assertIn("output", out)
            self.assertIsInstance(out["output"], np.ndarray)
            self.assertEqual(out["output"].shape, (2, 1))

    def test_execute_onnx_multiple_inputs_and_outputs(self):
        """Execute onnx multiple inputs and outputs."""
        with tempfile.TemporaryDirectory() as temp_dir:
            pkg = _build_onnx_deployable_package(
                temp_dir,
                input_specs=[("a", "FP32", [1]), ("b", "FP32", [1])],
                output_specs=[("out_a", "FP32", [1]), ("out_b", "FP32", [1])],
                onnx_builder=_export_multi_io_onnx,
            )
            mi = ModelInference(model_path=pkg)
            mi.load_model()

            out = mi.execute(
                {
                    "a": np.array([[1.0], [2.0]], dtype=np.float32),
                    "b": np.array([[3.0], [4.0]], dtype=np.float32),
                }
            )

            np.testing.assert_allclose(
                out["out_a"], np.array([[10.0], [20.0]], dtype=np.float32), rtol=1e-5
            )
            np.testing.assert_allclose(
                out["out_b"], np.array([[60.0], [80.0]], dtype=np.float32), rtol=1e-5
            )

    def test_execute_onnx_casts_input_dtype(self):
        """Inputs whose numpy dtype doesn't match the ONNX graph dtype get cast at execute time."""  # noqa: E501
        with tempfile.TemporaryDirectory() as temp_dir:
            pkg = _build_onnx_deployable_package(
                temp_dir,
                input_specs=[("x", "INT64", [1])],
                output_specs=[("y", "FP32", [1])],
                onnx_builder=_export_int_input_onnx,
            )
            mi = ModelInference(model_path=pkg)
            mi.load_model()

            # Pass int32 even though ONNX expects int64 — the execute path must cast.
            out = mi.execute({"x": np.array([[5], [6]], dtype=np.int32)})

            np.testing.assert_allclose(
                out["y"], np.array([[10.0], [12.0]], dtype=np.float32), rtol=1e-5
            )

    def test_execute_onnx_makes_input_contiguous(self):
        """Non-contiguous numpy inputs are converted to contiguous arrays before being fed to ONNX."""  # noqa: E501
        with tempfile.TemporaryDirectory() as temp_dir:
            pkg = _build_onnx_deployable_package(
                temp_dir,
                input_specs=[("input", "FP32", [1])],
                output_specs=[("output", "FP32", [1])],
                onnx_builder=_export_simple_linear_onnx,
            )
            mi = ModelInference(model_path=pkg)
            mi.load_model()

            base = np.array([[1.0, 9.0], [2.0, 9.0], [3.0, 9.0]], dtype=np.float32)
            non_contig = base[:, 0:1]  # slice → non-contiguous view
            self.assertFalse(non_contig.flags["C_CONTIGUOUS"])

            out = mi.execute({"input": non_contig})

            self.assertEqual(out["output"].shape, (3, 1))

    def test_execute_onnx_copies_readonly_input(self):
        """Read-only inputs are copied so ONNX Runtime / TensorRT get a writeable buffer."""  # noqa: E501
        mi = ModelInference(model_path="/tmp/pkg", model_type=TritonBackendType.ONNX)
        mi.model_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="seq", data_type=DataType.FLOAT, shape=[4])
            ],
            output_schema=[
                ModelSchemaItem(name="out", data_type=DataType.FLOAT, shape=[1])
            ],
        )
        mi._onnx_input_dtypes = {"seq": np.dtype(np.float32)}
        mi.model = MagicMock()
        mi.model.run.return_value = [np.array([[1.0]], dtype=np.float32)]

        arr = np.arange(4, dtype=np.float32).reshape(1, 4)
        arr.setflags(write=False)
        mi.execute({"seq": arr})

        feeds = mi.model.run.call_args[0][1]
        self.assertEqual(feeds["seq"].shape, (1, 4))
        self.assertTrue(feeds["seq"].flags.writeable)

    def test_load_onnx_from_local_path(self):
        """Load onnx from local path."""
        with tempfile.TemporaryDirectory() as temp_dir:
            pkg = _build_onnx_deployable_package(
                temp_dir,
                input_specs=[("input", "FP32", [1])],
                output_specs=[("output", "FP32", [1])],
                onnx_builder=_export_simple_linear_onnx,
            )
            mi = ModelInference(model_path=pkg)
            mi.load_model()
            self.assertEqual(mi.model_type, TritonBackendType.ONNX)

    def test_load_local_onnx_overrides_explicit_model_type(self):
        """The package decides the backend, not the caller-supplied model_type.

        A LIGHTNING model_type is ignored for an ONNX package. The raw
        framework can differ from the deployable backend.
        """
        with tempfile.TemporaryDirectory() as temp_dir:
            pkg = _build_onnx_deployable_package(
                temp_dir,
                input_specs=[("input", "FP32", [1])],
                output_specs=[("output", "FP32", [1])],
                onnx_builder=_export_simple_linear_onnx,
            )
            mi = ModelInference(model_path=pkg, model_type=RawModelType.LIGHTNING)
            mi.load_model()
            self.assertEqual(mi.model_type, TritonBackendType.ONNX)

    def test_load_onnx_missing_file_raises(self):
        """An ONNX package without 0/model.onnx raises a clear FileNotFoundError."""
        with tempfile.TemporaryDirectory() as temp_dir:
            dest = os.path.join(temp_dir, "broken_onnx_pkg")
            os.makedirs(dest)
            # config.pbtxt present, but no 0/model.onnx
            with open(os.path.join(dest, "config.pbtxt"), "w") as f:
                f.write('name: "m"\nbackend: "onnxruntime"\nmax_batch_size: 256\n')

            mi = ModelInference(model_path=dest, model_type=TritonBackendType.ONNX)
            with self.assertRaisesRegex(FileNotFoundError, "ONNX model file not found"):
                mi.load_model()


def _packaged_schema(inputs, outputs=("y",)):
    return ModelSchema(
        input_schema=[
            ModelSchemaItem(name=n, data_type=DataType.FLOAT, shape=[1]) for n in inputs
        ],
        output_schema=[
            ModelSchemaItem(name=n, data_type=DataType.FLOAT, shape=[1])
            for n in outputs
        ],
    )


class PackagedModelInferenceTest(unittest.TestCase):
    """End-to-end tests on packages built by the torch packager."""

    def _run(self, model, backend=None, inputs=("a", "b")):
        from pathlib import Path

        from michelangelo.lib.inference.lib.tests.fixtures.packages import (
            torch_deployable,
        )

        with tempfile.TemporaryDirectory() as tmp:
            package = torch_deployable(
                Path(tmp), model, _packaged_schema(inputs), backend
            )
            mi = ModelInference(model_path=package)
            mi.load_model()
            data = {
                "a": np.array([[1.0], [2.0]], dtype=np.float32),
                "b": np.array([[10.0], [20.0]], dtype=np.float32),
            }
            return mi, mi.execute(data)

    def test_torchscript_positional_inputs_in_schema_order(self):
        """Torchscript positional inputs in schema order."""
        from michelangelo.lib.inference.lib.tests.fixtures.packages import (
            PositionalModel,
        )

        mi, out = self._run(PositionalModel())
        self.assertEqual(mi.model_type, RawModelType.TORCH)
        self.assertFalse(mi._torch_model_accepts_dict)
        np.testing.assert_allclose(out["y"], [[12.0], [24.0]])

    def test_torchscript_dict_input(self):
        """Torchscript dict input."""
        from michelangelo.lib.inference.lib.tests.fixtures.packages import DictModel

        mi, out = self._run(DictModel())
        self.assertTrue(mi._torch_model_accepts_dict)
        np.testing.assert_allclose(out["y"], [[12.0], [24.0]])

    def test_onnx_package(self):
        """Onnx package."""
        from michelangelo.lib.inference.lib.tests.fixtures.packages import (
            PositionalModel,
        )

        mi, out = self._run(PositionalModel(), backend="onnxruntime")
        self.assertEqual(mi.model_type, TritonBackendType.ONNX)
        np.testing.assert_allclose(out["y"], [[12.0], [24.0]])
