"""Lightweight tests for the TorchTritonPackager public API.

These cover instantiation and the early-validation failure paths. Full
packaging behavior that requires loading and converting real models is exercised
by the lower-level conversion and validation tests.
"""

import os
import tempfile
from unittest import TestCase

from michelangelo.lib.model_manager._private.packager.torch_triton.tests.fixtures.simple_model import (  # noqa: E501
    save_state_dict,
)
from michelangelo.lib.model_manager.packager.torch_triton.torch_triton_packager import (
    TorchTritonPackager,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem


class TorchTritonPackagerTest(TestCase):
    """Test cases for ``TorchTritonPackager``."""

    def setUp(self):
        """Set up a packager and a minimal valid schema."""
        self.packager = TorchTritonPackager()
        self.schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="x", data_type=DataType.FLOAT, shape=[4]),
            ],
            output_schema=[
                ModelSchemaItem(name="y", data_type=DataType.FLOAT, shape=[2]),
            ],
        )

    def test_instantiation(self):
        """The packager constructs with a template renderer."""
        packager = TorchTritonPackager()

        self.assertIsNotNone(packager.gen)

    def test_unsupported_backend_raises_value_error(self):
        """An unsupported backend is rejected with ValueError."""
        with self.assertRaisesRegex(ValueError, "Unsupported backend"):
            self.packager.create_model_package(
                model_path="model.pt",
                model_schema=self.schema,
                backend="tensorflow",
            )

    def test_missing_model_path_raises_value_error(self):
        """A missing model_path is rejected with ValueError."""
        with self.assertRaisesRegex(ValueError, "model_path is required"):
            self.packager.create_model_package(
                model_path="",
                model_schema=self.schema,
            )

    def test_missing_model_schema_raises_value_error(self):
        """A missing model_schema is rejected with ValueError."""
        with self.assertRaisesRegex(ValueError, "model_schema is required"):
            self.packager.create_model_package(
                model_path="model.pt",
                model_schema=None,
            )

    def test_python_backend_without_model_class_raises_value_error(self):
        """The python backend requires a model_class."""
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model.pt")
            save_state_dict(model_path)

            with self.assertRaisesRegex(
                ValueError, "model_class is required for Python backend"
            ):
                self.packager.create_model_package(
                    model_path=model_path,
                    model_schema=self.schema,
                    backend="python",
                )

    def test_raw_model_package_missing_model_path_raises_value_error(self):
        """create_raw_model_package rejects a missing model_path."""
        with self.assertRaisesRegex(ValueError, "model_path is required"):
            self.packager.create_raw_model_package(
                model_path="",
                model_class="some.Model",
                model_schema=self.schema,
                sample_data=[],
            )

    def test_raw_model_package_missing_model_class_raises_value_error(self):
        """create_raw_model_package rejects a missing model_class."""
        with self.assertRaisesRegex(ValueError, "model_class is required"):
            self.packager.create_raw_model_package(
                model_path="model.pt",
                model_class="",
                model_schema=self.schema,
                sample_data=[],
            )

    def test_raw_model_package_invalid_model_class_raises(self):
        """create_raw_model_package rejects an invalid model_class."""
        not_a_module_class = (
            "michelangelo.lib.model_manager._private.packager.torch_triton."
            "tests.fixtures.simple_model.NotAModule"
        )
        with self.assertRaises(TypeError):
            self.packager.create_raw_model_package(
                model_path="model.pt",
                model_class=not_a_module_class,
                model_schema=self.schema,
            )

    def test_raw_model_package_missing_schema_raises_value_error(self):
        """create_raw_model_package rejects a missing model_schema."""
        model_class = (
            "michelangelo.lib.model_manager._private.packager.torch_triton."
            "tests.fixtures.simple_model.SimpleModel"
        )
        with self.assertRaisesRegex(ValueError, "model_schema is required"):
            self.packager.create_raw_model_package(
                model_path="model.pt",
                model_class=model_class,
                model_schema=None,
            )

    def test_create_raw_model_package_success(self):
        """create_raw_model_package returns a valid package directory."""
        model_class = (
            "michelangelo.lib.model_manager._private.packager.torch_triton."
            "tests.fixtures.simple_model.SimpleModel"
        )
        with tempfile.TemporaryDirectory() as tmp:
            model_path = os.path.join(tmp, "model.pt")
            save_state_dict(model_path)
            dest = os.path.join(tmp, "pkg")

            result = self.packager.create_raw_model_package(
                model_path=model_path,
                model_class=model_class,
                model_schema=self.schema,
                dest_model_path=dest,
                include_import_prefixes=["michelangelo"],
            )

            self.assertTrue(os.path.isdir(result))
            self.assertTrue(os.path.isfile(os.path.join(result, "model", "model.pt")))

    def test_create_model_package_invalid_schema_raises(self):
        """create_model_package raises when schema is invalid."""
        from michelangelo.lib.model_manager.schema import ModelSchema

        bad_schema = ModelSchema(input_schema=[], output_schema=[])
        with tempfile.TemporaryDirectory() as tmp:
            model_path = os.path.join(tmp, "model.pt")
            save_state_dict(model_path)

            with self.assertRaises(ValueError):
                self.packager.create_model_package(
                    model_path=model_path,
                    model_schema=bad_schema,
                )

    def test_create_model_package_python_backend_invalid_class_raises(self):
        """Python backend with invalid model_class raises."""
        not_a_module = (
            "michelangelo.lib.model_manager._private.packager.torch_triton."
            "tests.fixtures.simple_model.NotAModule"
        )
        with tempfile.TemporaryDirectory() as tmp:
            model_path = os.path.join(tmp, "model.pt")
            save_state_dict(model_path)

            with self.assertRaises(TypeError):
                self.packager.create_model_package(
                    model_path=model_path,
                    model_schema=self.schema,
                    backend="python",
                    model_class=not_a_module,
                )
