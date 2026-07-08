"""Tests for the torch raw model loader."""

import json
import os
import tempfile
from unittest import TestCase

import torch
import yaml

from michelangelo.lib.model_manager._private.serde.loader.torch_model_loader import (
    load_torch_raw_model,
)


class _LoaderPackage:
    """Builds an on-disk torch raw model package for tests."""

    def __init__(self, tmp_dir: str):
        self.path = tmp_dir
        os.makedirs(os.path.join(self.path, "model"), exist_ok=True)

    def write_model_class(self, value: str):
        with open(os.path.join(self.path, "model_class.txt"), "w") as f:
            f.write(value)

    def write_skeleton(self, skeleton: dict):
        with open(os.path.join(self.path, "skeleton.yaml"), "w") as f:
            yaml.dump(skeleton, f)

    def write_hyperparameters(self, hp: dict):
        with open(os.path.join(self.path, "hyperparameters.json"), "w") as f:
            json.dump(hp, f)

    def write_weights(self, state_dict):
        torch.save(state_dict, os.path.join(self.path, "model", "model.pt"))


class LoadTorchRawModelTest(TestCase):
    """Tests for load_torch_raw_model."""

    def test_missing_model_class_raises(self):
        """A missing model_class.txt raises ValueError."""
        with tempfile.TemporaryDirectory() as tmp:
            _LoaderPackage(tmp)
            with self.assertRaisesRegex(ValueError, "Missing model_class.txt"):
                load_torch_raw_model(tmp)

    def test_empty_model_class_raises(self):
        """An empty model_class.txt raises ValueError."""
        with tempfile.TemporaryDirectory() as tmp:
            pkg = _LoaderPackage(tmp)
            pkg.write_model_class("")
            with self.assertRaisesRegex(ValueError, "is empty"):
                load_torch_raw_model(tmp)

    def test_invalid_model_class_raises(self):
        """An invalid (non-dotted) class name raises ValueError."""
        with tempfile.TemporaryDirectory() as tmp:
            pkg = _LoaderPackage(tmp)
            pkg.write_model_class("Linear")
            with self.assertRaisesRegex(ValueError, "Invalid model class"):
                load_torch_raw_model(tmp)

    def test_missing_weights_raises(self):
        """Missing model weights raises FileNotFoundError."""
        with tempfile.TemporaryDirectory() as tmp:
            pkg = _LoaderPackage(tmp)
            pkg.write_model_class("torch.nn.Linear")
            pkg.write_skeleton({"in_features": 4, "out_features": 2})
            with self.assertRaisesRegex(FileNotFoundError, "No model weights"):
                load_torch_raw_model(tmp)

    def test_loads_with_plain_skeleton(self):
        """Loads model from plain skeleton kwargs."""
        with tempfile.TemporaryDirectory() as tmp:
            pkg = _LoaderPackage(tmp)
            pkg.write_model_class("torch.nn.Linear")
            pkg.write_skeleton({"in_features": 4, "out_features": 2})
            pkg.write_weights(torch.nn.Linear(4, 2).state_dict())

            model = load_torch_raw_model(tmp)
            self.assertIsInstance(model, torch.nn.Linear)
            self.assertFalse(model.training)

    def test_loads_with_target_spec(self):
        """Loads model from _target_ spec skeleton."""
        with tempfile.TemporaryDirectory() as tmp:
            pkg = _LoaderPackage(tmp)
            pkg.write_model_class("torch.nn.Linear")
            pkg.write_skeleton(
                {"_target_": "torch.nn.Linear", "in_features": 4, "out_features": 2}
            )
            pkg.write_weights(torch.nn.Linear(4, 2).state_dict())

            model = load_torch_raw_model(tmp)
            self.assertIsInstance(model, torch.nn.Linear)

    def test_loads_with_hyperparameters_fallback(self):
        """Falls back to hyperparameters.json when skeleton.yaml absent."""
        with tempfile.TemporaryDirectory() as tmp:
            pkg = _LoaderPackage(tmp)
            pkg.write_model_class("torch.nn.Linear")
            pkg.write_hyperparameters({"in_features": 4, "out_features": 2})
            pkg.write_weights(torch.nn.Linear(4, 2).state_dict())

            model = load_torch_raw_model(tmp)
            self.assertIsInstance(model, torch.nn.Linear)

    def test_state_dict_mismatch_raises(self):
        """A mismatched state dict raises RuntimeError."""
        with tempfile.TemporaryDirectory() as tmp:
            pkg = _LoaderPackage(tmp)
            pkg.write_model_class("torch.nn.Linear")
            pkg.write_skeleton({"in_features": 4, "out_features": 2})
            pkg.write_weights(torch.nn.Linear(4, 8).state_dict())

            with self.assertRaisesRegex(RuntimeError, "Failed to load state_dict"):
                load_torch_raw_model(tmp)

    def test_loads_from_metadata_hyperparameters_yaml(self):
        """Falls back to metadata/hyperparameters.yaml for the skeleton."""
        with tempfile.TemporaryDirectory() as tmp:
            pkg = _LoaderPackage(tmp)
            pkg.write_model_class("torch.nn.Linear")
            # Write to metadata/hyperparameters.yaml instead of skeleton.yaml
            os.makedirs(os.path.join(tmp, "metadata"))
            with open(os.path.join(tmp, "metadata", "hyperparameters.yaml"), "w") as f:
                yaml.dump({"in_features": 4, "out_features": 2}, f)
            pkg.write_weights(torch.nn.Linear(4, 2).state_dict())

            model = load_torch_raw_model(tmp)
            self.assertIsInstance(model, torch.nn.Linear)

    def test_non_dict_state_dict_raises_type_error(self):
        """A model file that is not a state_dict raises TypeError."""
        with tempfile.TemporaryDirectory() as tmp:
            pkg = _LoaderPackage(tmp)
            pkg.write_model_class("torch.nn.Linear")
            pkg.write_skeleton({"in_features": 4, "out_features": 2})
            # Save a tensor instead of a state_dict
            torch.save(torch.tensor([1.0, 2.0]), os.path.join(tmp, "model", "model.pt"))

            with self.assertRaisesRegex(TypeError, "Expected state_dict format"):
                load_torch_raw_model(tmp)
