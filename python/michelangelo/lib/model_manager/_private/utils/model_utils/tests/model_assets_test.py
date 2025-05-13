import os
import yaml
import shutil
import tempfile
from unittest import TestCase
from unittest.mock import patch, call
from uber.ai.michelangelo.shared.errors.terrablob_error import TerrablobFileNotFoundError, TerrablobFailedPreconditionError
from uber.ai.michelangelo.sdk.model_manager.constants import StorageType, PackageType
from uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils import (
    download_assets_given_download_yaml,
    validate_deployable_download_yaml,
    validate_deployable_model_assets,
    convert_assets_to_download_yaml,
)


class ModelAssetsTest(TestCase):
    def setUp(self):
        self.download_yaml_content = {
            "assets": [
                {"x/y/a": "/root/a"},
                {"x/y/b": "/root/b"},
            ],
            "source_type": StorageType.LOCAL,
        }
        self.deployable_download_yaml_content = {
            "assets": [
                {"x/y/a": "/root/a"},
                {"x/y/b": "/root/b"},
            ],
            "source_type": StorageType.TERRABLOB,
        }

    def create_yaml_files(self, content: dict, target_dir: str):
        model_path = os.path.join(target_dir, "model")
        os.makedirs(model_path)

        with open(os.path.join(target_dir, "download.yaml"), "w") as f:
            yaml.dump(content, f)

        download_yaml_path = os.path.join(model_path, "download.yaml")

        with open(download_yaml_path, "w") as f:
            yaml.dump(content, f)

        with open(os.path.join(model_path, "file.txt"), "w") as f:
            f.write("content")

    def test_download_assets_given_download_yaml(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            source_dir = os.path.join(temp_dir, "source")
            target_dir = os.path.join(temp_dir, "target")
            os.makedirs(source_dir)
            os.makedirs(target_dir)
            with open(os.path.join(source_dir, "a"), "w") as f:
                f.write("a")
            with open(os.path.join(source_dir, "b"), "w") as f:
                f.write("b")

            yaml_content = {
                "assets": [
                    {"x/y/a": f"{source_dir}/a"},
                    {"x/y/b": f"{source_dir}/b"},
                ],
                "source_type": StorageType.LOCAL,
            }

            download_yaml_path = os.path.join(target_dir, "download.yaml")

            with open(download_yaml_path, "w") as f:
                yaml.dump(yaml_content, f)

            download_assets_given_download_yaml(download_yaml_path, target_dir)

            files = [os.path.join(dirpath, filename) for dirpath, _, filenames in os.walk(target_dir) for filename in filenames]
            files = sorted(files)

            self.assertEqual(files, [os.path.join(target_dir, "download.yaml"), os.path.join(target_dir, "x/y/a"), os.path.join(target_dir, "x/y/b")])

            with open(os.path.join(target_dir, "x/y/a")) as f:
                self.assertEqual(f.read(), "a")

            with open(os.path.join(target_dir, "x/y/b")) as f:
                self.assertEqual(f.read(), "b")

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_assets.get_blob_info")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_assets.get_terrablob_auth_mode")
    def test_validate_deployable_download_yaml_success(self, mock_get_terrablob_auth_mode, mock_get_blob_info):
        mock_get_terrablob_auth_mode.return_value = None
        with tempfile.TemporaryDirectory() as temp_dir:
            download_yaml_path = os.path.join(temp_dir, "download.yaml")
            with open(download_yaml_path, "w") as f:
                yaml.dump(self.deployable_download_yaml_content, f)

            validate_deployable_download_yaml(download_yaml_path)
            mock_get_blob_info.assert_has_calls(
                [
                    call("/root/a", timeout=None, source_entity=None, auth_mode=None),
                    call("/root/b", timeout=None, source_entity=None, auth_mode=None),
                ]
            )

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_assets.get_blob_info")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_assets.get_terrablob_auth_mode")
    def test_validate_deployable_download_yaml_incorrect_source_type(self, mock_get_terrablob_auth_mode, mock_get_blob_info):
        mock_get_terrablob_auth_mode.return_value = None
        with tempfile.TemporaryDirectory() as temp_dir:
            download_yaml_path = os.path.join(temp_dir, "download.yaml")
            with open(download_yaml_path, "w") as f:
                yaml.dump(self.download_yaml_content, f)

            with self.assertRaisesRegex(ValueError, "Remote assets must be stored in Terrablob in deployable package"):
                validate_deployable_download_yaml(download_yaml_path)

            mock_get_blob_info.assert_not_called()
            mock_get_terrablob_auth_mode.assert_not_called()

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_assets.get_blob_info")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_assets.get_terrablob_auth_mode")
    def test_validate_deployable_download_yaml_asset_not_exist(self, mock_get_terrablob_auth_mode, mock_get_blob_info):
        mock_get_terrablob_auth_mode.return_value = None
        mock_get_blob_info.side_effect = TerrablobFileNotFoundError("error")
        with tempfile.TemporaryDirectory() as temp_dir:
            download_yaml_path = os.path.join(temp_dir, "download.yaml")
            with open(download_yaml_path, "w") as f:
                yaml.dump(self.deployable_download_yaml_content, f)

            with self.assertRaisesRegex(ValueError, "Asset /root/a does not exist in Terrablob"):
                validate_deployable_download_yaml(download_yaml_path)

            mock_get_blob_info.assert_called_once_with("/root/a", timeout=None, source_entity=None, auth_mode=None)

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_assets.get_blob_info")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_assets.get_terrablob_auth_mode")
    def test_validate_deployable_model_assets_asset_is_dir(self, mock_get_terrablob_auth_mode, mock_get_blob_info):
        mock_get_terrablob_auth_mode.return_value = None
        mock_get_blob_info.side_effect = TerrablobFailedPreconditionError("error")
        with tempfile.TemporaryDirectory() as temp_dir:
            download_yaml_path = os.path.join(temp_dir, "download.yaml")
            with open(download_yaml_path, "w") as f:
                yaml.dump(self.deployable_download_yaml_content, f)

            with self.assertRaisesRegex(ValueError, "Asset /root/a is a directory, but expecting a file"):
                validate_deployable_download_yaml(download_yaml_path)

            mock_get_blob_info.assert_called_once_with("/root/a", timeout=None, source_entity=None, auth_mode=None)

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_assets.get_blob_info")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_assets.get_terrablob_auth_mode")
    def test_validate_deployable_model_assets_success(self, mock_get_terrablob_auth_mode, mock_get_blob_info):
        mock_get_terrablob_auth_mode.return_value = None
        with tempfile.TemporaryDirectory() as model_package:
            self.create_yaml_files(self.deployable_download_yaml_content, model_package)
            validate_deployable_model_assets(model_package)

        mock_get_blob_info.assert_has_calls(
            [
                call("/root/a", timeout=None, source_entity=None, auth_mode=None),
                call("/root/b", timeout=None, source_entity=None, auth_mode=None),
                call("/root/a", timeout=None, source_entity=None, auth_mode=None),
                call("/root/b", timeout=None, source_entity=None, auth_mode=None),
            ]
        )

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_assets.get_blob_info")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_assets.get_terrablob_auth_mode")
    def test_validate_deployable_model_assets_error(self, mock_get_terrablob_auth_mode, mock_get_blob_info):
        mock_get_terrablob_auth_mode.return_value = None
        with tempfile.TemporaryDirectory() as model_package:
            self.create_yaml_files(self.deployable_download_yaml_content, model_package)
            with patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_assets.get_blob_info") as mock_get_blob_info:
                mock_get_blob_info.side_effect = TerrablobFileNotFoundError("error")
                with self.assertRaisesRegex(RuntimeError, "Error validating remote assets in the deployable model package"):
                    validate_deployable_model_assets(model_package)

                mock_get_blob_info.assert_called_once_with("/root/a", timeout=None, source_entity=None, auth_mode=None)

    def test_convert_assets_to_download_yaml(self):
        with tempfile.TemporaryDirectory() as model_package:
            model_path = os.path.join(model_package, "0", "model")
            subsubdir1 = os.path.join(model_path, "subdir1", "subsubdir1")
            subdir2 = os.path.join(model_path, "subdir2")
            os.makedirs(subsubdir1)
            os.makedirs(subdir2)

            with open(os.path.join(model_package, "config.pbtxt"), "w") as f:
                f.write("name: model")

            with open(os.path.join(model_path, "a"), "w") as f:
                f.write("a")

            with open(os.path.join(subsubdir1, "b"), "w") as f:
                f.write("b")

            convert_assets_to_download_yaml(model_package, PackageType.TRITON, StorageType.TERRABLOB, "/root/")

            files = [
                os.path.relpath(os.path.join(dirpath, filename), model_package)
                for dirpath, _, filenames in os.walk(model_package)
                for filename in filenames
            ]
            files = sorted(files)

            self.assertEqual(files, ["0/download.yaml", "config.pbtxt"])

            with open(os.path.join(model_package, "0", "download.yaml")) as f:
                content = yaml.safe_load(f)

                print(content)
                self.assertEqual(
                    content,
                    {
                        "assets": [
                            {"model/a": "/root/a"},
                            {"model/subdir1/subsubdir1/b": "/root/subdir1/subsubdir1/b"},
                        ],
                        "source_type": StorageType.TERRABLOB,
                        "source_prefix": "/root/",
                    },
                )

    def test_convert_assets_to_download_yaml_skip_conversion(self):
        with tempfile.TemporaryDirectory() as model_package:
            model_path = os.path.join(model_package, "0", "model")
            os.makedirs(model_path)

            with open(os.path.join(model_path, "a"), "w") as f:
                f.write("a")

            with open(os.path.join(model_package, "config.pbtxt"), "w") as f:
                f.write("name: model")

            convert_assets_to_download_yaml(model_package, PackageType.RAW, StorageType.TERRABLOB, "/root/")

            self.assertTrue(os.path.isdir(model_path))

            with open(os.path.join(model_package, "0", "download.yaml"), "w") as f:
                yaml.dump({}, f)

            convert_assets_to_download_yaml(model_package, PackageType.TRITON, StorageType.TERRABLOB, "/root/")

            self.assertTrue(os.path.isdir(model_path))

            shutil.rmtree(model_path)

            convert_assets_to_download_yaml(model_package, PackageType.TRITON, StorageType.TERRABLOB, "/root/")

            with open(os.path.join(model_package, "0", "download.yaml")) as f:
                self.assertEqual(yaml.safe_load(f), {})
