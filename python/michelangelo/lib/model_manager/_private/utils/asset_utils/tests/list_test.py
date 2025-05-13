import os
import tempfile
from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.sdk.model_manager.constants import StorageType
from uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils import list_assets


class ListTest(TestCase):
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.list.list_terrablob_dir")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.list.ls_files")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.list.get_terrablob_auth_mode")
    def test_list_assets_terrablob(self, mock_get_terrablob_auth_mode, mock_ls_files, mock_list_terrablob_dir):
        mock_list_terrablob_dir.return_value = ["a", "b"]
        mock_get_terrablob_auth_mode.return_value = None
        assets = list_assets("root", StorageType.TERRABLOB)
        self.assertEqual(assets, ["a", "b"])
        mock_ls_files.assert_not_called()
        mock_list_terrablob_dir.assert_called_once_with(
            "root",
            recursive=True,
            output_relative_path=True,
            source_entity="michelangelo-apiserver",
            auth_mode=None,
        )

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.list.list_terrablob_dir")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.list.ls_files")
    def test_list_assets_hdfs(self, mock_ls_files, mock_list_terrablob_dir):
        mock_ls_files.return_value = ["a", "b"]
        assets = list_assets("root", StorageType.HDFS)
        self.assertEqual(assets, ["a", "b"])
        mock_list_terrablob_dir.assert_not_called()
        mock_ls_files.assert_called_once_with("hdfs://root", recursive=True, output_relative_path=True)

        assets = list_assets("hdfs://root", StorageType.HDFS)
        self.assertEqual(assets, ["a", "b"])
        mock_list_terrablob_dir.assert_not_called()
        mock_ls_files.assert_called_with("hdfs://root", recursive=True, output_relative_path=True)

        assets = list_assets("gcs://root", StorageType.HDFS)
        self.assertEqual(assets, ["a", "b"])
        mock_list_terrablob_dir.assert_not_called()
        mock_ls_files.assert_called_with("gcs://root", recursive=True, output_relative_path=True)

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.list.list_terrablob_dir")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.asset_utils.list.ls_files")
    def test_list_assets_local(self, mock_ls_files, mock_list_terrablob_dir):
        with tempfile.TemporaryDirectory() as root:
            with open(os.path.join(root, "file1.txt"), "w") as f:
                f.write("file1")
            os.makedirs(os.path.join(root, "subdir1"))
            os.makedirs(os.path.join(root, "subdir2"))
            with open(os.path.join(root, "subdir1", "file2.txt"), "w") as f:
                f.write("file2")

            assets = list_assets(root, StorageType.LOCAL)

            mock_list_terrablob_dir.assert_not_called()
            mock_ls_files.assert_not_called()

            self.assertEqual(
                sorted(assets),
                [
                    "file1.txt",
                    os.path.join("subdir1", "file2.txt"),
                ],
            )

    def test_list_assets_unknown_source_type(self):
        assets = list_assets("root", "unknown")
        self.assertEqual(assets, None)
