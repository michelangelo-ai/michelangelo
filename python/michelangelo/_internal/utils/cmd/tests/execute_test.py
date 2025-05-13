from unittest import TestCase
from unittest.mock import patch
import subprocess
from michelangelo._internal.utils.cmd import execute_cmd


class ExecuteTest(TestCase):
    @patch("subprocess.Popen")
    def test_execute_cmd(self, mock_subprocess_Popen):
        mock_subprocess_Popen.return_value.communicate.return_value = (b"output", b"error")
        mock_subprocess_Popen.return_value.returncode = 0
        out, err, exitcode = execute_cmd(["ls", "-la"])
        self.assertEqual(out, b"output")
        self.assertEqual(err, b"error")
        self.assertEqual(exitcode, 0)
        mock_subprocess_Popen.assert_called_once_with(
            ["ls", "-la"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
