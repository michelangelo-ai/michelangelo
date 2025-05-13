import subprocess
from unittest import TestCase
from unittest.mock import patch
from michelangelo._internal.errors.hdfs_error import HDFSError
from michelangelo._internal.gateways.hdfs_gateway import create_dir_in_hdfs


class CreateDirTest(TestCase):
    @patch("subprocess.Popen")
    def test_create_dir_in_hdfs(self, mock_subprocess_Popen):
        mock_subprocess_Popen.return_value.communicate.return_value = (b"output", b"")
        mock_subprocess_Popen.return_value.returncode = 0

        create_dir_in_hdfs("hdfs_model_dir")

        mock_subprocess_Popen.assert_called_once_with(
            ["hdfs", "dfs", "-mkdir", "-p", "hdfs_model_dir"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )

    @patch("subprocess.Popen")
    def test_create_dir_in_hdfs_error(self, mock_subprocess_Popen):
        mock_subprocess_Popen.return_value.communicate.return_value = (b"", b"error")
        mock_subprocess_Popen.return_value.returncode = 1

        with self.assertRaises(HDFSError):
            create_dir_in_hdfs("hdfs_model_dir")

        mock_subprocess_Popen.assert_called_once_with(
            ["hdfs", "dfs", "-mkdir", "-p", "hdfs_model_dir"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
