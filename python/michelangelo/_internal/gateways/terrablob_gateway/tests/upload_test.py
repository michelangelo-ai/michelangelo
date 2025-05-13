from unittest import TestCase
from unittest.mock import patch, call
import os
import tempfile
from uber.ai.michelangelo.shared.gateways.terrablob_gateway import upload_to_terrablob
from uber.ai.michelangelo.shared.errors.terrablob_error import (
    TerrablobError,
    TerrablobPermissionError,
    TerrablobFileNotFoundError,
)


class UploadTest(TestCase):
    @patch("os.stat")
    @patch("os.path.exists")
    @patch("os.path.isfile")
    @patch("uber.ai.michelangelo.shared.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd")
    def test_upload_one_file_to_terrablob_success(
        self,
        mock_execute_terrablob_cmd,
        mock_isfile,
        mock_exists,
        mock_os_stat,
    ):
        mock_execute_terrablob_cmd.return_value = (b"pass", b"", 0)
        mock_os_stat.return_value.st_size = 100
        mock_exists.return_value = True
        mock_isfile.return_value = True

        result = upload_to_terrablob("src", "dest")

        mock_execute_terrablob_cmd.assert_called_once_with(
            ["tb-cli", "put", "src", "dest", "-p"],
        )
        self.assertEqual(result, {"exitcode": 0, "message": "pass", "error": ""})

        result = upload_to_terrablob("src", "dest", use_kraken=True, timeout="2h", source_entity="user", auth_mode="auto")

        mock_execute_terrablob_cmd.assert_called_with(
            ["tb-cli", "put", "src", "dest", "-p", "--kraken", "-t", "2h", "-a", "user", "--auth-mode", "auto"],
        )
        self.assertEqual(result, {"exitcode": 0, "message": "pass", "error": ""})

        result = upload_to_terrablob("src", "dest", is_staging=True)

        mock_execute_terrablob_cmd.assert_called_with(
            ["tb-cli", "put", "src", "dest", "-p", "-s"],
        )
        self.assertEqual(result, {"exitcode": 0, "message": "pass", "error": ""})

        result = upload_to_terrablob("src", "dest", use_kraken=True, multipart=True, concurrency=10)

        mock_execute_terrablob_cmd.assert_called_with(["tb-cli", "put", "src", "dest", "-p", "--kraken", "-m", "-C", "10"])
        self.assertEqual(result, {"exitcode": 0, "message": "pass", "error": ""})

    @patch("os.stat")
    @patch("os.path.exists")
    @patch("os.path.isfile")
    @patch("uber.ai.michelangelo.shared.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd")
    def test_upload_to_terrablob_failure(
        self,
        mock_execute_terrablob_cmd,
        mock_isfile,
        mock_exists,
        mock_os_stat,
    ):
        mock_execute_terrablob_cmd.return_value = (b"", b"error", 1)
        mock_os_stat.return_value.st_size = 100
        mock_exists.return_value = True
        mock_isfile.return_value = True

        with self.assertRaises(TerrablobError):
            upload_to_terrablob("src", "dest")

        mock_execute_terrablob_cmd.return_value = (b"", b"error code:permission-denied ...", 1)
        with self.assertRaises(TerrablobPermissionError):
            upload_to_terrablob("src", "dest")

        mock_execute_terrablob_cmd.return_value = (b"", b"error code:not-found ...", 1)
        with self.assertRaises(TerrablobFileNotFoundError):
            upload_to_terrablob("src", "dest")

    def test_upload_to_terrablob_file_not_found(self):
        with self.assertRaises(FileNotFoundError):
            upload_to_terrablob("src_path", "dest")

    @patch("uber.ai.michelangelo.shared.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd")
    def test_upload_dir_to_terrablob(self, mock_execute_terrablob_cmd):
        mock_execute_terrablob_cmd.return_value = (b"pass", b"", 0)

        with tempfile.TemporaryDirectory() as temp_dir:
            subdir1 = os.path.join(temp_dir, "subdir1")
            subdir2 = os.path.join(temp_dir, "subdir2")
            subsubdir1 = os.path.join(subdir1, "subsubdir1")
            os.makedirs(subdir1)
            os.makedirs(subdir2)
            os.makedirs(subsubdir1)

            with open(os.path.join(temp_dir, "file1"), "w") as f:
                f.write("file1")

            with open(os.path.join(temp_dir, "file2"), "w") as f:
                f.write("file2")

            with open(os.path.join(subdir1, "file3"), "w") as f:
                f.write("file3")

            with open(os.path.join(subdir2, "file4"), "w") as f:
                f.write("file4")

            with open(os.path.join(subsubdir1, "file5"), "w") as f:
                f.write("file5")

            with open(os.path.join(subsubdir1, "file6"), "w") as f:
                f.write("file6")

            result = upload_to_terrablob(temp_dir, "dest")

            self.assertEqual(
                result,
                {
                    "exitcode": 0,
                    "message": f"Uploaded directory {temp_dir} to Terrablob dest. File count: 6.",
                    "error": "",
                },
            )

            mock_execute_terrablob_cmd.assert_has_calls(
                [
                    call(["tb-cli", "put", os.path.join(temp_dir, "file1"), "dest/file1", "-p"]),
                    call(["tb-cli", "put", os.path.join(temp_dir, "file2"), "dest/file2", "-p"]),
                    call(["tb-cli", "put", os.path.join(subdir1, "file3"), "dest/subdir1/file3", "-p"]),
                    call(["tb-cli", "put", os.path.join(subdir2, "file4"), "dest/subdir2/file4", "-p"]),
                    call(["tb-cli", "put", os.path.join(subsubdir1, "file5"), "dest/subdir1/subsubdir1/file5", "-p"]),
                    call(["tb-cli", "put", os.path.join(subsubdir1, "file6"), "dest/subdir1/subsubdir1/file6", "-p"]),
                ],
                any_order=True,
            )

    @patch("uber.ai.michelangelo.shared.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd")
    def test_upload_to_terrablob_single_thread(self, mock_execute_terrablob_cmd):
        mock_execute_terrablob_cmd.return_value = (b"pass", b"", 0)

        with tempfile.TemporaryDirectory() as temp_dir:
            subdir1 = os.path.join(temp_dir, "subdir1")
            subdir2 = os.path.join(temp_dir, "subdir2")
            subsubdir1 = os.path.join(subdir1, "subsubdir1")
            os.makedirs(subdir1)
            os.makedirs(subdir2)
            os.makedirs(subsubdir1)

            with open(os.path.join(temp_dir, "file1"), "w") as f:
                f.write("file1")

            with open(os.path.join(temp_dir, "file2"), "w") as f:
                f.write("file2")

            with open(os.path.join(subdir1, "file3"), "w") as f:
                f.write("file3")

            with open(os.path.join(subdir2, "file4"), "w") as f:
                f.write("file4")

            with open(os.path.join(subsubdir1, "file5"), "w") as f:
                f.write("file5")

            with open(os.path.join(subsubdir1, "file6"), "w") as f:
                f.write("file6")

            result = upload_to_terrablob(temp_dir, "dest", use_threads=False)

            self.assertEqual(
                result,
                {
                    "exitcode": 0,
                    "message": f"Uploaded directory {temp_dir} to Terrablob dest. File count: 6.",
                    "error": "",
                },
            )

            mock_execute_terrablob_cmd.assert_has_calls(
                [
                    call(["tb-cli", "put", os.path.join(temp_dir, "file1"), "dest/file1", "-p"]),
                    call(["tb-cli", "put", os.path.join(temp_dir, "file2"), "dest/file2", "-p"]),
                    call(["tb-cli", "put", os.path.join(subdir1, "file3"), "dest/subdir1/file3", "-p"]),
                    call(["tb-cli", "put", os.path.join(subdir2, "file4"), "dest/subdir2/file4", "-p"]),
                    call(["tb-cli", "put", os.path.join(subsubdir1, "file5"), "dest/subdir1/subsubdir1/file5", "-p"]),
                    call(["tb-cli", "put", os.path.join(subsubdir1, "file6"), "dest/subdir1/subsubdir1/file6", "-p"]),
                ],
                any_order=True,
            )

    def test_upload_to_terrablob_invalid_arguments(self):
        with self.assertRaises(TypeError):
            upload_to_terrablob("src", "dest", test="test")
