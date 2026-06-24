"""Tests for the torch Python-backend deployable model loader."""

import os
import tempfile
from typing import Optional
from unittest import TestCase

import torch

from michelangelo.lib.model_manager._private.serde.loader.torch_deployable_model_loader import (  # noqa: E501
    _import_model_class,
    _load_skeleton,
    _load_torch_python_deployable_model,
    _sys_path,
)


def _make_deployable_package(
    tmp_dir: str,
    model_class_str: str,
    state_dict: dict,
    skeleton: Optional[dict] = None,
) -> str:
    """Build a minimal Triton python-backend package under tmp_dir."""
    version_dir = os.path.join(tmp_dir, "0")
    model_dir = os.path.join(version_dir, "model")
    os.makedirs(model_dir)

    with open(os.path.join(version_dir, "model_class.txt"), "w") as f:
        f.write(model_class_str)

    torch.save(state_dict, os.path.join(model_dir, "model.pt"))

    if skeleton is not None:
        import yaml

        with open(os.path.join(version_dir, "skeleton.yaml"), "w") as f:
            yaml.dump(skeleton, f)

    return tmp_dir


class SysPathTest(TestCase):
    """Tests for the _sys_path context manager."""

    def test_directory_prepended_then_removed(self):
        """The directory is on sys.path inside the block and removed after."""
        import sys

        sentinel = "/tmp/__test_sentinel_path__"
        with _sys_path(sentinel):
            self.assertIn(sentinel, sys.path)
        self.assertNotIn(sentinel, sys.path)

    def test_removal_is_idempotent_when_already_absent(self):
        """No error when directory was already removed from sys.path inside block."""
        import sys

        sentinel = "/tmp/__test_sentinel_path_2__"
        with _sys_path(sentinel):
            sys.path.remove(sentinel)
        # No ValueError raised — the finally block handles the missing entry.


class ImportModelClassTest(TestCase):
    """Tests for _import_model_class."""

    def test_empty_string_raises_value_error(self):
        """An empty model_class_str raises ValueError."""
        with (
            tempfile.TemporaryDirectory() as tmp,
            self.assertRaisesRegex(ValueError, "model_class.txt is empty"),
        ):
            _import_model_class(tmp, "")

    def test_valid_class_imported(self):
        """A valid dotted class name is imported correctly."""
        with tempfile.TemporaryDirectory() as tmp:
            cls = _import_model_class(tmp, "torch.nn.Linear")
            self.assertIs(cls, torch.nn.Linear)


class LoadSkeletonTest(TestCase):
    """Tests for _load_skeleton."""

    def test_no_file_returns_empty_dict(self):
        """Returns empty dict when no skeleton or hyperparameters file exists."""
        with tempfile.TemporaryDirectory() as tmp:
            version_dir = os.path.join(tmp, "0")
            os.makedirs(version_dir)
            result = _load_skeleton(version_dir)
            self.assertEqual(result, {})

    def test_reads_yaml_skeleton(self):
        """Reads constructor kwargs from skeleton.yaml."""
        import yaml

        with tempfile.TemporaryDirectory() as tmp:
            version_dir = os.path.join(tmp, "0")
            os.makedirs(version_dir)
            data = {"in_features": 4, "out_features": 2}
            with open(os.path.join(version_dir, "skeleton.yaml"), "w") as f:
                yaml.dump(data, f)

            result = _load_skeleton(version_dir)
            self.assertEqual(result, data)

    def test_reads_hyperparameters_json_fallback(self):
        """Falls back to hyperparameters.json when skeleton.yaml is absent."""
        import json

        with tempfile.TemporaryDirectory() as tmp:
            version_dir = os.path.join(tmp, "0")
            os.makedirs(version_dir)
            data = {"in_features": 8, "out_features": 4}
            with open(os.path.join(version_dir, "hyperparameters.json"), "w") as f:
                json.dump(data, f)

            result = _load_skeleton(version_dir)
            self.assertEqual(result, data)


class LoadTorchPythonDeployableModelTest(TestCase):
    """Tests for _load_torch_python_deployable_model."""

    def test_missing_model_class_txt_raises_value_error(self):
        """Missing model_class.txt raises ValueError."""
        with tempfile.TemporaryDirectory() as tmp:
            os.makedirs(os.path.join(tmp, "0"))
            with self.assertRaisesRegex(ValueError, "Missing model_class.txt"):
                _load_torch_python_deployable_model(tmp)

    def test_missing_weights_raises_file_not_found(self):
        """Missing model.pt raises FileNotFoundError."""
        with tempfile.TemporaryDirectory() as tmp:
            version_dir = os.path.join(tmp, "0")
            os.makedirs(version_dir)
            with open(os.path.join(version_dir, "model_class.txt"), "w") as f:
                f.write("torch.nn.Linear")
            import yaml

            with open(os.path.join(version_dir, "skeleton.yaml"), "w") as f:
                yaml.dump({"in_features": 4, "out_features": 2}, f)

            with self.assertRaisesRegex(FileNotFoundError, "No model weights"):
                _load_torch_python_deployable_model(tmp)

    def test_loads_simple_model_from_plain_skeleton(self):
        """Successfully loads a model from a plain skeleton (no _target_)."""
        model = torch.nn.Linear(4, 2)
        state_dict = model.state_dict()

        with tempfile.TemporaryDirectory() as tmp:
            _make_deployable_package(
                tmp,
                model_class_str="torch.nn.Linear",
                state_dict=state_dict,
                skeleton={"in_features": 4, "out_features": 2},
            )

            loaded = _load_torch_python_deployable_model(tmp)

            self.assertIsInstance(loaded, torch.nn.Linear)
            self.assertFalse(loaded.training)
            x = torch.randn(1, 4)
            with torch.no_grad():
                self.assertEqual(loaded(x).shape, (1, 2))

    def test_loads_model_from_target_skeleton(self):
        """Successfully loads a model from a _target_ skeleton."""
        model = torch.nn.Linear(4, 2)
        state_dict = model.state_dict()

        with tempfile.TemporaryDirectory() as tmp:
            _make_deployable_package(
                tmp,
                model_class_str="torch.nn.Linear",
                state_dict=state_dict,
                skeleton={
                    "_target_": "torch.nn.Linear",
                    "in_features": 4,
                    "out_features": 2,
                },
            )

            loaded = _load_torch_python_deployable_model(tmp)

            self.assertIsInstance(loaded, torch.nn.Linear)

    def test_state_dict_mismatch_raises_runtime_error(self):
        """A mismatched state dict raises RuntimeError."""
        wrong_state_dict = torch.nn.Linear(8, 4).state_dict()

        with tempfile.TemporaryDirectory() as tmp:
            _make_deployable_package(
                tmp,
                model_class_str="torch.nn.Linear",
                state_dict=wrong_state_dict,
                skeleton={"in_features": 4, "out_features": 2},
            )

            with self.assertRaisesRegex(RuntimeError, "Failed to load state dict"):
                _load_torch_python_deployable_model(tmp)
