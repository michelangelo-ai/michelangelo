from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.shared.gateways.terrablob_gateway import path_is_dir
from uber.ai.michelangelo.shared.errors.terrablob_error import (
    TerrablobError,
    TerrablobFailedPreconditionError,
)


class IsDirTest(TestCase):
    @patch("uber.ai.michelangelo.shared.gateways.terrablob_gateway.is_dir.list_terrablob_dir")
    def test_path_is_dir(self, mock_list_terrablob_dir):
        self.assertTrue(path_is_dir("test"))

        mock_list_terrablob_dir.side_effect = TerrablobFailedPreconditionError("test")
        self.assertFalse(path_is_dir("test"))

        mock_list_terrablob_dir.side_effect = TerrablobError("test")
        with self.assertRaises(TerrablobError):
            path_is_dir("test")

    def test_path_is_dir_invalid_arguments(self):
        with self.assertRaises(TypeError):
            path_is_dir("test", test="test")
