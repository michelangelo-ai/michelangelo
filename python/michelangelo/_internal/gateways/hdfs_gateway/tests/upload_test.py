import subprocess
from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.shared.errors.hdfs_error import HDFSError
from uber.ai.michelangelo.shared.gateways.hdfs_gateway import upload_to_hdfs


class UploadTest(TestCase):
    @patch("subprocess.Popen")
    def test_upload_to_hdfs(self, mock_subprocess_Popen):
        mock_subprocess_Popen.return_value.communicate.return_value = (b"output", b"")
        mock_subprocess_Popen.return_value.returncode = 0

        upload_to_hdfs("model_path", "hdfs_model_dir")

        mock_subprocess_Popen.assert_called_once_with(
            ["hdfs", "dfs", "-put", "-f", "model_path", "hdfs_model_dir"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )

    @patch("subprocess.Popen")
    def test_upload_to_hdfs_error(self, mock_subprocess_Popen):
        mock_subprocess_Popen.return_value.communicate.return_value = (b"", b"error")
        mock_subprocess_Popen.return_value.returncode = 1

        with self.assertRaises(HDFSError):
            upload_to_hdfs("model_path", "hdfs_model_dir")

        mock_subprocess_Popen.assert_called_once_with(
            ["hdfs", "dfs", "-put", "-f", "model_path", "hdfs_model_dir"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
