"""Tests for exporting PyTorch artifacts to ONNX."""

import os
import shutil
import tempfile
from unittest import TestCase
from unittest.mock import patch

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

    def test_onnx_directory_source_moves_onnx_and_sidecars(self):
        """A directory source (ONNX graph + external-data sidecars) is moved.

        The graph is moved next to dest_onnx_path and renamed to dest's
        basename.
        """
        with tempfile.TemporaryDirectory() as temp_dir:
            source_dir = os.path.join(temp_dir, "source")
            os.makedirs(source_dir)
            self._export_reference_onnx(os.path.join(source_dir, "graph.onnx"))
            sidecar_path = os.path.join(source_dir, "embedding.weight")
            with open(sidecar_path, "wb") as f:
                f.write(b"fake-external-weight-bytes")

            dest_dir = os.path.join(temp_dir, "dest")
            os.makedirs(dest_dir)
            dest = os.path.join(dest_dir, "model.onnx")

            convert_to_onnx(source_dir, dest, self.schema, sample_data=None)

            self.assertTrue(os.path.exists(dest))
            self.assertTrue(os.path.exists(os.path.join(dest_dir, "embedding.weight")))
            self.assertEqual(os.listdir(source_dir), [])
            is_onnx, error = validate_deployable_onnx_file(dest)
            self.assertTrue(is_onnx, error)

    def test_onnx_directory_without_exactly_one_onnx_file_raises_value_error(self):
        """A directory with zero or multiple .onnx files is rejected."""
        with tempfile.TemporaryDirectory() as temp_dir:
            source_dir = os.path.join(temp_dir, "source")
            os.makedirs(source_dir)
            dest = os.path.join(temp_dir, "model.onnx")

            with self.assertRaises(ValueError):
                convert_to_onnx(source_dir, dest, self.schema, sample_data=None)

            self._export_reference_onnx(os.path.join(source_dir, "a.onnx"))
            self._export_reference_onnx(os.path.join(source_dir, "b.onnx"))

            with self.assertRaises(ValueError):
                convert_to_onnx(source_dir, dest, self.schema, sample_data=None)

    def test_onnx_directory_with_invalid_onnx_raises_value_error(self):
        """A directory whose sole .onnx file fails validation is rejected."""
        with tempfile.TemporaryDirectory() as temp_dir:
            source_dir = os.path.join(temp_dir, "source")
            os.makedirs(source_dir)
            with open(os.path.join(source_dir, "graph.onnx"), "wb") as f:
                f.write(b"not a real onnx file")
            dest = os.path.join(temp_dir, "model.onnx")

            with self.assertRaises(ValueError):
                convert_to_onnx(source_dir, dest, self.schema, sample_data=None)

    def test_onnx_directory_with_subdirectory_raises_value_error(self):
        """A source directory containing a nested subdirectory is rejected."""
        with tempfile.TemporaryDirectory() as temp_dir:
            source_dir = os.path.join(temp_dir, "source")
            os.makedirs(source_dir)
            self._export_reference_onnx(os.path.join(source_dir, "graph.onnx"))
            os.makedirs(os.path.join(source_dir, "nested"))
            dest = os.path.join(temp_dir, "model.onnx")

            with self.assertRaises(ValueError):
                convert_to_onnx(source_dir, dest, self.schema, sample_data=None)

    def test_onnx_directory_sidecar_colliding_with_dest_basename_raises_value_error(
        self,
    ):
        """A sidecar file already named like the destination graph is rejected."""
        with tempfile.TemporaryDirectory() as temp_dir:
            source_dir = os.path.join(temp_dir, "source")
            os.makedirs(source_dir)
            self._export_reference_onnx(os.path.join(source_dir, "graph.onnx"))
            with open(os.path.join(source_dir, "model.onnx.data"), "wb") as f:
                f.write(b"fake-sidecar")
            dest_dir = os.path.join(temp_dir, "dest")
            os.makedirs(dest_dir)
            dest = os.path.join(dest_dir, "model.onnx.data")

            with self.assertRaises(ValueError):
                convert_to_onnx(source_dir, dest, self.schema, sample_data=None)

    def test_export_passes_external_data_when_torch_supports_it(self):
        """Export includes external_data=True when torch.onnx.export supports it."""
        with tempfile.TemporaryDirectory() as temp_dir:
            source = os.path.join(temp_dir, "model.pt")
            dest = os.path.join(temp_dir, "model.onnx")
            save_full_model(source)
            reference = os.path.join(temp_dir, "reference.onnx")
            self._export_reference_onnx(reference)

            def fake_export(model, args, f, *, external_data=False, **kwargs):
                self.assertTrue(external_data)
                shutil.copy2(reference, f)

            with patch("torch.onnx.export", new=fake_export):
                convert_to_onnx(source, dest, self.schema, sample_data=self.sample_data)

            self.assertTrue(os.path.exists(dest))

    def test_export_omits_external_data_when_torch_does_not_support_it(self):
        """Export doesn't pass external_data when the installed torch lacks it."""
        with tempfile.TemporaryDirectory() as temp_dir:
            source = os.path.join(temp_dir, "model.pt")
            dest = os.path.join(temp_dir, "model.onnx")
            save_full_model(source)
            reference = os.path.join(temp_dir, "reference.onnx")
            self._export_reference_onnx(reference)

            def fake_export(model, args, f, **kwargs):
                self.assertNotIn("external_data", kwargs)
                shutil.copy2(reference, f)

            with patch("torch.onnx.export", new=fake_export):
                convert_to_onnx(source, dest, self.schema, sample_data=self.sample_data)

            self.assertTrue(os.path.exists(dest))
