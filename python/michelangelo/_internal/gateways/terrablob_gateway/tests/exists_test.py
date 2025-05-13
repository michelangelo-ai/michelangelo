from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.shared.gateways.terrablob_gateway import path_exists
from uber.ai.michelangelo.shared.errors.terrablob_error import (
    TerrablobError,
    TerrablobFileNotFoundError,
    TerrablobFailedPreconditionError,
)


class ExistsTest(TestCase):
    @patch("uber.ai.michelangelo.shared.gateways.terrablob_gateway.exists.get_blob_info")
    @patch("uber.ai.michelangelo.shared.gateways.terrablob_gateway.exists.list_terrablob_dir")
    def test_path_exists(self, mock_list_terrablob_dir, mock_get_blob_info):
        self.assertTrue(path_exists("test"))

        mock_get_blob_info.side_effect = TerrablobFailedPreconditionError("test")
        self.assertTrue(path_exists("test"))

        mock_list_terrablob_dir.side_effect = TerrablobFileNotFoundError("test")
        self.assertFalse(path_exists("test"))

        mock_get_blob_info.side_effect = TerrablobFileNotFoundError("test")
        self.assertFalse(path_exists("test"))

        mock_get_blob_info.side_effect = TerrablobError("test")
        with self.assertRaises(TerrablobError):
            path_exists("test")

    def test_path_exists_invalid_arguments(self):
        with self.assertRaises(TypeError):
            path_exists("test", test="test")
