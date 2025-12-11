"""Tests for model package generation."""

import re
import tempfile
from unittest import TestCase
from unittest.mock import patch
from michelangelo.lib.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from michelangelo.lib.model_manager._private.packager.custom_triton import generate_model_package_content


class ModelPackageTest(TestCase):
    """Tests for model package generation."""

    def test_generate_model_package_content(self):
        """It generates the model package content."""
        with tempfile.TemporaryDirectory() as temp_dir:
            gen = TritonTemplateRenderer()
            input_schema = {
                "input": {
                    "type": "int32",
                    "shape": "[ 1 ]",
                },
            }
            output_schema = {
                "response": {
                    "type": "int32",
                    "shape": "[ 1 ]",
                },
            }

            content = generate_model_package_content(
                gen,
                temp_dir,
                "test_model_name",
                "test_model_revision",
                "michelangelo.lib.model_manager._private.packager.custom_triton.tests.fixtures.predict.Predict",
                input_schema,
                output_schema,
                include_import_prefixes=["michelangelo"],
            )

            self.assertIsNotNone(content)
            self.assertIn("config.pbtxt", content)
            self.assertIn("0", content)
            self.assertIn("model.py", content["0"])
            self.assertIn("user_model.py", content["0"])
            predict = content["0"]["model_class.txt"]
            self.assertIsNotNone(re.fullmatch(r"file://(?:/.+)*/model_class.txt", predict))
            model = content["0"]["model"]
            self.assertIsNotNone(re.fullmatch(r"dir://(?:/.+)*/model", model))
