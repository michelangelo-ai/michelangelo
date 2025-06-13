import os
import yaml
import tempfile
import shutil
from unittest.mock import patch
from michelangelo._internal.testing.env import EnvTestCase
from michelangelo.lib.model_manager.constants import PackageType, StorageType
from michelangelo.lib.model_manager._private.downloader import download_generic_deployable_model


def download_from_terrablob_simple(
    src_path,  # noqa: ARG001
    dest_path,
    multipart=None,  # noqa: ARG001
    timeout=None,  # noqa: ARG001
    keepalive=None,  # noqa: ARG001
    source_entity=None,  # noqa: ARG001
    auth_mode=None,  # noqa: ARG001
):
    with tempfile.TemporaryDirectory() as temp_dir:
        with open(os.path.join(temp_dir, "file.txt"), "w") as f:
            f.write("file_content")

        tar_name, _ = os.path.splitext(dest_path)
        shutil.make_archive(tar_name, "tar", temp_dir)


def make_download_from_terrablob_with_yaml(source: str):
    def download_from_terrablob(
        src_path,  # noqa: ARG001
        dest_path,
        multipart=None,  # noqa: ARG001
        timeout=None,  # noqa: ARG001
        keepalive=None,  # noqa: ARG001
        source_entity=None,  # noqa: ARG001
        auth_mode=None,  # noqa: ARG001
    ):
        with tempfile.TemporaryDirectory() as temp_dir:
            with open(os.path.join(temp_dir, "download.yaml"), "w") as f:
                yaml.dump(
                    {
                        "assets": [
                            {"a": f"{source}/a"},
                            {"b": f"{source}/b"},
                        ],
                        "source_type": StorageType.LOCAL,
                    },
                    f,
                )

            tar_name, _ = os.path.splitext(dest_path)
            shutil.make_archive(tar_name, "tar", temp_dir)

    return download_from_terrablob


class GenericDeployableModelTest(EnvTestCase):
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_terrablob_auth_mode")
    @patch(
        "michelangelo.lib.model_manager._private.downloader.generic_deployable_model.download_from_terrablob", wraps=download_from_terrablob_simple
    )
    @patch.dict(os.environ, {})
    def test_download_generic_deployable_model_local_env(
        self,
        mock_download_from_terrablob,
        mock_get_terrablob_auth_mode,
        mock_list_terrablob_dir,
        mock_path_exists_revision_id,
        mock_get_latest_model_revision_id,
    ):
        mock_get_terrablob_auth_mode.return_value = None
        mock_get_latest_model_revision_id.return_value = 0
        mock_path_exists_revision_id.return_value = False
        mock_list_terrablob_dir.return_value = ["0"]
        project_name = "test_project"
        model_name = "test_model"

        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")
            download_generic_deployable_model(
                project_name,
                model_name,
                "0",
                PackageType.TRITON,
                dest_model_path,
            )

            args, kwargs = mock_download_from_terrablob.call_args
            self.assertEqual(
                args[0],
                ("/prod/michelangelo/deployable_models/projects/test_project/models/test_model/revisions/0/package/triton/deploy_tar/model.tar"),
            )
            self.assertTrue(args[1].endswith(".tar"))
            self.assertEqual(kwargs["timeout"], None)
            self.assertEqual(kwargs["source_entity"], None)
            self.assertEqual(kwargs["auth_mode"], None)
            self.assertTrue(kwargs["multipart"])
            self.assertTrue(kwargs["keepalive"])

            mock_get_terrablob_auth_mode.assert_not_called()
            mock_get_latest_model_revision_id.assert_not_called()
            mock_path_exists_revision_id.assert_not_called()
            mock_list_terrablob_dir.assert_not_called()

            with open(os.path.join(dest_model_path, "file.txt")) as f:
                self.assertEqual(f.read(), "file_content")

    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_terrablob_auth_mode")
    @patch(
        "michelangelo.lib.model_manager._private.downloader.generic_deployable_model.download_from_terrablob", wraps=download_from_terrablob_simple
    )
    @patch.dict(os.environ, {"UF_TASK_IMAGE": "image"})
    def test_download_generic_deployable_model_remote_env(
        self,
        mock_download_from_terrablob,
        mock_get_terrablob_auth_mode,
        mock_list_terrablob_dir,
        mock_path_exists_revision_id,
        mock_get_latest_model_revision_id,
    ):
        mock_get_terrablob_auth_mode.return_value = None
        mock_get_latest_model_revision_id.return_value = 0
        mock_path_exists_revision_id.return_value = False
        mock_list_terrablob_dir.return_value = ["0"]
        project_name = "test_project"
        model_name = "test_model"

        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")
            download_generic_deployable_model(
                project_name,
                model_name,
                "0",
                PackageType.TRITON,
                dest_model_path,
            )

            args, kwargs = mock_download_from_terrablob.call_args
            self.assertEqual(
                args[0],
                ("/prod/michelangelo/deployable_models/projects/test_project/models/test_model/revisions/0/package/triton/deploy_tar/model.tar"),
            )
            self.assertTrue(args[1].endswith(".tar"))
            self.assertEqual(kwargs["timeout"], None)
            self.assertEqual(kwargs["source_entity"], None)
            self.assertEqual(kwargs["auth_mode"], None)
            self.assertNotIn("multipart", kwargs)
            self.assertNotIn("keepalive", kwargs)

            mock_get_terrablob_auth_mode.assert_not_called()
            mock_get_latest_model_revision_id.assert_not_called()
            mock_path_exists_revision_id.assert_not_called()
            mock_list_terrablob_dir.assert_not_called()

            with open(os.path.join(dest_model_path, "file.txt")) as f:
                self.assertEqual(f.read(), "file_content")

    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_terrablob_auth_mode")
    @patch(
        "michelangelo.lib.model_manager._private.downloader.generic_deployable_model.download_from_terrablob", wraps=download_from_terrablob_simple
    )
    def test_download_generic_deployable_model_with_no_model_revision(
        self,
        mock_download_from_terrablob,
        mock_get_terrablob_auth_mode,
        mock_list_terrablob_dir,
        mock_path_exists_revision_id,
        mock_get_latest_model_revision_id,
    ):
        mock_get_terrablob_auth_mode.return_value = None
        mock_get_latest_model_revision_id.return_value = 0
        mock_path_exists_revision_id.return_value = False
        mock_list_terrablob_dir.return_value = ["0"]
        project_name = "test_project"
        model_name = "test_model"

        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")
            download_generic_deployable_model(
                project_name,
                model_name,
                None,
                PackageType.TRITON,
                dest_model_path,
            )

            with open(os.path.join(dest_model_path, "file.txt")) as f:
                self.assertEqual(f.read(), "file_content")

            mock_path_exists_revision_id.assert_called_once_with(
                "/prod/michelangelo/deployable_models/projects/test_project/models/test_model/revisions/0/package/triton/deploy_tar/model.tar",
                timeout=None,
                source_entity=None,
                auth_mode=None,
            )

            mock_list_terrablob_dir.assert_called_once_with(
                "/prod/michelangelo/deployable_models/projects/test_project/models/test_model/revisions",
                output_relative_path=True,
                include_dir=True,
                timeout=None,
                source_entity=None,
                auth_mode=None,
            )
            mock_download_from_terrablob.assert_called_once()

    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_terrablob_auth_mode")
    @patch(
        "michelangelo.lib.model_manager._private.downloader.generic_deployable_model.download_from_terrablob", wraps=download_from_terrablob_simple
    )
    def test_download_generic_deployable_model_with_model_revision_as_empty_str(
        self,
        mock_download_from_terrablob,
        mock_get_terrablob_auth_mode,
        mock_list_terrablob_dir,
        mock_path_exists_revision_id,
        mock_get_latest_model_revision_id,
    ):
        mock_get_terrablob_auth_mode.return_value = None
        mock_get_latest_model_revision_id.return_value = 0
        mock_path_exists_revision_id.return_value = False
        mock_list_terrablob_dir.return_value = ["0"]
        project_name = "test_project"
        model_name = "test_model"

        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")
            download_generic_deployable_model(
                project_name,
                model_name,
                "",
                PackageType.TRITON,
                dest_model_path,
            )

            with open(os.path.join(dest_model_path, "file.txt")) as f:
                self.assertEqual(f.read(), "file_content")

        mock_download_from_terrablob.assert_called_once()

    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_terrablob_auth_mode")
    @patch(
        "michelangelo.lib.model_manager._private.downloader.generic_deployable_model.download_from_terrablob", wraps=download_from_terrablob_simple
    )
    def test_download_generic_deployable_model_with_no_model_revision_and_no_model_found(
        self,
        mock_download_from_terrablob,
        mock_get_terrablob_auth_mode,
        mock_list_terrablob_dir,
        mock_path_exists_revision_id,
        mock_get_latest_model_revision_id,
    ):
        mock_get_terrablob_auth_mode.return_value = None
        mock_get_latest_model_revision_id.return_value = 0
        mock_path_exists_revision_id.return_value = False
        mock_list_terrablob_dir.return_value = []
        project_name = "test_project"
        model_name = "test_model"

        with (
            tempfile.TemporaryDirectory() as temp_dir,
            self.assertRaisesRegex(ValueError, "No model revision found for the model"),
        ):
            dest_model_path = os.path.join(temp_dir, "model")
            download_generic_deployable_model(
                project_name,
                model_name,
                None,
                PackageType.TRITON,
                dest_model_path,
            )
        mock_download_from_terrablob.assert_not_called()

    @patch("michelangelo.lib.model_manager._private.downloader.generic_deployable_model.get_latest_uploaded_model_revision")
    def test_download_generic_deployable_model_retrieve_model_assets(self, mock_get_latest_uploaded_model_revision):
        mock_get_latest_uploaded_model_revision.return_value = "0"
        project_name = "test_project"
        model_name = "test_model"

        with tempfile.TemporaryDirectory() as temp_dir:
            source = os.path.join(temp_dir, "source")
            dest_model_path = os.path.join(temp_dir, "model_package", "model")
            os.makedirs(source)
            os.makedirs(os.path.join(dest_model_path, "model"))

            with open(os.path.join(source, "a"), "w") as f:
                f.write("a")
            with open(os.path.join(source, "b"), "w") as f:
                f.write("b")

            with patch(
                "michelangelo.lib.model_manager._private.downloader.generic_deployable_model.download_from_terrablob",
                wraps=make_download_from_terrablob_with_yaml(source),
            ) as mock_download_from_terrablob:
                download_generic_deployable_model(
                    project_name,
                    model_name,
                    "0",
                    PackageType.TRITON,
                    dest_model_path,
                )

                files = [
                    os.path.relpath(os.path.join(dirpath, filename), dest_model_path)
                    for dirpath, _, filenames in os.walk(dest_model_path)
                    for filename in filenames
                ]

                self.assertEqual(sorted(files), ["a", "b"])

                with open(os.path.join(dest_model_path, "a")) as f:
                    self.assertEqual(f.read(), "a")

                with open(os.path.join(dest_model_path, "b")) as f:
                    self.assertEqual(f.read(), "b")

                mock_download_from_terrablob.assert_called_once()
