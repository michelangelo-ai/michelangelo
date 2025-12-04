import os
import tempfile
from unittest import TestCase
from michelangelo.lib.model_manager._private.packager.custom_triton import (
    serialize_model_interface,
    validate_model_class,
)

module_path = os.path.join(
    "michelangelo", "lib", "model_manager", "interface", "custom_model.py"
)


class ModelInterfaceTest(TestCase):
    """Tests model interface serialization and validation."""

    def test_serialize_model_interface(self):
        """It serializes the model interface to the target directory."""
        with tempfile.TemporaryDirectory() as target_dir:
            serialize_model_interface(target_dir)
            with open(os.path.join(target_dir, module_path)) as f:
                self.assertIn("class Model", f.read())

    def test_serialize_model_interface_already_exists(self):
        """It does not overwrite the existing model interface."""
        with tempfile.TemporaryDirectory() as target_dir:
            target_path = os.path.join(target_dir, module_path)
            os.makedirs(os.path.dirname(target_path))
            with open(target_path, "w") as f:
                f.write("content")

            serialize_model_interface(target_dir)
            with open(target_path) as f:
                self.assertEqual(f.read(), "content")

    def test_validate_model_class(self):
        """It validates the model class."""
        model_class_name = (
            "michelangelo.lib.model_manager.interface.tests."
            "fixtures.custom_model.CustomModel"
        )
        valid, error = validate_model_class(model_class_name)
        self.assertTrue(valid)
        self.assertIsNone(error)

    def test_validate_model_class_invalid(self):
        """It validates the model class is invalid."""
        valid, error = validate_model_class("a.b")
        self.assertFalse(valid)
        self.assertIsInstance(error, ImportError)

        valid, error = validate_model_class("a")
        self.assertFalse(valid)
        self.assertIsInstance(error, ValueError)

        model_class_name = (
            "michelangelo.lib.model_manager._private.packager.custom_triton."
            "tests.fixtures.invalid_model.Model"
        )
        valid, error = validate_model_class(model_class_name)
        self.assertFalse(valid)
        self.assertIsInstance(error, TypeError)
