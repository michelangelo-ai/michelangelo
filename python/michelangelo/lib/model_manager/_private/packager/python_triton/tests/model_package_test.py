from unittest import TestCase
from unittest.mock import patch
import re
from michelangelo.lib.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from michelangelo.lib.model_manager._private.packager.python_triton import generate_model_package_content

# enable metabuild to build bazel dependencies
import michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.predict  # noqa:F401


class ModelPackageTest(TestCase):
    @patch("michelangelo.lib.model_manager._private.packager.python_triton.model_package.download_model")
    def test_generate_model_package_content(self, mock_download_model):
        mock_download_model.return_value = None
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
            "test_model_path",
            "test_model_name",
            "test_model_revision",
            "michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.predict.Predict",
            input_schema,
            output_schema,
            include_import_prefixes=["uber"],
        )

        self.assertIsNotNone(content)
        self.assertIn("config.pbtxt", content)
        self.assertIn("0", content)
        self.assertIn("model.py", content["0"])
        self.assertIn("user_model.py", content["0"])
        predict = content["0"]["model_class.txt"]
        self.assertIsNotNone(re.fullmatch(r"file://(?:/tmp/.+)/model_class.txt", predict))
        model = content["0"]["model"]
        self.assertIsNotNone(re.fullmatch(r"dir://(?:/tmp/.+)/model", model))
