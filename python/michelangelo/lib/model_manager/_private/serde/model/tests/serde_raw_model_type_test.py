"""Tests for raw model type."""

import os
import tempfile
from unittest import TestCase

from michelangelo.lib.model_manager._private.serde.model import get_raw_model_type
from michelangelo.lib.model_manager.constants import RawModelType


def create_model_package(model_path: str, raw_model_type: str):
    """Create a model package with the given raw model type."""
    os.makedirs(os.path.join(model_path, "metadata"), exist_ok=True)
    with open(os.path.join(model_path, "metadata", "type.yaml"), "w") as f:
        f.write(f"type: {raw_model_type}")


class RawModelTypeTest(TestCase):
    """Tests for raw model type."""

    def test_get_raw_model_type(self):
        """Test getting the raw model type."""
        with tempfile.TemporaryDirectory() as model_path:
            create_model_package(model_path, "custom-python")
            self.assertEqual(get_raw_model_type(model_path), RawModelType.CUSTOM_PYTHON)

            create_model_package(model_path, "huggingface")
            self.assertEqual(get_raw_model_type(model_path), RawModelType.HUGGINGFACE)

            create_model_package(model_path, "abc")
            with self.assertRaisesRegex(ValueError, "Invalid model type abc"):
                get_raw_model_type(model_path)

            create_model_package(model_path, "")
            with self.assertRaisesRegex(ValueError, "Model type is empty"):
                get_raw_model_type(model_path)

    def test_get_raw_model_type_no_type_yaml(self):
        """Test getting the raw model type when no type.yaml file is present."""
        with (
            tempfile.TemporaryDirectory() as model_path,
            self.assertRaisesRegex(FileNotFoundError, "type.yaml file not found"),
        ):
            get_raw_model_type(model_path)
