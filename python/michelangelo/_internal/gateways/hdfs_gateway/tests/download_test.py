import subprocess
from unittest import TestCase
from unittest.mock import patch
from michelangelo._internal.errors.hdfs_error import HDFSError
from michelangelo._internal.gateways.hdfs_gateway import download_from_hdfs


class DownloadTest(TestCase):
    @patch("subprocess.Popen")
    def test_download_from_hdfs(self, mock_subprocess_Popen):
        mock_subprocess_Popen.return_value.communicate.return_value = (b"output", b"")
        mock_subprocess_Popen.return_value.returncode = 0
        download_from_hdfs("hdfs_model_dir", "model_path")
        mock_subprocess_Popen.assert_called_once_with(
            ["hdfs", "dfs", "-get", "hdfs_model_dir", "model_path"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )

    @patch("subprocess.Popen")
    def test_download_from_hdfs_error(self, mock_subprocess_Popen):
        mock_subprocess_Popen.return_value.communicate.return_value = (b"", b"error")
        mock_subprocess_Popen.return_value.returncode = 1

        with self.assertRaises(HDFSError):
            download_from_hdfs("hdfs_model_dir", "model_path")

        mock_subprocess_Popen.assert_called_once_with(
            ["hdfs", "dfs", "-get", "hdfs_model_dir", "model_path"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
