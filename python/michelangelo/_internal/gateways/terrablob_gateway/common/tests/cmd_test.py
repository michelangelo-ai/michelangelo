from unittest import TestCase
from unittest.mock import patch, call
from uber.ai.michelangelo.shared.gateways.terrablob_gateway.common import (
    construct_terrablob_cmd,
    execute_terrablob_cmd,
    execute_terrablob_cmd_with_exception,
    TerrablobOptions,
)

from uber.ai.michelangelo.shared.errors.terrablob_error import (
    TerrablobError,
    TerrablobConnectionTimeoutError,
    TerrablobConnectionError,
    TerrablobBadFileDescriptorError,
)


class CmdTest(TestCase):
    @patch("uber.ai.michelangelo.shared.gateways.terrablob_gateway.common.cmd.execute_cmd")
    def test_execute_terrablob_cmd(self, mock_execute_cmd):
        mock_execute_cmd.return_value = (b"output", b"error", 0)
        out, err, exitcode = execute_terrablob_cmd(["tb-cli", "ls", "test"])
        self.assertEqual(out, b"output")
        self.assertEqual(err, b"error")
        self.assertEqual(exitcode, 0)
        mock_execute_cmd.assert_called_once_with(
            ["tb-cli", "ls", "test"],
        )

    @patch("time.sleep")
    @patch("uber.ai.michelangelo.shared.gateways.terrablob_gateway.common.cmd.execute_cmd")
    def test_execute_terrablob_cmd_with_exception_raise_exception(self, mock_execute_cmd, mock_time_sleep):
        mock_execute_cmd.return_value = (b"output", b"error", 1)
        with self.assertRaises(TerrablobError):
            _out = execute_terrablob_cmd_with_exception(["tb-cli", "ls", "test"], "error")
        mock_execute_cmd.assert_called_once_with(["tb-cli", "ls", "test"])
        mock_time_sleep.assert_not_called()

    @patch("time.sleep")
    @patch("uber.ai.michelangelo.shared.gateways.terrablob_gateway.common.cmd.execute_cmd")
    def test_execute_terrablob_cmd_with_exception_raise_terrablob_connection_timeout_error(self, mock_execute_cmd, mock_time_sleep):
        mock_execute_cmd.return_value = (b"output", b"reset reason: connection timeout", 1)
        with self.assertRaises(TerrablobConnectionTimeoutError):
            _out = execute_terrablob_cmd_with_exception(["tb-cli", "ls", "test"], "error")
        mock_execute_cmd.assert_has_calls([call(["tb-cli", "ls", "test"])])
        mock_time_sleep.assert_has_calls([call(8.0), call(16.0)])

    @patch("time.sleep")
    @patch("uber.ai.michelangelo.shared.gateways.terrablob_gateway.common.cmd.execute_cmd")
    def test_execute_terrablob_cmd_with_exception_raise_terrablob_connection_error(self, mock_execute_cmd, mock_time_sleep):
        mock_execute_cmd.return_value = (b"output", b"code:unavailable message:closing transport due to: connection error", 1)
        with self.assertRaises(TerrablobConnectionError):
            _out = execute_terrablob_cmd_with_exception(["tb-cli", "ls", "test"], "error")
        mock_execute_cmd.assert_has_calls([call(["tb-cli", "ls", "test"])])
        mock_time_sleep.assert_has_calls([call(8.0), call(16.0)])

    @patch("time.sleep")
    @patch("uber.ai.michelangelo.shared.gateways.terrablob_gateway.common.cmd.execute_cmd")
    def test_execute_terrablob_cmd_with_exception_raise_terrablob_bad_file_descriptor_error(self, mock_execute_cmd, mock_time_sleep):
        mock_execute_cmd.return_value = (b"output", b'os_error:"Bad file descriptor"', 1)
        with self.assertRaises(TerrablobBadFileDescriptorError):
            _out = execute_terrablob_cmd_with_exception(["tb-cli", "ls", "test"], "error")
        mock_execute_cmd.assert_has_calls([call(["tb-cli", "ls", "test"])])
        mock_time_sleep.assert_has_calls([call(8.0), call(16.0)])

    @patch("uber.ai.michelangelo.shared.gateways.terrablob_gateway.common.cmd.execute_cmd")
    def test_execute_terrablob_cmd_with_exception_raise_unknown_error(self, mock_execute_cmd):
        mock_execute_cmd.return_value = (b"output", b"", 1)
        with self.assertRaises(TerrablobError):
            _out = execute_terrablob_cmd_with_exception(["tb-cli", "ls", "test"], "error")

    @patch("time.sleep")
    @patch("uber.ai.michelangelo.shared.gateways.terrablob_gateway.common.cmd.execute_cmd")
    def test_execute_terrablob_cmd_with_exception_no_exception(self, mock_execute_cmd, mock_time_sleep):
        mock_execute_cmd.return_value = (b"output", b"", 0)
        out = execute_terrablob_cmd_with_exception(["tb-cli", "ls", "test"], "error")
        self.assertEqual(out, "output")
        mock_execute_cmd.assert_has_calls([call(["tb-cli", "ls", "test"])])
        mock_time_sleep.assert_not_called()

    def test_construct_terrablob_cmd(self):
        cmd = ["tb-cli", "ls", "test"]
        options = TerrablobOptions(timeout="2h", source_entity="test", is_staging=True, auth_mode="auto", keepalive=True)
        res = construct_terrablob_cmd(cmd, options)
        self.assertEqual(res, ["tb-cli", "ls", "test", "-t", "2h", "-k", "-a", "test", "-s", "--auth-mode", "auto"])

        options = TerrablobOptions()
        res = construct_terrablob_cmd(cmd, options)
        self.assertEqual(res, ["tb-cli", "ls", "test"])
