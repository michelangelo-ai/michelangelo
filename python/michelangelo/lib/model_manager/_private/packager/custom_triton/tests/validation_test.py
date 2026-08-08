"""Tests for custom Triton package validation."""

import os
import tempfile
from unittest import TestCase

import numpy as np

from michelangelo.lib.model_manager._private.packager.custom_triton import (
    validate_raw_model_package,
)
from michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures.model_for_validation import (  # noqa: E501
    Predict,
)
from michelangelo.lib.model_manager.packager.custom_triton import CustomTritonPackager
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem


class ValidationTest(TestCase):
    """Test cases for validating custom Triton model packages."""

    def setUp(self):
        """Set up test fixtures."""
        self.schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="input", data_type=DataType.STRING, shape=[1]),
            ],
            output_schema=[
                ModelSchemaItem(name="response", data_type=DataType.STRING, shape=[1]),
            ],
        )
        self.sample_data = [{"input": np.array(["test_input"])}]

    def generate_package(self, model_path: str, model_class: str, dest_model_path: str):
        """Generate a model package for testing."""
        packager = CustomTritonPackager()
        return packager.create_raw_model_package(
            model_path=model_path,
            model_class=model_class,
            model_schema=self.schema,
            sample_data=self.sample_data,
            dest_model_path=dest_model_path,
            include_import_prefixes=["michelangelo"],
        )

    def test_validate_raw_model_package(self):
        """Test validation of a valid raw model package."""
        predict = Predict("test_content")
        model_class = (
            "michelangelo.lib.model_manager._private.packager.custom_triton."
            "tests.fixtures.model_for_validation.Predict"
        )
        with tempfile.TemporaryDirectory() as temp_dir:
            src_model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "model_package")
            os.makedirs(src_model_path)
            predict.save(src_model_path)
            self.generate_package(src_model_path, model_class, dest_model_path)

    def test_validate_raw_model_package_error_cases(self):
        """Test validation failure messages for various model error conditions."""
        cases = [
            (
                "ModelWithPredictError",
                "Error when validating the raw model package. "
                "Error when test prediction with the model",
            ),
            (
                "ModelWithInvalidOutput",
                "Error when validating the raw model package. "
                "Error validating model output data",
            ),
            (
                "ModelWithOutputNotMatchingSchema",
                "Error when validating the raw model package. "
                "Error validating model output data. "
                "Data fields do not match schema",
            ),
            (
                "ModelWithSaveError",
                "Error when validating the raw model package. "
                "Error when test saving the model",
            ),
            (
                "ModelWithReloadError",
                "Error when validating the raw model package. "
                "Error when test reloading the saved model, please double check",
            ),
            (
                "ModelWithMismatchingLoad",
                "Error when validating the raw model package. "
                "The loaded model is not an instance of",
            ),
        ]
        for class_suffix, expected_message in cases:
            with self.subTest(class_suffix=class_suffix):
                predict = Predict("test_content")
                model_class = (
                    "michelangelo.lib.model_manager._private.packager.custom_triton."
                    f"tests.fixtures.model_for_validation.{class_suffix}"
                )
                with tempfile.TemporaryDirectory() as temp_dir:
                    src_model_path = os.path.join(temp_dir, "model")
                    dest_model_path = os.path.join(temp_dir, "model_package")
                    os.makedirs(src_model_path)
                    predict.save(src_model_path)
                    with self.assertRaisesRegex(RuntimeError, expected_message):
                        self.generate_package(
                            src_model_path, model_class, dest_model_path
                        )

    def test_validate_raw_model_package_without_sample_data(self):
        """Test validation of a raw model package without sample data."""
        predict = Predict("test_content")
        model_class = (
            "michelangelo.lib.model_manager._private.packager.custom_triton."
            "tests.fixtures.model_for_validation.Predict"
        )
        with tempfile.TemporaryDirectory() as temp_dir:
            src_model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "model_package")
            os.makedirs(src_model_path)
            predict.save(src_model_path)
            package = self.generate_package(
                src_model_path, model_class, dest_model_path
            )
            validate_raw_model_package(package, None, self.schema)

    def test_validate_model_files_with_reserved_model_py(self):
        """Test that validation rejects models with model.py in root folder."""
        predict = Predict("test_content")
        model_class = (
            "michelangelo.lib.model_manager._private.packager.custom_triton."
            "tests.fixtures.model_for_validation.Predict"
        )

        with tempfile.TemporaryDirectory() as temp_dir:
            # Create source model in a subdirectory to avoid recursive copy issues
            src_model_path = os.path.join(temp_dir, "source_model")
            os.makedirs(src_model_path)
            predict.save(src_model_path)

            # Create a __init__.py file in the root of the source model
            model_py_path = os.path.join(src_model_path, "__init__.py")
            with open(model_py_path, "w") as f:
                f.write("# This is a reserved file name")

            dest_model_path = os.path.join(temp_dir, "model_package")

            with self.assertRaisesRegex(
                ValueError,
                (
                    "Custom model contains the file'__init__.py' "
                    "in the model assets folder."
                ),
            ):
                self.generate_package(src_model_path, model_class, dest_model_path)

    def test_validate_model_files_without_reserved_model_py(self):
        """Test that validation allows models without model.py in root folder."""
        predict = Predict("test_content")
        model_class = (
            "michelangelo.lib.model_manager._private.packager.custom_triton."
            "tests.fixtures.model_for_validation.Predict"
        )
        with tempfile.TemporaryDirectory() as temp_dir:
            src_model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "model_package")
            os.makedirs(src_model_path)
            predict.save(src_model_path)
            # This should succeed without raising an error
            self.generate_package(src_model_path, model_class, dest_model_path)

    def test_validate_model_files_with_model_py_in_subdirectory(self):
        """Test that validation allows model.py in subdirectories."""
        predict = Predict("test_content")
        model_class = (
            "michelangelo.lib.model_manager._private.packager.custom_triton."
            "tests.fixtures.model_for_validation.Predict"
        )
        with tempfile.TemporaryDirectory() as temp_dir:
            src_model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "model_package")
            os.makedirs(src_model_path)
            predict.save(src_model_path)

            # Create a model.py file in a subdirectory (should be allowed)
            subdir = os.path.join(src_model_path, "subdir")
            os.makedirs(subdir)
            with open(os.path.join(subdir, "model.py"), "w") as f:
                f.write("# This is allowed in a subdirectory")

            # This should succeed without raising an error
            self.generate_package(src_model_path, model_class, dest_model_path)
