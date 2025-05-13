from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.sdk.model_manager.packager.mobile import MobileModelPackager


class MobileModelPackagerTest(TestCase):
    @patch("uber.ai.michelangelo.sdk.model_manager.packager.mobile.mobile_model_packager.download_model")
    def test_create_model_package(self, mock_download_model):
        mock_download_model.return_value = "test"

        packager = MobileModelPackager()
        path = packager.create_model_package("test")
        self.assertEqual(path, "test")
