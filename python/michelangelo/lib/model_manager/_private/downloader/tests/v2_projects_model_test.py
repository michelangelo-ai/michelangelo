import os
import shutil
import tempfile
from unittest.mock import patch
from michelangelo._internal.testing.env import EnvTestCase
from michelangelo.lib.model_manager._private.utils.file_utils.gzip import gzip_compress
from michelangelo.lib.model_manager._private.downloader import download_v2_projects_model
from .utils.env import mimic_local_env, mimic_remote_env


def make_download_from_terrablob(project_name: str):
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


class V2ProjectsModelTest(EnvTestCase):
    def test_download_v2_projects_model_local_env(self):
        mimic_local_env()
        project_name = "test_project"
        model_name = "test_model"

        with (
            patch(
                "michelangelo.lib.model_manager._private.downloader.v2_projects_model.download_from_terrablob",
                wraps=make_download_from_terrablob(project_name),
            ) as mock_download_from_terrablob,
            tempfile.TemporaryDirectory() as temp_dir,
        ):
            dest_model_path = os.path.join(temp_dir, "model")
            download_v2_projects_model(
                project_name,
                model_name,
                "0",
                dest_model_path,
            )

            with open(os.path.join(dest_model_path, "test_file")) as f:
                self.assertEqual(f.read(), "test")

            mock_download_from_terrablob.assert_called_once()
            args, kwargs = mock_download_from_terrablob.call_args
            self.assertEqual(args[0], "/prod/michelangelo/v2_projects/test_project/trained_models/test_model/0/deploy_jar/model.jar.gz")
            self.assertTrue(args[1].endswith("model.jar.gz"))
            self.assertTrue(kwargs["multipart"])
            self.assertTrue(kwargs["keepalive"])
            self.assertEqual(kwargs["timeout"], None)
            self.assertEqual(kwargs["source_entity"], None)

    def test_download_v2_projects_model_remote_env(self):
        mimic_remote_env()
        project_name = "test_project"
        model_name = "test_model"

        with (
            patch(
                "michelangelo.lib.model_manager._private.downloader.v2_projects_model.download_from_terrablob",
                wraps=make_download_from_terrablob(project_name),
            ) as mock_download_from_terrablob,
            tempfile.TemporaryDirectory() as temp_dir,
        ):
            dest_model_path = os.path.join(temp_dir, "model")
            download_v2_projects_model(
                project_name,
                model_name,
                "0",
                dest_model_path,
            )

            with open(os.path.join(dest_model_path, "test_file")) as f:
                self.assertEqual(f.read(), "test")

            mock_download_from_terrablob.assert_called_once()
            args, kwargs = mock_download_from_terrablob.call_args
            self.assertEqual(args[0], "/prod/michelangelo/v2_projects/test_project/trained_models/test_model/0/deploy_jar/model.jar.gz")
            self.assertTrue(args[1].endswith("model.jar.gz"))
            self.assertNotIn("multipart", kwargs)
            self.assertNotIn("keepalive", kwargs)
            self.assertEqual(kwargs["timeout"], None)
            self.assertEqual(kwargs["source_entity"], None)
