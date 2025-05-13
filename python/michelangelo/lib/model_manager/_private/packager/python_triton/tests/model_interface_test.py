import os
import tempfile
from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager._private.packager.python_triton import (
    serialize_model_interface,
    validate_model_class,
)

# enable metabuild to build bazel dependencies
import uber.ai.michelangelo.sdk.model_manager.interface.tests.fixtures.custom_model
import uber.ai.michelangelo.sdk.model_manager._private.packager.python_triton.tests.fixtures.invalid_model  # noqa:F401

module_path = os.path.join("uber", "ai", "michelangelo", "sdk", "model_manager", "interface", "custom_model.py")


class ModelInterfaceTest(TestCase):
    def test_serialize_model_interface(self):
        with tempfile.TemporaryDirectory() as target_dir:
            serialize_model_interface(target_dir)
            with open(os.path.join(target_dir, module_path)) as f:
                self.assertIn("class Model", f.read())

    def test_serialize_model_interface_already_exists(self):
        with tempfile.TemporaryDirectory() as target_dir:
            target_path = os.path.join(target_dir, module_path)
            os.makedirs(os.path.dirname(target_path))
            with open(target_path, "w") as f:
                f.write("content")

            serialize_model_interface(target_dir)
            with open(target_path) as f:
                self.assertEqual(f.read(), "content")

    def test_validate_model_class(self):
        valid, error = validate_model_class("uber.ai.michelangelo.sdk.model_manager.interface.tests.fixtures.custom_model.CustomModel")
        self.assertTrue(valid)
        self.assertIsNone(error)

    def test_validate_model_class_invalid(self):
        valid, error = validate_model_class("a.b")
        self.assertFalse(valid)
        self.assertIsInstance(error, ImportError)

        valid, error = validate_model_class("a")
        self.assertFalse(valid)
        self.assertIsInstance(error, ValueError)

        valid, error = validate_model_class("uber.ai.michelangelo.sdk.model_manager._private.packager.python_triton.tests.fixtures.invalid_model.Model")
        self.assertFalse(valid)
        self.assertIsInstance(error, TypeError)
