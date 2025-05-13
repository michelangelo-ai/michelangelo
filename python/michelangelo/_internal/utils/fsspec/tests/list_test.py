from unittest import TestCase
from unittest.mock import patch, MagicMock
from michelangelo._internal.utils.fsspec_utils import ls_files


class ListTest(TestCase):
    @patch("michelangelo._internal.utils.fsspec_utils.list.fsspec.core.url_to_fs")
    def test_ls_files(self, mock_url_to_fs):
        fs = MagicMock()
        fs.ls.return_value = [
            {"name": "foo/bar/dir1", "type": "directory"},
            {"name": "foo/bar/file1", "type": "file"},
            {"name": "foo/bar/file2", "type": "file"},
        ]

        mock_url_to_fs.return_value = (fs, "foo/bar")

        paths = ls_files("foo/bar")

        self.assertEqual(paths, ["foo/bar/file1", "foo/bar/file2"])

    @patch("michelangelo._internal.utils.fsspec_utils.list.fsspec.core.url_to_fs")
    def test_ls_files_output_relative_path(self, mock_url_to_fs):
        fs = MagicMock()
        fs.ls.return_value = [
            {"name": "foo/bar/dir1", "type": "directory"},
            {"name": "foo/bar/file1", "type": "file"},
            {"name": "foo/bar/file2", "type": "file"},
        ]

        mock_url_to_fs.return_value = (fs, "foo/bar")

        paths = ls_files("foo/bar", output_relative_path=True)

        self.assertEqual(paths, ["file1", "file2"])

    @patch("michelangelo._internal.utils.fsspec_utils.list.fsspec.core.url_to_fs")
    def test_ls_files_recursive(self, mock_url_to_fs):
        fs = MagicMock()
        fs.ls.side_effect = [
            [
                {"name": "foo/bar/dir1", "type": "directory"},
                {"name": "foo/bar/dir2", "type": "directory"},
                {"name": "foo/bar/file1", "type": "file"},
            ],
            [
                {"name": "foo/bar/dir1/file1", "type": "file"},
            ],
            [
                {"name": "foo/bar/dir2/file2", "type": "file"},
                {"name": "foo/bar/dir2/dir3", "type": "directory"},
            ],
            [
                {"name": "foo/bar/dir2/dir3/file3", "type": "file"},
            ],
        ]

        mock_url_to_fs.return_value = (fs, "foo/bar")

        paths = ls_files("foo/bar", recursive=True)

        self.assertEqual(paths, ["foo/bar/file1", "foo/bar/dir1/file1", "foo/bar/dir2/file2", "foo/bar/dir2/dir3/file3"])

    @patch("michelangelo._internal.utils.fsspec_utils.list.fsspec.core.url_to_fs")
    def test_ls_files_recursive_relative(self, mock_url_to_fs):
        fs = MagicMock()
        fs.ls.side_effect = [
            [
                {"name": "foo/bar/dir1", "type": "directory"},
                {"name": "foo/bar/dir2", "type": "directory"},
                {"name": "foo/bar/file1", "type": "file"},
            ],
            [
                {"name": "foo/bar/dir1/file1", "type": "file"},
            ],
            [
                {"name": "foo/bar/dir2/file2", "type": "file"},
                {"name": "foo/bar/dir2/dir3", "type": "directory"},
            ],
            [
                {"name": "foo/bar/dir2/dir3/file3", "type": "file"},
            ],
        ]

        mock_url_to_fs.return_value = (fs, "foo/bar")

        paths = ls_files("foo/bar", recursive=True, output_relative_path=True)

        self.assertEqual(paths, ["file1", "dir1/file1", "dir2/file2", "dir2/dir3/file3"])
