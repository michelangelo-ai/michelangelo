import os
import yaml
import tempfile
from unittest.mock import patch
from uber.ai.michelangelo.shared.testing.env import EnvTestCase
from michelangelo.lib.model_manager.constants import PackageType, StorageType
from michelangelo.lib.model_manager._private.uploader import upload_generic_deployable_model
from michelangelo.lib.model_manager._private.constants import Placeholder
from .utils.env import mimic_local_env, mimic_remote_env


class GenericDeployableModelTest(EnvTestCase):
    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    def test_upload_generic_deployable_model_local_env(self, mock_upload_to_terrablob):
        mimic_local_env()
        project_name = "project_name"
        model_name = "model_name"
        model_revision = "0"
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            tb_path = upload_generic_deployable_model(
                model_path,
                project_name,
                model_name,
                model_revision,
                PackageType.TRITON,
            )

            expected_tb_path = (
                "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/triton/deploy_tar/model.tar"
            )

            mock_upload_to_terrablob.assert_called_once()

            args, kwargs = mock_upload_to_terrablob.call_args
            self.assertTrue(args[0].endswith("model.tar"))
            self.assertEqual(args[1], expected_tb_path)
            self.assertTrue(kwargs["multipart"])
            self.assertEqual(kwargs["concurrency"], 10)
            self.assertTrue(kwargs["keepalive"])
            self.assertIsNone(kwargs["timeout"])
            self.assertIsNone(kwargs["source_entity"])
            self.assertEqual(tb_path, expected_tb_path)

    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    def test_upload_generic_deployable_model_remote_env(self, mock_upload_to_terrablob):
        mimic_remote_env()
        project_name = "project_name"
        model_name = "model_name"
        model_revision = "0"
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            tb_path = upload_generic_deployable_model(
                model_path,
                project_name,
                model_name,
                model_revision,
                PackageType.TRITON,
            )

            expected_tb_path = (
                "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/triton/deploy_tar/model.tar"
            )

            mock_upload_to_terrablob.assert_called_once()

            args, kwargs = mock_upload_to_terrablob.call_args
            self.assertTrue(args[0].endswith("model.tar"))
            self.assertEqual(args[1], expected_tb_path)
            self.assertNotIn("multipart", kwargs)
            self.assertNotIn("concurrency", kwargs)
            self.assertNotIn("keepalive", kwargs)
            self.assertIsNone(kwargs["timeout"])
            self.assertIsNone(kwargs["source_entity"])
            self.assertEqual(tb_path, expected_tb_path)

    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    def test_upload_generic_deployable_model_with_replace_model_name_placeholder_and_params(self, mock_upload_to_terrablob):
        project_name = "project_name"
        model_name = "model_name"
        model_revision = "0"
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            with open(os.path.join(model_path, "config.pbtxt"), "w") as f:
                f.write(f"model_name: {Placeholder.MODEL_NAME}")

            tb_path = upload_generic_deployable_model(
                model_path,
                project_name,
                model_name,
                model_revision,
                PackageType.TRITON,
                timeout="2h",
                source_entity="source_entity",
            )

            expected_tb_path = (
                "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/triton/deploy_tar/model.tar"
            )

            mock_upload_to_terrablob.assert_called_once()

            args, kwargs = mock_upload_to_terrablob.call_args
            self.assertTrue(args[0].endswith("model.tar"))
            self.assertEqual(args[1], expected_tb_path)
            self.assertEqual(kwargs["timeout"], "2h")
            self.assertEqual(kwargs["source_entity"], "source_entity")

            self.assertEqual(tb_path, expected_tb_path)

            with open(os.path.join(model_path, "config.pbtxt")) as f:
                self.assertEqual(f.read(), "model_name: model_name")

    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_assets.get_blob_info")
    def test_upload_generic_deployable_model_with_download_yaml(self, mock_get_blob_info, mock_upload_to_terrablob):
        project_name = "project_name"
        model_name = "model_name"
        model_revision = "0"

        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            download_yaml_path = os.path.join(model_path, "download.yaml")
            with open(download_yaml_path, "w") as f:
                yaml.dump(
                    {
                        "assets": [{"a": "root/a"}, {"b": "root/b"}],
                        "source_type": StorageType.HDFS,
                        "source_prefix": "root/",
                    },
                    f,
                )

            tb_path = upload_generic_deployable_model(
                model_path,
                project_name,
                model_name,
                model_revision,
                PackageType.TRITON,
            )

            expected_tb_path = (
                "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/triton/deploy_tar/model.tar"
            )

            mock_upload_to_terrablob.assert_called_once()

            args, kwargs = mock_upload_to_terrablob.call_args
            self.assertTrue(args[0].endswith("model.tar"))
            self.assertEqual(args[1], expected_tb_path)
            self.assertIsNone(kwargs["timeout"])
            self.assertIsNone(kwargs["source_entity"])

            self.assertEqual(tb_path, expected_tb_path)

            with open(download_yaml_path) as f:
                content = yaml.safe_load(f)
                self.assertEqual(
                    content,
                    {
                        "assets": [
                            {"a": f"/prod/michelangelo/raw_models/projects/{project_name}/models/{model_name}/revisions/{model_revision}/main/model/a"},
                            {"b": f"/prod/michelangelo/raw_models/projects/{project_name}/models/{model_name}/revisions/{model_revision}/main/model/b"},
                        ],
                        "source_type": "terrablob",
                        "source_prefix": (
                            f"/prod/michelangelo/raw_models/projects/{project_name}/models/{model_name}/revisions/{model_revision}/main/model/"
                        ),
                    },
                )

        mock_get_blob_info.assert_called()

    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_assets.get_blob_info")
    def test_upload_generic_deployable_model_with_download_yaml_no_conversion(self, mock_get_blob_info, mock_upload_to_terrablob):
        project_name = "project_name"
        model_name = "model_name"
        model_revision = "0"

        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            download_yaml_path = os.path.join(model_path, "download.yaml")
            with open(download_yaml_path, "w") as f:
                yaml.dump(
                    {
                        "assets": [{"a": "root/a"}, {"b": "root/b"}],
                    },
                    f,
                )

            tb_path = upload_generic_deployable_model(
                model_path,
                project_name,
                model_name,
                model_revision,
                PackageType.TRITON,
            )

            expected_tb_path = (
                "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/triton/deploy_tar/model.tar"
            )

            mock_upload_to_terrablob.assert_called_once()

            args, kwargs = mock_upload_to_terrablob.call_args
            self.assertTrue(args[0].endswith("model.tar"))
            self.assertEqual(args[1], expected_tb_path)
            self.assertIsNone(kwargs["timeout"])
            self.assertIsNone(kwargs["source_entity"])

            self.assertEqual(tb_path, expected_tb_path)

            with open(download_yaml_path) as f:
                content = yaml.safe_load(f)
                self.assertEqual(
                    content,
                    {
                        "assets": [{"a": "root/a"}, {"b": "root/b"}],
                    },
                )

        mock_get_blob_info.assert_called()

    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.validate_deployable_model_assets")
    def test_upload_generic_deployable_model_validation_error(self, mock_validate_deployable_model_assets):
        mock_validate_deployable_model_assets.side_effect = RuntimeError("error")
        project_name = "project_name"
        model_name = "model_name"
        model_revision = "0"

        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            with self.assertRaisesRegex(RuntimeError, "error"):
                upload_generic_deployable_model(
                    model_path,
                    project_name,
                    model_name,
                    model_revision,
                    PackageType.TRITON,
                )

    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.validate_deployable_model_assets")
    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.convert_assets_to_download_yaml")
    def test_upload_generic_deployable_model_download_yaml_source_prefix(
        self, mock_convert_assets_to_download_yaml, mock_validate_deployable_model_assets, mock_upload_to_terrablob
    ):
        project_name = "project_name"
        model_name = "model_name"
        model_revision = "0"

        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)

            upload_generic_deployable_model(
                model_path,
                project_name,
                model_name,
                model_revision,
                PackageType.TRITON,
            )

            mock_convert_assets_to_download_yaml.assert_called_once()
            kwargs = mock_convert_assets_to_download_yaml.call_args.kwargs
            self.assertEqual(kwargs["source_type"], StorageType.TERRABLOB)
            self.assertEqual(kwargs["source_prefix"], "/prod/michelangelo/raw_models/projects/project_name/models/model_name/revisions/0/main/model/")
            mock_validate_deployable_model_assets.assert_called_once()
            mock_upload_to_terrablob.assert_called_once()
