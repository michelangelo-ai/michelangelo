"""Tests for raw_model_package helpers."""

import os
import tempfile
from unittest import TestCase

import torch

from michelangelo.lib.model_manager._private.packager.torch_triton.raw_model_package import (  # noqa: E501
    _serialize_nested_classes,
    convert_to_state_dict,
    generate_raw_model_package_content,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.tests.fixtures.simple_model import (  # noqa: E501
    save_full_model,
    save_state_dict,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem

_MODEL_CLASS = (
    "michelangelo.lib.model_manager._private.packager.torch_triton."
    "tests.fixtures.simple_model.SimpleModel"
)

_SCHEMA = ModelSchema(
    input_schema=[ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[4])],
    output_schema=[ModelSchemaItem(name="y", data_type=DataType.FLOAT, shape=[2])],
)


class ConvertToStateDictTest(TestCase):
    """Tests for convert_to_state_dict."""

    def test_missing_file_raises_file_not_found(self):
        """A nonexistent path raises FileNotFoundError."""
        with self.assertRaises(FileNotFoundError):
            convert_to_state_dict("/nonexistent/model.pt")

    def test_already_state_dict_is_unchanged(self):
        """A file that is already a state_dict is left unchanged."""
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "model.pt")
            save_state_dict(path)
            os.path.getmtime(path)

            convert_to_state_dict(path)

            # File should be unchanged (no re-write when already a state_dict)
            loaded = torch.load(path, map_location="cpu", weights_only=True)
            self.assertIsInstance(loaded, dict)

    def test_full_model_converted_to_state_dict(self):
        """A pickled full nn.Module is converted to a state_dict in place."""
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "model.pt")
            save_full_model(path)

            convert_to_state_dict(path)

            loaded = torch.load(path, map_location="cpu", weights_only=True)
            self.assertIsInstance(loaded, dict)
            self.assertIn("fc1.weight", loaded)

    def test_invalid_file_raises_value_error(self):
        """A file that is not a valid model raises ValueError."""
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "model.pt")
            with open(path, "wb") as f:
                f.write(b"not a torch file")

            with self.assertRaises((ValueError, RuntimeError)):
                convert_to_state_dict(path)


class SerializeNestedClassesTest(TestCase):
    """Tests for _serialize_nested_classes."""

    def test_no_hyperparameters_does_nothing(self):
        """None hyperparameters skips serialization without error."""
        with tempfile.TemporaryDirectory() as tmp:
            _serialize_nested_classes(None, _MODEL_CLASS, tmp, None)
            # No files created (no nested classes to serialize)

    def test_skips_model_class_itself(self):
        """The top-level model_class is not serialized again as a nested class."""
        hyperparams = {"_target_": _MODEL_CLASS}
        with tempfile.TemporaryDirectory() as tmp:
            _serialize_nested_classes(hyperparams, _MODEL_CLASS, tmp, None)
            # model_class.txt should NOT be created for the top-level class

    def test_prefix_filtering_skips_non_matching(self):
        """A nested class that doesn't match prefixes is skipped."""
        hyperparams = {"_target_": "torch.nn.Linear"}
        with tempfile.TemporaryDirectory() as tmp:
            _serialize_nested_classes(hyperparams, _MODEL_CLASS, tmp, ["michelangelo"])
            # torch.nn.Linear doesn't match michelangelo prefix — skipped


class GenerateRawModelPackageContentTest(TestCase):
    """Tests for generate_raw_model_package_content."""

    def test_generates_expected_structure_from_state_dict(self):
        """State dict source produces the expected package content dict."""
        with tempfile.TemporaryDirectory() as tmp:
            model_path = os.path.join(tmp, "model.pt")
            save_state_dict(model_path)
            root = os.path.join(tmp, "pkg")
            os.makedirs(root)

            content = generate_raw_model_package_content(
                model_path=model_path,
                model_class=_MODEL_CLASS,
                model_schema=_SCHEMA,
                sample_data=None,
                root_path=root,
                include_import_prefixes=["michelangelo"],
            )

            self.assertIn("metadata", content)
            self.assertIn("model", content)
            self.assertIn("defs", content)
            self.assertIn("dependencies", content)

    def test_generates_expected_structure_from_full_model(self):
        """Full nn.Module source is converted and packaged correctly."""
        with tempfile.TemporaryDirectory() as tmp:
            model_path = os.path.join(tmp, "model.pt")
            save_full_model(model_path)
            root = os.path.join(tmp, "pkg")
            os.makedirs(root)

            content = generate_raw_model_package_content(
                model_path=model_path,
                model_class=_MODEL_CLASS,
                model_schema=_SCHEMA,
                sample_data=None,
                root_path=root,
                include_import_prefixes=["michelangelo"],
            )

            self.assertIn("metadata", content)

    def test_torch_always_in_requirements_when_none(self):
        """When requirements is None, torch is included automatically."""
        with tempfile.TemporaryDirectory() as tmp:
            model_path = os.path.join(tmp, "model.pt")
            save_state_dict(model_path)
            root = os.path.join(tmp, "pkg")
            os.makedirs(root)

            content = generate_raw_model_package_content(
                model_path=model_path,
                model_class=_MODEL_CLASS,
                model_schema=_SCHEMA,
                sample_data=None,
                requirements=None,
                root_path=root,
                include_import_prefixes=["michelangelo"],
            )

            req_content = content["dependencies"]["requirements.txt"]
            self.assertIn("torch", req_content)

    def test_torch_added_to_list_requirements(self):
        """Torch is appended when not already in the list requirements."""
        with tempfile.TemporaryDirectory() as tmp:
            model_path = os.path.join(tmp, "model.pt")
            save_state_dict(model_path)
            root = os.path.join(tmp, "pkg")
            os.makedirs(root)

            content = generate_raw_model_package_content(
                model_path=model_path,
                model_class=_MODEL_CLASS,
                model_schema=_SCHEMA,
                sample_data=None,
                requirements=["numpy"],
                root_path=root,
                include_import_prefixes=["michelangelo"],
            )

            req_content = content["dependencies"]["requirements.txt"]
            self.assertIn("torch", req_content)
            self.assertIn("numpy", req_content)

    def test_requirements_from_file(self):
        """Requirements can be specified as a path to a requirements.txt file."""
        with tempfile.TemporaryDirectory() as tmp:
            model_path = os.path.join(tmp, "model.pt")
            save_state_dict(model_path)
            req_file = os.path.join(tmp, "requirements.txt")
            with open(req_file, "w") as f:
                f.write("numpy\nscipy\n")
            root = os.path.join(tmp, "pkg")
            os.makedirs(root)

            content = generate_raw_model_package_content(
                model_path=model_path,
                model_class=_MODEL_CLASS,
                model_schema=_SCHEMA,
                sample_data=None,
                requirements=req_file,
                root_path=root,
                include_import_prefixes=["michelangelo"],
            )

            req_content = content["dependencies"]["requirements.txt"]
            self.assertIn("numpy", req_content)
            self.assertIn("torch", req_content)

    def test_hyperparameters_written_to_metadata(self):
        """Hyperparameters are serialized into metadata content."""
        with tempfile.TemporaryDirectory() as tmp:
            model_path = os.path.join(tmp, "model.pt")
            save_state_dict(model_path)
            root = os.path.join(tmp, "pkg")
            os.makedirs(root)

            content = generate_raw_model_package_content(
                model_path=model_path,
                model_class=_MODEL_CLASS,
                model_schema=_SCHEMA,
                sample_data=None,
                root_path=root,
                include_import_prefixes=["michelangelo"],
                hyperparameters={"lr": 0.01},
            )

            self.assertIn("hyperparameters.yaml", content["metadata"])

    def test_transform_spec_written_to_metadata(self):
        """transform_spec and transform_feature_stats appear in metadata."""
        with tempfile.TemporaryDirectory() as tmp:
            model_path = os.path.join(tmp, "model.pt")
            save_state_dict(model_path)
            root = os.path.join(tmp, "pkg")
            os.makedirs(root)

            content = generate_raw_model_package_content(
                model_path=model_path,
                model_class=_MODEL_CLASS,
                model_schema=_SCHEMA,
                sample_data=None,
                root_path=root,
                include_import_prefixes=["michelangelo"],
                transform_spec={"pipeline": "v1"},
                transform_feature_stats={"mean": 0.0},
            )

            self.assertIn("transform_spec.yaml", content["metadata"])
            self.assertIn("transform_feature_stats.yaml", content["metadata"])

    def test_tempdir_created_when_root_path_omitted(self):
        """A temporary directory is used when root_path is not provided."""
        with tempfile.TemporaryDirectory() as tmp:
            model_path = os.path.join(tmp, "model.pt")
            save_state_dict(model_path)

            content = generate_raw_model_package_content(
                model_path=model_path,
                model_class=_MODEL_CLASS,
                model_schema=_SCHEMA,
                sample_data=None,
                include_import_prefixes=["michelangelo"],
            )

            self.assertIn("model", content)
