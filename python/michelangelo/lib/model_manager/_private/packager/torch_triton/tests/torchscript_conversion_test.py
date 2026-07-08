"""Tests for converting PyTorch artifacts to TorchScript."""

import os
import tempfile
from unittest import TestCase

import torch

from michelangelo.lib.model_manager._private.packager.torch_triton.tests.fixtures.simple_model import (  # noqa: E501
    SimpleModel,
    save_full_model,
    save_scripted_model,
    save_state_dict,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.torchscript_conversion import (  # noqa: E501
    _convert_to_torchscript,
)

_MODEL_CLASS = (
    "michelangelo.lib.model_manager._private.packager.torch_triton."
    "tests.fixtures.simple_model.SimpleModel"
)


class _UnscriptableModel(torch.nn.Module):
    """Model that cannot be TorchScript compiled due to list() on a tensor."""

    def forward(self, x: torch.Tensor) -> list:
        """Forward pass."""
        return list(x)  # list(tensor) is not TorchScript-compatible


class TorchScriptConversionTest(TestCase):
    """Test cases for ``_convert_to_torchscript``."""

    def _assert_loads_as_torchscript(self, model_path: str):
        """Assert the file at model_path is a loadable TorchScript model."""
        loaded = torch.jit.load(model_path, map_location="cpu")
        self.assertIsInstance(loaded, torch.jit.ScriptModule)

    def test_already_torchscript_is_passthrough(self):
        """An already-scripted file is left byte-for-byte unchanged."""
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model.pt")
            save_scripted_model(model_path)
            with open(model_path, "rb") as f:
                before = f.read()

            _convert_to_torchscript(model_path)

            with open(model_path, "rb") as f:
                after = f.read()
            self.assertEqual(before, after)
            self._assert_loads_as_torchscript(model_path)

    def test_nn_module_is_converted(self):
        """A pickled nn.Module is scripted in place."""
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model.pt")
            save_full_model(model_path)

            _convert_to_torchscript(model_path)

            self._assert_loads_as_torchscript(model_path)

    def test_state_dict_with_model_class_is_converted(self):
        """A state_dict plus a model_class is rebuilt and scripted in place."""
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model.pt")
            save_state_dict(model_path)

            _convert_to_torchscript(model_path, model_class=_MODEL_CLASS)

            self._assert_loads_as_torchscript(model_path)

    def test_state_dict_without_model_class_raises_value_error(self):
        """A state_dict with no model_class raises ValueError (not TypeError)."""
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model.pt")
            save_state_dict(model_path)

            with self.assertRaises(ValueError):
                _convert_to_torchscript(model_path, model_class=None)

    def test_nonexistent_file_raises_file_not_found(self):
        """A missing source path raises FileNotFoundError."""
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "missing.pt")

            with self.assertRaises(FileNotFoundError):
                _convert_to_torchscript(model_path)

    def test_converted_state_dict_model_produces_same_output(self):
        """The scripted state_dict model matches the eager model's forward."""
        torch.manual_seed(0)
        model = SimpleModel()
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model.pt")
            torch.save(model.state_dict(), model_path)

            _convert_to_torchscript(model_path, model_class=_MODEL_CLASS)

            scripted = torch.jit.load(model_path, map_location="cpu")
            sample = torch.randn(2, 4)
            model.eval()
            with torch.no_grad():
                expected = model(sample)
                actual = scripted(sample)
            self.assertTrue(torch.allclose(expected, actual, atol=1e-6))

    def test_non_convertible_model_raises_type_error(self):
        """A model that cannot be scripted raises TypeError."""
        with tempfile.TemporaryDirectory() as tmp:
            model_path = os.path.join(tmp, "model.pt")
            torch.save(_UnscriptableModel(), model_path)
            with self.assertRaises(TypeError):
                _convert_to_torchscript(model_path)
