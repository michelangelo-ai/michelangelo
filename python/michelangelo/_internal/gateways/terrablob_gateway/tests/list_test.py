from unittest import TestCase
from unittest.mock import patch, call
from michelangelo._internal.gateways.terrablob_gateway import list_terrablob_dir
from michelangelo._internal.errors.terrablob_error import (
    TerrablobError,
    TerrablobPermissionError,
    TerrablobFileNotFoundError,
)


class ListTest(TestCase):
    def setUp(self):
        self.recursive_side_effect = [
            (
                b'{"result": [{"type": "dir", "name": "dir1"},{"type": "dir", "name": "dir2"},{"type": "blob", "name": "file1"}]}',
                b"",
                0,
            ),
            (
                b'{"result": [{"type": "blob", "name": "file2"}]}',
                b"",
                0,
            ),
            (
                b'{"result": [{"type": "dir", "name": "dir2subdir1"},{"type": "blob", "name": "file3"}]}',
                b"",
                0,
            ),
            (
                b'{"result": [{"type": "blob", "name": "file4"}]}',
                b"",
                0,
            ),
        ]

    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd"
    )
    def test_list_terrablob_dir(self, mock_execute_terrablob_cmd):
        mock_execute_terrablob_cmd.return_value = (
            b'{"result": [{"type": "dir", "name": "dir1"},{"type": "blob", "name": "file1"}]}',
            b"",
            0,
        )
        paths = list_terrablob_dir("test", output_relative_path=True)
        mock_execute_terrablob_cmd.assert_called_once_with(
            ["tb-cli", "ls", "test", "--json"],
        )
        self.assertEqual(paths, ["file1"])

        paths = list_terrablob_dir("test", output_relative_path=True, include_dir=True)
        mock_execute_terrablob_cmd.assert_called_with(
            ["tb-cli", "ls", "test", "--json"],
        )
        self.assertEqual(paths, ["dir1", "file1"])

        paths = list_terrablob_dir("test", include_dir=True)
        mock_execute_terrablob_cmd.assert_called_with(
            ["tb-cli", "ls", "test", "--json"],
        )
        self.assertEqual(paths, ["test/dir1", "test/file1"])

        paths = list_terrablob_dir("test")
        mock_execute_terrablob_cmd.assert_called_with(
            ["tb-cli", "ls", "test", "--json"],
        )
        self.assertEqual(paths, ["test/file1"])

        paths = list_terrablob_dir("test", is_staging=True)
        mock_execute_terrablob_cmd.assert_called_with(
            ["tb-cli", "ls", "test", "--json", "-s"],
        )
        self.assertEqual(paths, ["test/file1"])

        paths = list_terrablob_dir(
            "test",
            timeout="2h",
            source_entity="user",
            limit=10,
            auth_mode="auto",
        )
        mock_execute_terrablob_cmd.assert_called_with(
            [
                "tb-cli",
                "ls",
                "test",
                "--json",
                "--limit",
                "10",
                "-t",
                "2h",
                "-a",
                "user",
                "--auth-mode",
                "auto",
            ],
        )
        self.assertEqual(paths, ["test/file1"])

    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd"
    )
    def test_list_terrablob_dir_recursively(self, mock_execute_terrablob_cmd):
        mock_execute_terrablob_cmd.side_effect = self.recursive_side_effect
        paths = list_terrablob_dir("test", recursive=True)
        mock_execute_terrablob_cmd.assert_has_calls(
            [
                call(["tb-cli", "ls", "test", "--json"]),
                call(["tb-cli", "ls", "test/dir1", "--json"]),
                call(["tb-cli", "ls", "test/dir2", "--json"]),
                call(["tb-cli", "ls", "test/dir2/dir2subdir1", "--json"]),
            ],
        )
        self.assertEqual(
            paths,
            [
                "test/file1",
                "test/dir1/file2",
                "test/dir2/file3",
                "test/dir2/dir2subdir1/file4",
            ],
        )

    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd"
    )
    def test_list_terrablob_dir_recursively_output_relative_path(
        self, mock_execute_terrablob_cmd
    ):
        mock_execute_terrablob_cmd.side_effect = self.recursive_side_effect

        paths = list_terrablob_dir("test", recursive=True, output_relative_path=True)
        mock_execute_terrablob_cmd.assert_has_calls(
            [
                call(["tb-cli", "ls", "test", "--json"]),
                call(["tb-cli", "ls", "test/dir1", "--json"]),
                call(["tb-cli", "ls", "test/dir2", "--json"]),
                call(["tb-cli", "ls", "test/dir2/dir2subdir1", "--json"]),
            ],
        )
        self.assertEqual(
            paths,
            [
                "file1",
                "dir1/file2",
                "dir2/file3",
                "dir2/dir2subdir1/file4",
            ],
        )

    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd"
    )
    def test_list_terrablob_dir_recursively_include_dir(
        self, mock_execute_terrablob_cmd
    ):
        mock_execute_terrablob_cmd.side_effect = self.recursive_side_effect

        paths = list_terrablob_dir("test", recursive=True, include_dir=True)
        mock_execute_terrablob_cmd.assert_has_calls(
            [
                call(["tb-cli", "ls", "test", "--json"]),
                call(["tb-cli", "ls", "test/dir1", "--json"]),
                call(["tb-cli", "ls", "test/dir2", "--json"]),
                call(["tb-cli", "ls", "test/dir2/dir2subdir1", "--json"]),
            ],
        )

        self.assertEqual(
            paths,
            [
                "test/dir1",
                "test/dir2",
                "test/file1",
                "test/dir1/file2",
                "test/dir2/dir2subdir1",
                "test/dir2/file3",
                "test/dir2/dir2subdir1/file4",
            ],
        )

    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd"
    )
    def test_list_terrablob_dir_recursively_oupout_relative_path_include_dir(
        self, mock_execute_terrablob_cmd
    ):
        mock_execute_terrablob_cmd.side_effect = self.recursive_side_effect

        paths = list_terrablob_dir(
            "test", recursive=True, output_relative_path=True, include_dir=True
        )
        mock_execute_terrablob_cmd.assert_has_calls(
            [
                call(["tb-cli", "ls", "test", "--json"]),
                call(["tb-cli", "ls", "test/dir1", "--json"]),
                call(["tb-cli", "ls", "test/dir2", "--json"]),
                call(["tb-cli", "ls", "test/dir2/dir2subdir1", "--json"]),
            ],
        )

        self.assertEqual(
            paths,
            [
                "dir1",
                "dir2",
                "file1",
                "dir1/file2",
                "dir2/dir2subdir1",
                "dir2/file3",
                "dir2/dir2subdir1/file4",
            ],
        )

    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd"
    )
    def test_list_terrablob_dir_failure(self, mock_execute_terrablob_cmd):
        mock_execute_terrablob_cmd.return_value = (b"", b"error", 1)
        with self.assertRaises(TerrablobError):
            list_terrablob_dir("dir")

        mock_execute_terrablob_cmd.return_value = (
            b"",
            b"error code:permission-denied ...",
            1,
        )
        with self.assertRaises(TerrablobPermissionError):
            list_terrablob_dir("dir")

        mock_execute_terrablob_cmd.return_value = (b"", b"error code:not-found ...", 1)
        with self.assertRaises(TerrablobFileNotFoundError):
            list_terrablob_dir("dir")

    @patch("time.sleep")
    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd"
    )
    def test_list_terrablob_dir_retry(
        self, mock_execute_terrablob_cmd, mock_time_sleep
    ):
        mock_execute_terrablob_cmd.return_value = (
            b"",
            b"E0731 1:6:58.413706363  251714 backup_poller.cc:138]       "
            b"Run client channel backup poller: UNKNOWN:pollset_work "
            b'{created_time:"2024-07-31T01:6:58.413081963+00:0", children:[UNKNOWN:Bad file descriptor '
            b'{syscall:"epoll_wait", os_error:"Bad file descriptor", errno:9, '
            b'created_time:"2024-07-31T01:6:58.413001053+00:0"}]}\n',
            1,
        )
        mock_time_sleep.return_value = None
        with self.assertRaises(TerrablobError):
            list_terrablob_dir("dir")
        mock_time_sleep.assert_has_calls([call(8.0), call(16.0)])

    def test_list_terrablob_dir_invalid_arguments(self):
        with self.assertRaises(TypeError):
            list_terrablob_dir("dir", test="test")
