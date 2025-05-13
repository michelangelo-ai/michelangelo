import os
import tempfile
from unittest import TestCase
from unittest.mock import patch, call
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager._private.packager.common import download_model


class ModelDownloaderTest(TestCase):
    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.download.download_from_hdfs")
    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.download.download_from_terrablob")
    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.download.get_terrablob_auth_mode")
    def test_download_model(
        self,
        mock_get_terrablob_auth_mode,
        mock_download_from_terrablob,
        mock_download_from_hdfs,
    ):
        mock_get_terrablob_auth_mode.return_value = None
        dest_model_path = download_model("test_model_path")
        mock_download_from_hdfs.assert_called_once()
        self.assertEqual(mock_download_from_hdfs.call_args.args[0], "test_model_path")
        self.assertIn("/tmp", mock_download_from_hdfs.call_args.args[1])
        mock_download_from_terrablob.assert_not_called()
        self.assertIsNotNone(dest_model_path)

        dest_model_path = download_model(
            "test_model_path",
            model_path_source_type=StorageType.TERRABLOB,
        )
        mock_download_from_terrablob.assert_called_once()
        self.assertEqual(mock_download_from_terrablob.call_args.args[0], "test_model_path")
        self.assertIn("/tmp", mock_download_from_terrablob.call_args.args[1])
        self.assertEqual(mock_download_from_terrablob.call_args.kwargs["source_entity"], "michelangelo-apiserver")
        self.assertIsNotNone(dest_model_path)

        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = download_model(
                "test_model_path",
                dest_model_path=temp_dir,
            )
            mock_download_from_hdfs.assert_called()
            self.assertEqual(mock_download_from_hdfs.call_args.args[0], "test_model_path")
            self.assertEqual(temp_dir, mock_download_from_hdfs.call_args.args[1])
            self.assertEqual(dest_model_path, temp_dir)

    def test_download_model_no_source_type(self):
        dest_model_path = download_model("test_model_path", model_path_source_type=None)
        self.assertIsNone(dest_model_path)

    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.download.download_from_hdfs")
    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.download.download_from_terrablob")
    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.download.get_terrablob_auth_mode")
    def test_download_model_with_include(
        self,
        mock_get_terrablob_auth_mode,
        mock_download_from_terrablob,
        mock_download_from_hdfs,
    ):
        mock_get_terrablob_auth_mode.return_value = None

        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = download_model(
                "test_model_path",
                dest_model_path=temp_dir,
                include=["file1", "file2"],
            )

            mock_download_from_hdfs.assert_has_calls(
                [
                    call("test_model_path/file1", os.path.join(temp_dir, "file1")),
                    call("test_model_path/file2", os.path.join(temp_dir, "file2")),
                ],
            )

            self.assertIsNotNone(dest_model_path)

        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = download_model(
                "test_model_path",
                dest_model_path=temp_dir,
                model_path_source_type=StorageType.TERRABLOB,
                include=["file1", "file2"],
            )

            mock_download_from_terrablob.assert_has_calls(
                [
                    call("test_model_path/file1", os.path.join(temp_dir, "file1"), source_entity="michelangelo-apiserver", auth_mode=None, timeout="2h"),
                    call("test_model_path/file2", os.path.join(temp_dir, "file2"), source_entity="michelangelo-apiserver", auth_mode=None, timeout="2h"),
                ],
            )

            self.assertIsNotNone(dest_model_path)

    def test_download_model_from_local(self):
        with (
            tempfile.TemporaryDirectory() as src_path,
            tempfile.TemporaryDirectory() as des_path,
        ):
            with open(os.path.join(src_path, "file1.txt"), "w") as f:
                f.write("file1")
            os.makedirs(os.path.join(src_path, "subdir1"))
            os.makedirs(os.path.join(src_path, "subdir2"))
            with open(os.path.join(src_path, "subdir1", "file2.txt"), "w") as f:
                f.write("file2")

            dest_model_path = download_model(
                src_path,
                dest_model_path=des_path,
                model_path_source_type=StorageType.LOCAL,
            )

            paths = []

            for dirpath, _, filenames in os.walk(dest_model_path):
                paths.extend([os.path.join(dirpath, filename) for filename in filenames])
                if len(filenames) == 0:
                    paths.append(dirpath)

            paths = sorted(paths)

            self.assertEqual(
                paths,
                [
                    os.path.join(dest_model_path, "file1.txt"),
                    os.path.join(dest_model_path, "subdir1", "file2.txt"),
                    os.path.join(dest_model_path, "subdir2"),
                ],
            )

    def test_download_model_from_local_with_include(self):
        with (
            tempfile.TemporaryDirectory() as src_path,
            tempfile.TemporaryDirectory() as des_path,
        ):
            with open(os.path.join(src_path, "file1.txt"), "w") as f:
                f.write("file1")
            os.makedirs(os.path.join(src_path, "subdir1"))
            os.makedirs(os.path.join(src_path, "subdir2"))
            with open(os.path.join(src_path, "subdir1", "file2.txt"), "w") as f:
                f.write("file2")
            with open(os.path.join(src_path, "subdir1", "file3.txt"), "w") as f:
                f.write("file3")
            with open(os.path.join(src_path, "subdir2", "file4.txt"), "w") as f:
                f.write("file4")

            dest_model_path = download_model(
                src_path,
                dest_model_path=des_path,
                model_path_source_type=StorageType.LOCAL,
                include=["file1.txt", "subdir1"],
            )

            paths = []

            for dirpath, _, filenames in os.walk(dest_model_path):
                paths.extend([os.path.join(dirpath, filename) for filename in filenames])
                if len(filenames) == 0:
                    paths.append(dirpath)

            paths = sorted(paths)

            self.assertEqual(
                paths,
                [
                    os.path.join(dest_model_path, "file1.txt"),
                    os.path.join(dest_model_path, "subdir1", "file2.txt"),
                    os.path.join(dest_model_path, "subdir1", "file3.txt"),
                ],
            )

    def test_download_model_from_local_with_same_path(self):
        with tempfile.TemporaryDirectory() as src_path:
            with open(os.path.join(src_path, "file1.txt"), "w") as f:
                f.write("file1")
            os.makedirs(os.path.join(src_path, "subdir1"))
            os.makedirs(os.path.join(src_path, "subdir2"))
            with open(os.path.join(src_path, "subdir1", "file2.txt"), "w") as f:
                f.write("file2")

            dest_model_path = download_model(
                src_path,
                dest_model_path=src_path,
                model_path_source_type=StorageType.LOCAL,
            )

            paths = []

            for dirpath, _, filenames in os.walk(dest_model_path):
                paths.extend([os.path.join(dirpath, filename) for filename in filenames])
                if len(filenames) == 0:
                    paths.append(dirpath)

            paths = sorted(paths)

            self.assertEqual(
                paths,
                [
                    os.path.join(dest_model_path, "file1.txt"),
                    os.path.join(dest_model_path, "subdir1", "file2.txt"),
                    os.path.join(dest_model_path, "subdir2"),
                ],
            )

    def test_download_model_from_local_with_overwrites(self):
        with (
            tempfile.TemporaryDirectory() as src_path,
            tempfile.TemporaryDirectory() as des_path,
        ):
            with open(os.path.join(src_path, "file1.txt"), "w") as f:
                f.write("file1")
            os.makedirs(os.path.join(src_path, "subdir1"))
            os.makedirs(os.path.join(src_path, "subdir2"))
            with open(os.path.join(src_path, "subdir1", "file2.txt"), "w") as f:
                f.write("file2_overwrite")
            with open(os.path.join(src_path, "subdir1", "file3.txt"), "w") as f:
                f.write("file3")
            with open(os.path.join(src_path, "subdir2", "file4.txt"), "w") as f:
                f.write("file4")

            os.makedirs(os.path.join(des_path, "subdir1"))
            os.makedirs(os.path.join(des_path, "subdir2"))
            with open(os.path.join(des_path, "subdir1", "file2.txt"), "w") as f:
                f.write("file2")

            dest_model_path = download_model(
                src_path,
                dest_model_path=des_path,
                model_path_source_type=StorageType.LOCAL,
            )

            paths = []

            for dirpath, _, filenames in os.walk(dest_model_path):
                paths.extend([os.path.join(dirpath, filename) for filename in filenames])
                if len(filenames) == 0:
                    paths.append(dirpath)

            paths = sorted(paths)

            self.assertEqual(
                paths,
                [
                    os.path.join(dest_model_path, "file1.txt"),
                    os.path.join(dest_model_path, "subdir1", "file2.txt"),
                    os.path.join(dest_model_path, "subdir1", "file3.txt"),
                    os.path.join(dest_model_path, "subdir2", "file4.txt"),
                ],
            )

            with open(os.path.join(dest_model_path, "subdir1", "file2.txt")) as f:
                self.assertEqual(f.read(), "file2_overwrite")
