import os
import tempfile
from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.sdk.model_manager.constants import StorageType
from uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils import download_assets


class DownloadTest(TestCase):
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.download.download_from_hdfs")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.download.download_from_terrablob")
    def test_download_assets_hdfs(self, mock_download_from_terrablob, mock_download_from_hdfs):
        download_assets("src_path", "des_path", StorageType.HDFS)
        mock_download_from_terrablob.assert_not_called()
        mock_download_from_hdfs.assert_called_once_with("src_path", "des_path")

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.download.download_from_hdfs")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.download.download_from_terrablob")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.download.get_terrablob_auth_mode")
    def test_download_assets_terrablob(self, mock_get_terrablob_auth_mode, mock_download_from_terrablob, mock_download_from_hdfs):
        mock_get_terrablob_auth_mode.return_value = None
        download_assets("src_path", "des_path", StorageType.TERRABLOB)
        mock_download_from_terrablob.assert_called_once_with("src_path", "des_path", source_entity=None, auth_mode=None, timeout=None)
        mock_download_from_hdfs.assert_not_called()

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.download.download_from_hdfs")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.download.download_from_terrablob")
    def test_download_assets_local(self, mock_download_from_terrablob, mock_download_from_hdfs):
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

            download_assets(src_path, des_path, StorageType.LOCAL)

            mock_download_from_terrablob.assert_not_called()
            mock_download_from_hdfs.assert_not_called()

            paths = []

            for dirpath, _, filenames in os.walk(des_path):
                paths.extend([os.path.join(dirpath, filename) for filename in filenames])
                if len(filenames) == 0:
                    paths.append(dirpath)

            paths = sorted(paths)

            self.assertEqual(
                paths,
                [
                    os.path.join(des_path, "file1.txt"),
                    os.path.join(des_path, "subdir1", "file2.txt"),
                    os.path.join(des_path, "subdir2"),
                ],
            )

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.download.download_from_hdfs")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.download.download_from_terrablob")
    def test_download_assets_local_single_file(self, mock_download_from_terrablob, mock_download_from_hdfs):
        with tempfile.TemporaryDirectory() as temp_dir:
            source = os.path.join(temp_dir, "source")
            with open(source, "w") as f:
                f.write("content")

            destination = os.path.join(temp_dir, "destination")

            download_assets(source, destination, StorageType.LOCAL)

            with open(destination) as f:
                self.assertEqual(f.read(), "content")

            mock_download_from_terrablob.assert_not_called()
            mock_download_from_hdfs.assert_not_called()

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.download.download_from_hdfs")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.download.download_from_terrablob")
    def test_download_assets_unknown_source_type(self, mock_download_from_terrablob, mock_download_from_hdfs):
        with tempfile.TemporaryDirectory() as des_path:
            download_assets("src_path", des_path, "unknown")
            mock_download_from_terrablob.assert_not_called()
            mock_download_from_hdfs.assert_not_called()
            self.assertEqual(os.listdir(des_path), [])
