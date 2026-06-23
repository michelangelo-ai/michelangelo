"""Tests for torch_triton validation helpers."""

import os
import tempfile
from unittest import TestCase

import torch

from michelangelo.lib.model_manager._private.packager.torch_triton.tests.fixtures.simple_model import (  # noqa: E501
    save_scripted_model,
    save_state_dict,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.validation import (
    _has_batch_dimension,
    validate_deployable_onnx_file,
    validate_model_class,
    validate_state_dict_file,
    validate_torchscript_file,
)

_MODEL_CLASS = (
    "michelangelo.lib.model_manager._private.packager.torch_triton."
    "tests.fixtures.simple_model.SimpleModel"
)
_NOT_A_MODULE_CLASS = (
    "michelangelo.lib.model_manager._private.packager.torch_triton."
    "tests.fixtures.simple_model.NotAModule"
)


class ValidateStateDictFileTest(TestCase):
    """Test cases for ``validate_state_dict_file``."""

    def test_valid_state_dict(self):
        """A genuine state_dict file validates successfully."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.pt")
            save_state_dict(path)

            is_valid, error = validate_state_dict_file(path)

            self.assertTrue(is_valid)
            self.assertIsNone(error)

    def test_torchscript_is_not_a_state_dict(self):
        """A TorchScript file is rejected as a state_dict."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.pt")
            save_scripted_model(path)

            is_valid, error = validate_state_dict_file(path)

            self.assertFalse(is_valid)
            self.assertIsNotNone(error)

    def test_empty_file_is_invalid(self):
        """An empty file is rejected."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.pt")
            open(path, "w").close()

            is_valid, error = validate_state_dict_file(path)

            self.assertFalse(is_valid)
            self.assertIsInstance(error, ValueError)

    def test_directory_is_invalid(self):
        """A directory path is rejected."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.pt")
            os.makedirs(path)

            is_valid, error = validate_state_dict_file(path)

            self.assertFalse(is_valid)
            self.assertIsInstance(error, ValueError)

    def test_missing_file_is_invalid(self):
        """A nonexistent path is rejected with FileNotFoundError."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "missing.pt")

            is_valid, error = validate_state_dict_file(path)

            self.assertFalse(is_valid)
            self.assertIsInstance(error, FileNotFoundError)


class ValidateTorchScriptFileTest(TestCase):
    """Test cases for ``validate_torchscript_file``."""

    def test_valid_torchscript(self):
        """A scripted model file validates successfully."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.pt")
            save_scripted_model(path)

            is_valid, error = validate_torchscript_file(path)

            self.assertTrue(is_valid)
            self.assertIsNone(error)

    def test_state_dict_is_not_torchscript(self):
        """A state_dict file is rejected as TorchScript."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.pt")
            save_state_dict(path)

            is_valid, error = validate_torchscript_file(path)

            self.assertFalse(is_valid)
            self.assertIsNotNone(error)

    def test_missing_file_is_invalid(self):
        """A nonexistent path is rejected."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "missing.pt")

            is_valid, error = validate_torchscript_file(path)

            self.assertFalse(is_valid)
            self.assertIsNotNone(error)


class ValidateDeployableOnnxFileTest(TestCase):
    """Test cases for ``validate_deployable_onnx_file``."""

    def test_valid_onnx(self):
        """A genuine ONNX export validates successfully."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.onnx")
            model = torch.nn.Linear(4, 2)
            model.eval()
            torch.onnx.export(
                model,
                (torch.randn(2, 4),),
                path,
                input_names=["x"],
                output_names=["y"],
                opset_version=14,
            )

            is_valid, error = validate_deployable_onnx_file(path)

            self.assertTrue(is_valid, error)
            self.assertIsNone(error)

    def test_non_onnx_file_is_invalid(self):
        """A file that is not an ONNX model is rejected."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.onnx")
            with open(path, "wb") as f:
                f.write(b"not an onnx model")

            is_valid, error = validate_deployable_onnx_file(path)

            self.assertFalse(is_valid)
            self.assertIsNotNone(error)

    def test_empty_file_is_invalid(self):
        """An empty file is rejected."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "model.onnx")
            open(path, "w").close()

            is_valid, error = validate_deployable_onnx_file(path)

            self.assertFalse(is_valid)
            self.assertIsInstance(error, ValueError)

    def test_missing_file_is_invalid(self):
        """A nonexistent path is rejected with FileNotFoundError."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = os.path.join(temp_dir, "missing.onnx")

            is_valid, error = validate_deployable_onnx_file(path)

            self.assertFalse(is_valid)
            self.assertIsInstance(error, FileNotFoundError)


class ValidateModelClassTest(TestCase):
    """Test cases for ``validate_model_class``."""

    def test_valid_nn_module_subclass(self):
        """A torch.nn.Module subclass import path validates successfully."""
        is_valid, error = validate_model_class(_MODEL_CLASS)

        self.assertTrue(is_valid)
        self.assertIsNone(error)

    def test_non_module_class_is_invalid(self):
        """A class that is not a torch.nn.Module is rejected with TypeError."""
        is_valid, error = validate_model_class(_NOT_A_MODULE_CLASS)

        self.assertFalse(is_valid)
        self.assertIsInstance(error, TypeError)

    def test_unimportable_class_is_invalid(self):
        """An unresolvable import path is rejected."""
        is_valid, error = validate_model_class("does.not.exist.Model")

        self.assertFalse(is_valid)
        self.assertIsNotNone(error)


class HasBatchDimensionTest(TestCase):
    """Test cases for ``_has_batch_dimension``."""

    def test_batched_tensor_detected(self):
        """A tensor with a batch dimension larger than one is detected."""
        tensor = torch.zeros(8, 4)

        self.assertTrue(_has_batch_dimension(tensor, expected_shape=[4]))

    def test_unbatched_tensor_detected(self):
        """A tensor matching the per-sample shape is not treated as batched."""
        tensor = torch.zeros(4)

        self.assertFalse(_has_batch_dimension(tensor, expected_shape=[4]))

    def test_empty_expected_shape_is_not_batched(self):
        """With no expected shape, the tensor is never treated as batched."""
        tensor = torch.zeros(8, 4)

        self.assertFalse(_has_batch_dimension(tensor, expected_shape=[]))

    def test_scalar_tensor_is_not_batched(self):
        """A 0-dim tensor is never treated as batched."""
        tensor = torch.tensor(1.0)

        self.assertFalse(_has_batch_dimension(tensor, expected_shape=[4]))
