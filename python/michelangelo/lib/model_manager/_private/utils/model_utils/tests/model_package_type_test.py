"""Tests for infer_deployable_model_package_type."""

from __future__ import annotations

import os
import tempfile
import uuid
from unittest import TestCase

from michelangelo.lib.model_manager._private.utils.model_utils import (
    infer_deployable_model_package_type,
)
from michelangelo.lib.model_manager.constants import RawModelType, TritonBackendType

_CUSTOM_MODEL = """
from michelangelo.lib.model_manager.interface.custom_model import Model


class Packaged(Model):
    def save(self, path):
        pass

    @classmethod
    def load(cls, path):
        return cls()

    def predict(self, inputs):
        return inputs
"""

_TORCH_MODEL = """
from torch import nn


class Packaged(nn.Module):
    def forward(self, x):
        return x
"""


class InferDeployableModelPackageTypeTest(TestCase):
    """Tests for inferring the training framework from a deployable package."""

    def _create_config_pbtxt(self, directory: str, backend: str) -> None:
        with open(os.path.join(directory, "config.pbtxt"), "w") as f:
            f.write(f'name: "model"\nbackend: "{backend}"\n')

    def _create_model_class_txt(self, directory: str, model_class: str) -> None:
        os.makedirs(os.path.join(directory, "0"), exist_ok=True)
        with open(os.path.join(directory, "0", "model_class.txt"), "w") as f:
            f.write(model_class)

    def _python_package(self, directory: str, source: str, attr="Packaged") -> None:
        """A python-backend package whose model class ships only in ``0/``."""
        module = f"packaged_{uuid.uuid4().hex}"
        self._create_config_pbtxt(directory, "python")
        self._create_model_class_txt(directory, f"{module}.{attr}")
        with open(os.path.join(directory, "0", f"{module}.py"), "w") as f:
            f.write(source)

    def test_pytorch_backend_returns_torch(self):
        """Test that a pytorch backend is a torch model."""
        with tempfile.TemporaryDirectory() as temp_dir:
            self._create_config_pbtxt(temp_dir, "pytorch")
            self.assertEqual(
                infer_deployable_model_package_type(temp_dir), RawModelType.TORCH
            )

    def test_python_backend_with_packaged_custom_model_returns_custom_python(self):
        """A custom Model class shipped inside the package is custom-python."""
        with tempfile.TemporaryDirectory() as temp_dir:
            self._python_package(temp_dir, _CUSTOM_MODEL)
            self.assertEqual(
                infer_deployable_model_package_type(temp_dir),
                RawModelType.CUSTOM_PYTHON,
            )

    def test_python_backend_with_packaged_torch_module_returns_torch(self):
        """An nn.Module class shipped inside the package is a torch model."""
        with tempfile.TemporaryDirectory() as temp_dir:
            self._python_package(temp_dir, _TORCH_MODEL)
            self.assertEqual(
                infer_deployable_model_package_type(temp_dir), RawModelType.TORCH
            )

    def test_python_backend_with_other_class_returns_none(self):
        """Test that a python backend class of neither kind is unrecognized."""
        with tempfile.TemporaryDirectory() as temp_dir:
            self._create_config_pbtxt(temp_dir, "python")
            self._create_model_class_txt(temp_dir, "builtins.dict")
            self.assertIsNone(infer_deployable_model_package_type(temp_dir))

    def test_python_backend_naming_a_function_returns_none(self):
        """A model_class.txt that names a function, not a class, is unrecognized."""
        with tempfile.TemporaryDirectory() as temp_dir:
            self._python_package(temp_dir, "def Packaged():\n    pass\n")
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

    def test_python_backend_unresolvable_class_returns_none(self):
        """Test that a model class that cannot be imported is unrecognized."""
        with tempfile.TemporaryDirectory() as temp_dir:
            self._create_config_pbtxt(temp_dir, "python")
            self._create_model_class_txt(
                temp_dir, f"nonexistent_{uuid.uuid4().hex}.MyModel"
            )
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
