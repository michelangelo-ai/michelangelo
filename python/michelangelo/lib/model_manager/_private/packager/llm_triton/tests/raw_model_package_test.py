from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton import generate_raw_model_package_content


class RawModelPackageTest(TestCase):
    @patch("uber.ai.michelangelo.sdk.model_manager._private.packager.llm_triton.raw_model_package.download_model")
    def test_generate_raw_model_package_content(self, mock_download_model):
        content = generate_raw_model_package_content("test_model_path")
        self.assertIsNotNone(content)
        self.assertIn("metadata", content)
        self.assertIn("type.yaml", content["metadata"])
        self.assertIn("model", content)
        mock_download_model.assert_called_once()
