from unittest import TestCase
from unittest.mock import patch
import os
import shutil
import tempfile
from uber.gen.code_uber_internal.uberai.michelangelo.api.v2beta1.model_proto import Model
from uber.ai.michelangelo.sdk.model_manager._private.utils.file_utils.gzip import gzip_compress
from uber.ai.michelangelo.sdk.model_manager._private.downloader import download_spark_pipeline_model


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
            os.system(f"jar cfvM {model_jar_path} -C {model_files_dir} .")

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
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.get_terrablob_auth_mode")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.downloader.spark_pipeline_model.path_exists")
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
                "uber.ai.michelangelo.sdk.model_manager._private.downloader.v2_projects_model.download_from_terrablob",
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

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.get_terrablob_auth_mode")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.downloader.spark_pipeline_model.path_exists")
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
                "uber.ai.michelangelo.sdk.model_manager._private.downloader.v2_projects_model.download_from_terrablob",
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

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.get_terrablob_auth_mode")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.downloader.spark_pipeline_model.path_exists")
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
                "uber.ai.michelangelo.sdk.model_manager._private.downloader.v2_projects_model.download_from_terrablob",
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

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.downloader.spark_pipeline_model.path_exists")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.api_client.APIClient.ModelService.get_model")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.downloader.legacy_ma_model.download_from_terrablob", wraps=download_from_terrablob_v1_projects)
    def test_download_spark_pipeline_model_with_legacy_ma(
        self,
        mock_download_from_terrablob,
        mock_get_model,
        mock_path_exists,
        mock_list_terrablob_dir,
        mock_path_exists_revision_id,
        mock_get_latest_model_revision_id,
    ):
        mock_get_latest_model_revision_id.return_value = 0
        mock_path_exists_revision_id.return_value = False
        mock_list_terrablob_dir.return_value = []
        mock_path_exists.return_value = False
        model_crd = Model()
        model_crd.spec.legacy_model_spec.project_id = "project_id"
        model_crd.spec.legacy_model_spec.tm_model_id = "tm_model_id"
        mock_get_model.return_value = model_crd

        project_name = "test_project"
        model_name = "test_model"

        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")

            download_spark_pipeline_model(
                project_name,
                model_name,
                "0",
                dest_model_path,
            )

            mock_download_from_terrablob.assert_called_once()

            args = mock_download_from_terrablob.call_args.args
            self.assertEqual(
                args[0],
                "/prod/michelangelo/v1_projects/project_id/trained_models/tm_model_id/sparkml_proto/project_id-v2.zip",
            )

            with open(os.path.join(dest_model_path, "test_file")) as f:
                self.assertEqual(f.read(), "test")

        mock_path_exists.assert_called_once_with(
            "/prod/michelangelo/v2_projects/test_project/trained_models/test_model/0/deploy_jar/model.jar.gz",
            timeout=None,
            source_entity=None,
        )

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.downloader.spark_pipeline_model.path_exists")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.api_client.APIClient.ModelService.get_model")
    @patch("uber.ai.michelangelo.sdk.model_manager._private.downloader.legacy_ma_model.download_from_terrablob", wraps=download_from_terrablob_v1_projects)
    def test_download_spark_pipeline_model_with_legacy_ma_no_revision(
        self,
        mock_download_from_terrablob,
        mock_get_model,
        mock_path_exists,
        mock_list_terrablob_dir,
        mock_path_exists_revision_id,
        mock_get_latest_model_revision_id,
    ):
        mock_get_latest_model_revision_id.return_value = 0
        mock_path_exists_revision_id.return_value = False
        mock_list_terrablob_dir.return_value = []
        mock_path_exists.return_value = False
        model_crd = Model()
        model_crd.spec.legacy_model_spec.project_id = "project_id"
        model_crd.spec.legacy_model_spec.tm_model_id = "tm_model_id"
        mock_get_model.return_value = model_crd

        project_name = "test_project"
        model_name = "test_model"

        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")
            download_spark_pipeline_model(
                project_name,
                model_name,
                None,
                dest_model_path,
            )

            with open(os.path.join(dest_model_path, "test_file")) as f:
                self.assertEqual(f.read(), "test")

        mock_download_from_terrablob.assert_called_once()
