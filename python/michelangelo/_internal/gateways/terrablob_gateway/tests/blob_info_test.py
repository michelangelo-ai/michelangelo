from unittest import TestCase
from unittest.mock import patch
from michelangelo._internal.gateways.terrablob_gateway import get_blob_info
from michelangelo._internal.errors.terrablob_error import (
    TerrablobError,
    TerrablobPermissionError,
    TerrablobFileNotFoundError,
    TerrablobFailedPreconditionError,
)


class BlobInfoTest(TestCase):
    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd"
    )
    def test_get_blob_info(self, mock_execute_terrablob_cmd):
        mock_execute_terrablob_cmd.return_value = (
            b'{"result":{"size": 100}}',
            b"",
            0,
        )

        blob_info = get_blob_info("test")

        mock_execute_terrablob_cmd.assert_called_once_with(
            ["tb-cli", "blobInfo", "test", "--json"],
        )

        self.assertEqual(blob_info, {"size": 100})

    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd"
    )
    def test_get_blob_info_with_options(self, mock_execute_terrablob_cmd):
        mock_execute_terrablob_cmd.return_value = (
            b'{"result":{"size": 100}}',
            b"",
            0,
        )

        blob_info = get_blob_info("test")
        mock_execute_terrablob_cmd.assert_called_once_with(
            ["tb-cli", "blobInfo", "test", "--json"],
        )
        self.assertEqual(blob_info, {"size": 100})

        blob_info = get_blob_info(
            "test", timeout="1m", source_entity="user", auth_mode="auto"
        )

        mock_execute_terrablob_cmd.assert_called_with(
            [
                "tb-cli",
                "blobInfo",
                "test",
                "--json",
                "-t",
                "1m",
                "-a",
                "user",
                "--auth-mode",
                "auto",
            ],
        )

        self.assertEqual(blob_info, {"size": 100})

        blob_info = get_blob_info("test", is_staging=True)
        mock_execute_terrablob_cmd.assert_called_with(
            ["tb-cli", "blobInfo", "test", "--json", "-s"],
        )
        self.assertEqual(blob_info, {"size": 100})

    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd"
    )
    def test_get_blob_info_failure(self, mock_execute_terrablob_cmd):
        mock_execute_terrablob_cmd.return_value = (b"", b"error", 1)
        with self.assertRaises(TerrablobError):
            get_blob_info("test")

        mock_execute_terrablob_cmd.return_value = (
            b"",
            b"error code:permission-denied ...",
            1,
        )
        with self.assertRaises(TerrablobPermissionError):
            get_blob_info("test")

        mock_execute_terrablob_cmd.return_value = (b"", b"error code:not-found ...", 1)
        with self.assertRaises(TerrablobFileNotFoundError):
            get_blob_info("test")

        mock_execute_terrablob_cmd.return_value = (
            b"",
            b"error code:failed-precondition ...",
            1,
        )
        with self.assertRaises(TerrablobFailedPreconditionError):
            get_blob_info("test")

    def test_get_blob_info_invalid_arguments(self):
        with self.assertRaises(TypeError):
            get_blob_info("test", test="test")
