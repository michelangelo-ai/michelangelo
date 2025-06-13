from unittest import TestCase
from unittest.mock import patch
import os
import shutil
import tempfile
from michelangelo.gen.api.v2.model_pb2 import Model
from michelangelo.lib.model_manager._private.utils.file_utils.gzip import gzip_compress
from michelangelo.lib.model_manager._private.downloader import download_spark_pipeline_model


def make_download_from_terrablob_v2_projects(project_name: str):
    def download_from_terrablob(
        src_path,  # noqa: ARG001
        dest_path,
        multipart=None,  # noqa: ARG001
        timeout=None,  # noqa: ARG001
        keepalive=None,  # noqa: ARG001
        source_entity=None,  # noqa: ARG001
    ):
        with tempfile.TemporaryDirectory() as temp_dir:
            model_files_dir = os.path.join(temp_dir, "model")

            if not os.path.exists(model_files_dir):
                os.makedirs(model_files_dir)

            model_binary_dir = os.path.join(temp_dir, "model_binary")
            if not os.path.exists(model_binary_dir):
                os.makedirs(model_binary_dir)

            with open(os.path.join(model_binary_dir, "test_file"), "w") as f:
                f.write("test")

            model_binary_zip_name = os.path.join(model_files_dir, project_name)
            shutil.make_archive(model_binary_zip_name, "zip", model_binary_dir)

            model_jar_path = os.path.join(temp_dir, "model.jar")
            shutil.make_archive(model_files_dir, "zip", model_files_dir)
            os.rename(f"{model_files_dir}.zip", model_jar_path)

            gzip_compress(model_jar_path, dest_path)

    return download_from_terrablob


def download_from_terrablob_v1_projects(
    src_path,  # noqa: ARG001
    dest_path,
    multipart=None,  # noqa: ARG001
    timeout=None,  # noqa: ARG001
    keepalive=None,  # noqa: ARG001
    source_entity=None,  # noqa: ARG001
):
    with tempfile.TemporaryDirectory() as temp_dir:
        model_binary_dir = os.path.join(temp_dir, "model_binary")
        if not os.path.exists(model_binary_dir):
            os.makedirs(model_binary_dir)

        with open(os.path.join(model_binary_dir, "test_file"), "w") as f:
            f.write("test")

        zip_name, _ = os.path.splitext(dest_path)
        shutil.make_archive(zip_name, "zip", model_binary_dir)


class SparkPipelineModelTest(TestCase):
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_terrablob_auth_mode")
    @patch("michelangelo.lib.model_manager._private.downloader.spark_pipeline_model.path_exists")
    def test_download_spark_pipeline_model_with_v2_projects(
        self,
        mock_path_exists,
        mock_get_terrablob_auth_mode,
        mock_list_terrablob_dir,
        mock_path_exists_revision_id,
        mock_get_latest_model_revision_id,
    ):
        mock_get_terrablob_auth_mode.return_value = None
        mock_get_latest_model_revision_id.return_value = 0
        mock_path_exists_revision_id.return_value = False
        mock_list_terrablob_dir.return_value = ["0"]
        mock_path_exists.return_value = True
        project_name = "test_project"
        model_name = "test_model"

        with (
            patch(
                "michelangelo.lib.model_manager._private.downloader.v2_projects_model.download_from_terrablob",
                wraps=make_download_from_terrablob_v2_projects(project_name),
            ) as mock_download_from_terrablob,
            tempfile.TemporaryDirectory() as temp_dir,
        ):
            dest_model_path = os.path.join(temp_dir, "model")
            download_spark_pipeline_model(
                project_name,
                model_name,
                "0",
                dest_model_path,
            )

            with open(os.path.join(dest_model_path, "test_file")) as f:
                self.assertEqual(f.read(), "test")

            mock_download_from_terrablob.assert_called_once()

    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_terrablob_auth_mode")
    @patch("michelangelo.lib.model_manager._private.downloader.spark_pipeline_model.path_exists")
    def test_download_spark_pipeline_model_with_v2_projects_with_no_revision(
        self,
        mock_path_exists,
        mock_get_terrablob_auth_mode,
        mock_list_terrablob_dir,
        mock_path_exists_revision_id,
        mock_get_latest_model_revision_id,
    ):
        mock_get_terrablob_auth_mode.return_value = None
        mock_get_latest_model_revision_id.return_value = 0
        mock_path_exists_revision_id.return_value = False
        mock_list_terrablob_dir.return_value = ["0"]
        mock_path_exists.return_value = True
        project_name = "test_project"
        model_name = "test_model"

        with (
            patch(
                "michelangelo.lib.model_manager._private.downloader.v2_projects_model.download_from_terrablob",
                wraps=make_download_from_terrablob_v2_projects(project_name),
            ) as mock_download_from_terrablob,
            tempfile.TemporaryDirectory() as temp_dir,
        ):
            dest_model_path = os.path.join(temp_dir, "model")
            download_spark_pipeline_model(
                project_name,
                model_name,
                None,
                dest_model_path,
            )

            with open(os.path.join(dest_model_path, "test_file")) as f:
                self.assertEqual(f.read(), "test")

            mock_path_exists_revision_id.assert_called_once_with(
                "/prod/michelangelo/v2_projects/test_project/trained_models/test_model/0/deploy_jar/model.jar.gz",
                timeout=None,
                source_entity=None,
                auth_mode=None,
            )

            mock_list_terrablob_dir.assert_called_once_with(
                "/prod/michelangelo/v2_projects/test_project/trained_models/test_model",
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
    @patch("michelangelo.lib.model_manager._private.downloader.spark_pipeline_model.path_exists")
    def test_download_spark_pipeline_model_with_v2_projects_with_empty_revision(
        self,
        mock_path_exists,
        mock_get_terrablob_auth_mode,
        mock_list_terrablob_dir,
        mock_path_exists_revision_id,
        mock_get_latest_model_revision_id,
    ):
        mock_get_terrablob_auth_mode.return_value = None
        mock_get_latest_model_revision_id.return_value = 0
        mock_path_exists_revision_id.return_value = False
        mock_list_terrablob_dir.return_value = ["0"]
        mock_path_exists.return_value = True
        project_name = "test_project"
        model_name = "test_model"

        with (
            patch(
                "michelangelo.lib.model_manager._private.downloader.v2_projects_model.download_from_terrablob",
                wraps=make_download_from_terrablob_v2_projects(project_name),
            ) as mock_download_from_terrablob,
            tempfile.TemporaryDirectory() as temp_dir,
        ):
            dest_model_path = os.path.join(temp_dir, "model")
            download_spark_pipeline_model(
                project_name,
                model_name,
                "",
                dest_model_path,
            )

            with open(os.path.join(dest_model_path, "test_file")) as f:
                self.assertEqual(f.read(), "test")

        mock_download_from_terrablob.assert_called_once()
