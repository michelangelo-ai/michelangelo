from __future__ import annotations
from unittest import TestCase
from unittest.mock import patch, call
import tempfile
import os
from michelangelo._internal.gateways.terrablob_gateway import download_from_terrablob
from michelangelo._internal.errors.terrablob_error import (
    TerrablobError,
    TerrablobPermissionError,
    TerrablobFileNotFoundError,
)


def tb_cli_get(cmd: list[str]):
    des_path = cmd[3]
    with open(des_path, "w") as f:
        f.write("pass")
    return b"pass", b"", 0


class DownloadTest(TestCase):
    @patch("michelangelo._internal.gateways.terrablob_gateway.download.path_is_dir")
    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd",
        wraps=tb_cli_get,
    )
    def test_download_from_terrablob_success(
        self,
        mock_tb_cli_get,
        mock_path_is_dir,
    ):
        mock_path_is_dir.return_value = False

        with tempfile.TemporaryDirectory() as temp_dir:
            dest = os.path.join(temp_dir, "dest")
            result = download_from_terrablob("src", dest)

            mock_tb_cli_get.assert_called_once_with(["tb-cli", "get", "src", dest])
            self.assertEqual(result, {"exitcode": 0, "message": "pass", "error": ""})

            result = download_from_terrablob(
                "src", dest, timeout="2h", source_entity="user", auth_mode="auto"
            )
            mock_tb_cli_get.assert_called_with(
                [
                    "tb-cli",
                    "get",
                    "src",
                    dest,
                    "-t",
                    "2h",
                    "-a",
                    "user",
                    "--auth-mode",
                    "auto",
                ],
            )
            self.assertEqual(result, {"exitcode": 0, "message": "pass", "error": ""})

            result = download_from_terrablob("src", dest, is_staging=True)

            mock_tb_cli_get.assert_called_with(["tb-cli", "get", "src", dest, "-s"])
            self.assertEqual(result, {"exitcode": 0, "message": "pass", "error": ""})

            result = download_from_terrablob("src", dest, multipart=True, timeout="2h")
            mock_tb_cli_get.assert_called_with(
                ["tb-cli", "get", "src", dest, "-m", "-t", "2h"],
            )
            self.assertEqual(result, {"exitcode": 0, "message": "pass", "error": ""})

    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd"
    )
    @patch("michelangelo._internal.gateways.terrablob_gateway.download.path_is_dir")
    def test_download_from_terrablob_failure(
        self,
        mock_path_is_dir,
        mock_execute_terrablob_cmd,
    ):
        mock_path_is_dir.return_value = False
        mock_execute_terrablob_cmd.return_value = (b"", b"error", 1)
        with self.assertRaises(TerrablobError):
            download_from_terrablob("src", "dest")

        mock_execute_terrablob_cmd.return_value = (
            b"",
            b"error code:permission-denied ...",
            1,
        )
        with self.assertRaises(TerrablobPermissionError):
            download_from_terrablob("src", "dest")

        mock_execute_terrablob_cmd.return_value = (b"", b"error code:not-found ...", 1)
        with self.assertRaises(TerrablobFileNotFoundError):
            download_from_terrablob("src", "dest")

    @patch("michelangelo._internal.gateways.terrablob_gateway.download.path_is_dir")
    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.download.list_terrablob_dir"
    )
    def test_download_dir_from_terrablob(
        self,
        mock_list_terrablob_dir,
        mock_path_is_dir,
    ):
        mock_path_is_dir.return_value = True
        mock_list_terrablob_dir.return_value = [
            "src/file1",
            "src/sub/file2",
            "src/sub/file3",
            "src/file4",
        ]
        with patch(
            "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd",
            wraps=tb_cli_get,
        ) as mock_tb_cli_get:
            with tempfile.TemporaryDirectory() as temp_dir:
                dest = os.path.join(temp_dir, "dest")
                result = download_from_terrablob("src", dest)

                self.assertEqual(
                    result,
                    {
                        "exitcode": 0,
                        "message": f"Downloaded from Terrablob src to {dest}. File count: 4.",
                        "error": "",
                    },
                )

                mock_tb_cli_get.assert_has_calls(
                    [
                        call(
                            ["tb-cli", "get", "src/file1", os.path.join(dest, "file1")]
                        ),
                        call(
                            [
                                "tb-cli",
                                "get",
                                "src/sub/file2",
                                os.path.join(dest, "sub/file2"),
                            ]
                        ),
                        call(
                            [
                                "tb-cli",
                                "get",
                                "src/sub/file3",
                                os.path.join(dest, "sub/file3"),
                            ]
                        ),
                        call(
                            ["tb-cli", "get", "src/file4", os.path.join(dest, "file4")]
                        ),
                    ],
                    any_order=True,
                )

    @patch("michelangelo._internal.gateways.terrablob_gateway.download.path_is_dir")
    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.download.list_terrablob_dir"
    )
    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd",
        wraps=tb_cli_get,
    )
    def test_download_dir_from_terrablob_single_thread(
        self,
        mock_tb_cli_get,
        mock_list_terrablob_dir,
        mock_path_is_dir,
    ):
        mock_path_is_dir.return_value = True
        mock_list_terrablob_dir.return_value = [
            "src/file1",
            "src/sub/file2",
            "src/sub/file3",
            "src/file4",
        ]
        with tempfile.TemporaryDirectory() as temp_dir:
            dest = os.path.join(temp_dir, "dest")
            result = download_from_terrablob("src", dest, use_threads=False)

            self.assertEqual(
                result,
                {
                    "exitcode": 0,
                    "message": f"Downloaded from Terrablob src to {dest}. File count: 4.",
                    "error": "",
                },
            )

            mock_tb_cli_get.assert_has_calls(
                [
                    call(["tb-cli", "get", "src/file1", os.path.join(dest, "file1")]),
                    call(
                        [
                            "tb-cli",
                            "get",
                            "src/sub/file2",
                            os.path.join(dest, "sub/file2"),
                        ]
                    ),
                    call(
                        [
                            "tb-cli",
                            "get",
                            "src/sub/file3",
                            os.path.join(dest, "sub/file3"),
                        ]
                    ),
                    call(["tb-cli", "get", "src/file4", os.path.join(dest, "file4")]),
                ],
                any_order=True,
            )

    @patch("michelangelo._internal.gateways.terrablob_gateway.download.path_is_dir")
    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.download.list_terrablob_dir"
    )
    def test_download_dir_from_terrablob_staging(
        self,
        mock_list_terrablob_dir,
        mock_path_is_dir,
    ):
        mock_path_is_dir.return_value = True
        mock_list_terrablob_dir.return_value = [
            "src/file1",
            "src/sub/file2",
            "src/sub/file3",
            "src/file4",
        ]
        with patch(
            "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd",
            wraps=tb_cli_get,
        ) as mock_tb_cli_get:
            with tempfile.TemporaryDirectory() as temp_dir:
                dest = os.path.join(temp_dir, "dest")
                result = download_from_terrablob("src", dest, is_staging=True)

                self.assertEqual(
                    result,
                    {
                        "exitcode": 0,
                        "message": f"Downloaded from Terrablob src (staging) to {dest}. File count: 4.",
                        "error": "",
                    },
                )

                mock_tb_cli_get.assert_has_calls(
                    [
                        call(
                            [
                                "tb-cli",
                                "get",
                                "src/file1",
                                os.path.join(dest, "file1"),
                                "-s",
                            ]
                        ),
                        call(
                            [
                                "tb-cli",
                                "get",
                                "src/sub/file2",
                                os.path.join(dest, "sub/file2"),
                                "-s",
                            ]
                        ),
                        call(
                            [
                                "tb-cli",
                                "get",
                                "src/sub/file3",
                                os.path.join(dest, "sub/file3"),
                                "-s",
                            ]
                        ),
                        call(
                            [
                                "tb-cli",
                                "get",
                                "src/file4",
                                os.path.join(dest, "file4"),
                                "-s",
                            ]
                        ),
                    ],
                    any_order=True,
                )

    @patch("time.sleep")
    @patch(
        "michelangelo._internal.gateways.terrablob_gateway.common.cmd.execute_terrablob_cmd"
    )
    @patch("michelangelo._internal.gateways.terrablob_gateway.download.path_is_dir")
    def test_download_from_terrablob_retry(
        self,
        mock_path_is_dir,
        mock_download_from_terrablob,
        mock_time_sleep,
    ):
        mock_path_is_dir.return_value = False
        mock_download_from_terrablob.return_value = (
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
            download_from_terrablob("src", "dest")
        mock_time_sleep.assert_has_calls([call(8.0), call(16.0)])

    def test_download_from_terrablob_invalid_arguments(self):
        with self.assertRaises(TypeError):
            download_from_terrablob("src", "dest", a="a")
