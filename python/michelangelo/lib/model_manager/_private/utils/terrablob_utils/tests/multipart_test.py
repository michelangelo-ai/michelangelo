from unittest import TestCase
from unittest.mock import patch
from michelangelo.lib.model_manager._private.utils.terrablob_utils import (
    get_download_multipart_options,
    get_upload_multipart_options,
)


class MultipartTest(TestCase):
    @patch("michelangelo.lib.model_manager._private.utils.terrablob_utils.multipart.is_local")
    def test_get_download_multipart_options(self, mock_is_local):
        mock_is_local.return_value = False
        self.assertEqual(get_download_multipart_options(), {})
        mock_is_local.return_value = True
        self.assertEqual(get_download_multipart_options(), {"multipart": True, "keepalive": True})

    @patch("michelangelo.lib.model_manager._private.utils.terrablob_utils.multipart.is_local")
    def test_get_upload_multipart_options(self, mock_is_local):
        mock_is_local.return_value = False
        self.assertEqual(get_upload_multipart_options(), {})
        mock_is_local.return_value = True
        self.assertEqual(get_upload_multipart_options(), {"multipart": True, "concurrency": 10, "keepalive": True})
