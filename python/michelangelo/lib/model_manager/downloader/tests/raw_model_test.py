from unittest import TestCase
from unittest.mock import patch
from michelangelo.lib.model_manager.downloader import download_raw_model


class RawModelTest(TestCase):
    @patch("michelangelo.lib.model_manager.downloader.raw_model.download_generic_raw_model")
    def test_download_raw_model(self, mock_download_generic_raw_model):
        dest_model_path = download_raw_model("test_project", "test_model")
        self.assertIsNotNone(dest_model_path)
        mock_download_generic_raw_model.assert_called_once_with("test_project", "test_model", None, dest_model_path, timeout=None, source_entity=None)

    @patch("michelangelo.lib.model_manager.downloader.raw_model.download_generic_raw_model")
    def test_download_raw_model_with_params(self, mock_download_generic_raw_model):
        dest_model_path = download_raw_model("test_project", "test_model", "1", "dest_path", timeout="2h", source_entity="source_entity")
        self.assertEqual(dest_model_path, "dest_path")
        mock_download_generic_raw_model.assert_called_once_with(
            "test_project", "test_model", "1", dest_model_path, timeout="2h", source_entity="source_entity"
        )
