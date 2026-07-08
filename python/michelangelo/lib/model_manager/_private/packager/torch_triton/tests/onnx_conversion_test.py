"""Tests for exporting PyTorch artifacts to ONNX."""

import os
import tempfile
from unittest import TestCase

import numpy as np
import torch

from michelangelo.lib.model_manager._private.packager.torch_triton.onnx_conversion import (  # noqa: E501
    convert_to_onnx,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.tests.fixtures.simple_model import (  # noqa: E501
    SimpleModel,
    save_full_model,
    save_state_dict,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.validation import (
    validate_deployable_onnx_file,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem

_MODEL_CLASS = (
    "michelangelo.lib.model_manager._private.packager.torch_triton."
    "tests.fixtures.simple_model.SimpleModel"
)


class OnnxConversionTest(TestCase):
    """Test cases for ``convert_to_onnx``."""

    def setUp(self):
        """Set up a single-input/single-output schema and trace inputs."""
        self.schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[4]),
            ],
            output_schema=[
                ModelSchemaItem(name="y", data_type=DataType.FLOAT, shape=[2]),
            ],
        )
        self.sample_data = {"x": np.random.randn(2, 4).astype(np.float32)}

    def _export_reference_onnx(self, dest_path: str):
        """Export a SimpleModel to ONNX at dest_path for passthrough tests."""
        model = SimpleModel()
        model.eval()
        torch.onnx.export(
            model,
            (torch.randn(2, 4),),
            dest_path,
            input_names=["x"],
            output_names=["y"],
            opset_version=14,
        )

    def test_already_onnx_is_copied(self):
        """An existing valid .onnx source is copied to the destination."""
        with tempfile.TemporaryDirectory() as temp_dir:
            source = os.path.join(temp_dir, "source.onnx")
            dest = os.path.join(temp_dir, "model.onnx")
            self._export_reference_onnx(source)

            convert_to_onnx(source, dest, self.schema, sample_data=None)

            self.assertTrue(os.path.exists(dest))
            is_onnx, _ = validate_deployable_onnx_file(dest)
            self.assertTrue(is_onnx)

    def test_nn_module_is_exported(self):
        """A pickled nn.Module is exported to a valid ONNX file."""
        with tempfile.TemporaryDirectory() as temp_dir:
            source = os.path.join(temp_dir, "model.pt")
            dest = os.path.join(temp_dir, "model.onnx")
            save_full_model(source)

            convert_to_onnx(source, dest, self.schema, sample_data=self.sample_data)

            is_onnx, error = validate_deployable_onnx_file(dest)
            self.assertTrue(is_onnx, error)

    def test_state_dict_with_model_class_is_exported(self):
        """A state_dict plus a model_class is rebuilt and exported to ONNX."""
        with tempfile.TemporaryDirectory() as temp_dir:
            source = os.path.join(temp_dir, "model.pt")
            dest = os.path.join(temp_dir, "model.onnx")
            save_state_dict(source)

            convert_to_onnx(
                source,
                dest,
                self.schema,
                sample_data=self.sample_data,
                model_class=_MODEL_CLASS,
            )

            is_onnx, error = validate_deployable_onnx_file(dest)
            self.assertTrue(is_onnx, error)

    def test_missing_sample_data_raises_value_error(self):
        """Exporting a PyTorch source without sample_data raises ValueError."""
        with tempfile.TemporaryDirectory() as temp_dir:
            source = os.path.join(temp_dir, "model.pt")
            dest = os.path.join(temp_dir, "model.onnx")
            save_full_model(source)

            with self.assertRaises(ValueError):
                convert_to_onnx(source, dest, self.schema, sample_data=None)

    def test_nonexistent_source_raises_file_not_found(self):
        """A missing source path raises FileNotFoundError."""
        with tempfile.TemporaryDirectory() as temp_dir:
            source = os.path.join(temp_dir, "missing.pt")
            dest = os.path.join(temp_dir, "model.onnx")

            with self.assertRaises(FileNotFoundError):
                convert_to_onnx(source, dest, self.schema, sample_data=self.sample_data)

    def test_state_dict_without_model_class_raises_value_error(self):
        """A state_dict source without model_class raises ValueError."""
        with tempfile.TemporaryDirectory() as temp_dir:
            source = os.path.join(temp_dir, "model.pt")
            dest = os.path.join(temp_dir, "model.onnx")
            save_state_dict(source)

            with self.assertRaises(ValueError):
                convert_to_onnx(source, dest, self.schema, sample_data=self.sample_data)
