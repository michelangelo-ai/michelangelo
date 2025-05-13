import re
import numpy as np
from unittest import TestCase
from unittest.mock import patch
from michelangelo.lib.model_manager.schema import ModelSchema
from michelangelo.lib.model_manager._private.packager.python_triton import generate_raw_model_package_content

# enable metabuild to build bazel dependencies
import michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.predict  # noqa:F401


class RawModelPackageTest(TestCase):
    @patch("michelangelo.lib.model_manager._private.packager.python_triton.raw_model_package.download_model")
    def test_generate_raw_model_package_content(self, mock_download_model):
        content = generate_raw_model_package_content(
            "test_model_path",
            "michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.predict.Predict",
            ModelSchema(),
            [{"input": np.array([1, 2])}],
            include_import_prefixes=["uber"],
        )

        mock_download_model.assert_called_once()
        self.assertIsNotNone(content)
        self.assertIn("metadata", content)
        self.assertIn("type.yaml", content["metadata"])
        self.assertIn("schema.yaml", content["metadata"])
        self.assertIn("sample_data.json", content["metadata"])
        self.assertIn("model", content)
        self.assertIn("defs", content)
        model = content["model"]
        self.assertIsNotNone(re.fullmatch(r"dir://(?:/tmp/.+)/model", model))
        defs = content["defs"]
        self.assertIsNotNone(re.fullmatch(r"dir://(?:/tmp/.+)/defs", defs))
        self.assertNotIn("dependencies", content)

    @patch("michelangelo.lib.model_manager._private.packager.python_triton.raw_model_package.download_model")
    def test_generate_raw_model_package_content_with_batch_inference(self, mock_download_model):
        content = generate_raw_model_package_content(
            "test_model_path",
            "michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.predict.Predict",
            ModelSchema(),
            [{"input": np.array([1, 2])}],
            include_import_prefixes=["uber"],
            batch_inference=True,
        )

        mock_download_model.assert_called_once()
        self.assertIsNotNone(content)
        self.assertIn("metadata", content)
        self.assertIn("type.yaml", content["metadata"])
        self.assertIn("schema.yaml", content["metadata"])
        self.assertIn("sample_data.json", content["metadata"])
        self.assertIn("model", content)
        self.assertIn("defs", content)
        model = content["model"]
        self.assertIsNotNone(re.fullmatch(r"dir://(?:/tmp/.+)/model", model))
        defs = content["defs"]
        self.assertIsNotNone(re.fullmatch(r"dir://(?:/tmp/.+)/defs", defs))
        self.assertNotIn("dependencies", content)

    @patch("michelangelo.lib.model_manager._private.packager.python_triton.raw_model_package.download_model")
    def test_generate_raw_model_package_content_with_requirements(self, mock_download_model):
        content = generate_raw_model_package_content(
            "test_model_path",
            "michelangelo.lib.model_manager._private.packager.python_triton.tests.fixtures.predict.Predict",
            ModelSchema(),
            [{"input": np.array([1, 2])}],
            requirements=["numpy", "torch"],
            include_import_prefixes=["uber"],
        )

        mock_download_model.assert_called()
        self.assertIsNotNone(content)
        self.assertIn("metadata", content)
        self.assertIn("type.yaml", content["metadata"])
        self.assertIn("schema.yaml", content["metadata"])
        self.assertIn("sample_data.json", content["metadata"])
        self.assertIn("model", content)
        self.assertIn("defs", content)
        self.assertIn("dependencies", content)
        self.assertIn("requirements.txt", content["dependencies"])
        requirements = content["dependencies"]["requirements.txt"]
        self.assertEqual(requirements, "numpy\ntorch")
