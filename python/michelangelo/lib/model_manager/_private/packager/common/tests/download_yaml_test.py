import os
import tempfile
from unittest import TestCase
from unittest.mock import patch
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager._private.packager.common import generate_download_yaml_content


class DownloadYamlTest(TestCase):
    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.list.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.list.ls_files")
    def test_generate_download_yaml_content(
        self,
        mock_ls_files,
        mock_list_terrablob_dir,
    ):
        mock_list_terrablob_dir.return_value = ["a", "b"]
        content = generate_download_yaml_content("root")
        self.assertEqual(
            content,
            {
                "assets": [
                    {"a": "root/a"},
                    {"b": "root/b"},
                ],
                "source_type": "terrablob",
                "source_prefix": "root/",
            },
        )
        mock_ls_files.assert_not_called()

    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.list.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.list.ls_files")
    def test_generate_download_yaml_content_hdfs(
        self,
        mock_ls_files,
        mock_list_terrablob_dir,
    ):
        mock_ls_files.return_value = ["a", "b"]
        content = generate_download_yaml_content("root", model_path_source_type=StorageType.HDFS)
        self.assertEqual(
            content,
            {
                "assets": [
                    {"a": "root/a"},
                    {"b": "root/b"},
                ],
                "source_type": "hdfs",
                "source_prefix": "root/",
            },
        )
        mock_list_terrablob_dir.assert_not_called()
        mock_ls_files.assert_called_once_with("hdfs://root", recursive=True, output_relative_path=True)

        generate_download_yaml_content("hdfs://root", model_path_source_type=StorageType.HDFS)
        mock_list_terrablob_dir.assert_not_called()
        mock_ls_files.assert_called_with("hdfs://root", recursive=True, output_relative_path=True)

    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.list.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.list.ls_files")
    def test_generate_download_yaml_content_local(
        self,
        mock_ls_files,
        mock_list_terrablob_dir,
    ):
        with tempfile.TemporaryDirectory() as temp_dir:
            with open(os.path.join(temp_dir, "a"), "w") as f:
                f.write("a")
            with open(os.path.join(temp_dir, "b"), "w") as f:
                f.write("b")

            content = generate_download_yaml_content(temp_dir, model_path_source_type=StorageType.LOCAL)
            content["assets"] = sorted(content["assets"], key=lambda x: next(iter(x.keys())))
            self.assertEqual(
                content,
                {
                    "assets": [
                        {"a": f"{temp_dir}/a"},
                        {"b": f"{temp_dir}/b"},
                    ],
                    "source_type": "local",
                    "source_prefix": temp_dir + "/",
                },
            )

            mock_list_terrablob_dir.assert_not_called()
            mock_ls_files.assert_not_called()

    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.list.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.asset_utils.list.ls_files")
    def test_generate_download_yaml_content_with_other_params(
        self,
        mock_ls_files,
        mock_list_terrablob_dir,
    ):
        mock_list_terrablob_dir.return_value = ["a", "b"]
        content = generate_download_yaml_content("root", target_prefix="target/", source_prefix="source/", output_source_type="source_type")
        self.assertEqual(
            content,
            {
                "assets": [
                    {"target/a": "source/a"},
                    {"target/b": "source/b"},
                ],
                "source_type": "source_type",
                "source_prefix": "source/",
            },
        )
        mock_ls_files.assert_not_called()
