"""Tests for infer_deployable_model_package_type."""

from __future__ import annotations

import os
import tempfile
from unittest import TestCase
from unittest.mock import patch

from torch import nn

from michelangelo.lib.model_manager._private.utils.model_utils import (
    infer_deployable_model_package_type,
)
from michelangelo.lib.model_manager.constants import RawModelType, TritonBackendType
from michelangelo.lib.model_manager.interface.custom_model import Model

_GET_MODULE_ATTR = (
    "michelangelo.lib.model_manager._private.utils.model_utils."
    "model_package_type.get_module_attr"
)


class InferDeployableModelPackageTypeTest(TestCase):
    """Tests for inferring the training framework from a deployable package."""

    def _create_config_pbtxt(self, directory: str, backend: str) -> None:
        with open(os.path.join(directory, "config.pbtxt"), "w") as f:
            f.write(f'name: "model"\nbackend: "{backend}"\n')

    def _create_model_class_txt(self, directory: str, model_class: str) -> None:
        os.makedirs(os.path.join(directory, "0"), exist_ok=True)
        with open(os.path.join(directory, "0", "model_class.txt"), "w") as f:
            f.write(model_class)

    def test_pytorch_backend_returns_torch(self):
        """Test that a pytorch backend is a torch model."""
        with tempfile.TemporaryDirectory() as temp_dir:
            self._create_config_pbtxt(temp_dir, "pytorch")
            self.assertEqual(
                infer_deployable_model_package_type(temp_dir), RawModelType.TORCH
            )

    @patch(_GET_MODULE_ATTR)
    def test_python_backend_with_custom_model_returns_custom_python(
        self, mock_get_module_attr
    ):
        """Test that a python backend with a custom Model is custom-python."""

        class MyCustomModel(Model):
            def save(self, _path):
                pass

            @classmethod
            def load(cls, _path):
                return cls()

            def predict(self, _inputs):
                return {}

        mock_get_module_attr.return_value = MyCustomModel

        with tempfile.TemporaryDirectory() as temp_dir:
            self._create_config_pbtxt(temp_dir, "python")
            self._create_model_class_txt(temp_dir, "my_module.MyCustomModel")
            self.assertEqual(
                infer_deployable_model_package_type(temp_dir),
                RawModelType.CUSTOM_PYTHON,
            )

    @patch(_GET_MODULE_ATTR)
    def test_python_backend_with_torch_module_returns_torch(self, mock_get_module_attr):
        """Test that a python backend with an nn.Module is a torch model."""

        class MyTorchModel(nn.Module):
            def forward(self, x):
                return x

        mock_get_module_attr.return_value = MyTorchModel

        with tempfile.TemporaryDirectory() as temp_dir:
            self._create_config_pbtxt(temp_dir, "python")
            self._create_model_class_txt(temp_dir, "my_module.MyTorchModel")
            self.assertEqual(
                infer_deployable_model_package_type(temp_dir), RawModelType.TORCH
            )

    @patch(_GET_MODULE_ATTR)
    def test_python_backend_with_other_class_returns_none(self, mock_get_module_attr):
        """Test that a python backend class of neither kind is unrecognized."""
        mock_get_module_attr.return_value = dict

        with tempfile.TemporaryDirectory() as temp_dir:
            self._create_config_pbtxt(temp_dir, "python")
            self._create_model_class_txt(temp_dir, "builtins.dict")
            self.assertIsNone(infer_deployable_model_package_type(temp_dir))

    def test_no_config_pbtxt_returns_none(self):
        """Test that a directory without config.pbtxt is unrecognized."""
        with tempfile.TemporaryDirectory() as temp_dir:
            self.assertIsNone(infer_deployable_model_package_type(temp_dir))

    def test_python_backend_no_model_class_txt_returns_none(self):
        """Test that a python backend without model_class.txt is unrecognized."""
        with tempfile.TemporaryDirectory() as temp_dir:
            self._create_config_pbtxt(temp_dir, "python")
            self.assertIsNone(infer_deployable_model_package_type(temp_dir))

    def test_python_backend_empty_model_class_txt_returns_none(self):
        """Test that an empty model_class.txt is unrecognized."""
        with tempfile.TemporaryDirectory() as temp_dir:
            self._create_config_pbtxt(temp_dir, "python")
            self._create_model_class_txt(temp_dir, "")
            self.assertIsNone(infer_deployable_model_package_type(temp_dir))

    @patch(_GET_MODULE_ATTR, side_effect=ImportError("module not found"))
    def test_python_backend_unresolvable_class_returns_none(self, _mock):
        """Test that a model class that cannot be imported is unrecognized."""
        with tempfile.TemporaryDirectory() as temp_dir:
            self._create_config_pbtxt(temp_dir, "python")
            self._create_model_class_txt(temp_dir, "nonexistent.module.MyModel")
            self.assertIsNone(infer_deployable_model_package_type(temp_dir))

    def test_unknown_backend_returns_none(self):
        """Test that a backend that is not supported is unrecognized."""
        with tempfile.TemporaryDirectory() as temp_dir:
            self._create_config_pbtxt(temp_dir, "tensorrt")
            self.assertIsNone(infer_deployable_model_package_type(temp_dir))

    def test_onnxruntime_backend_returns_onnx(self):
        """Test that an onnxruntime backend returns the Triton backend string."""
        with tempfile.TemporaryDirectory() as temp_dir:
            self._create_config_pbtxt(temp_dir, "onnxruntime")
            self.assertEqual(
                infer_deployable_model_package_type(temp_dir), TritonBackendType.ONNX
            )
