import os
import shutil
import tempfile
from unittest.mock import patch
from uber.ai.michelangelo.shared.testing.env import EnvTestCase
from michelangelo.lib.model_manager._private.downloader import download_legacy_ma_model
from .utils.env import mimic_local_env, mimic_remote_env


def download_from_terrablob(
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


class LegacyMaModelTest(EnvTestCase):
    @patch("michelangelo.lib.model_manager._private.downloader.legacy_ma_model.download_from_terrablob", wraps=download_from_terrablob)
    def test_download_legacy_ma_model_local_env(self, mock_download_from_terrablob):
        mimic_local_env()
        project_id = "project_id"
        model_id = "model_id"

        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")

            download_legacy_ma_model(
                project_id,
                model_id,
                dest_model_path,
            )

            test_file = os.path.join(dest_model_path, "test_file")
            with open(test_file) as f:
                self.assertEqual(f.read(), "test")

            mock_download_from_terrablob.assert_called_once()
            args, kwargs = mock_download_from_terrablob.call_args
            self.assertEqual(args[0], "/prod/michelangelo/v1_projects/project_id/trained_models/model_id/sparkml_proto/project_id-v2.zip")
            self.assertTrue(args[1].endswith("model.zip"))
            self.assertTrue(kwargs["multipart"])
            self.assertTrue(kwargs["keepalive"])
            self.assertEqual(kwargs["timeout"], None)
            self.assertEqual(kwargs["source_entity"], None)

    @patch("michelangelo.lib.model_manager._private.downloader.legacy_ma_model.download_from_terrablob", wraps=download_from_terrablob)
    def test_download_legacy_ma_model_remote_env(self, mock_download_from_terrablob):
        mimic_remote_env()
        project_id = "project_id"
        model_id = "model_id"

        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")

            download_legacy_ma_model(
                project_id,
                model_id,
                dest_model_path,
            )

            test_file = os.path.join(dest_model_path, "test_file")
            with open(test_file) as f:
                self.assertEqual(f.read(), "test")

            mock_download_from_terrablob.assert_called_once()
            args, kwargs = mock_download_from_terrablob.call_args
            self.assertEqual(args[0], "/prod/michelangelo/v1_projects/project_id/trained_models/model_id/sparkml_proto/project_id-v2.zip")
            self.assertTrue(args[1].endswith("model.zip"))
            self.assertNotIn("multipart", kwargs)
            self.assertNotIn("keepalive", kwargs)
            self.assertEqual(kwargs["timeout"], None)
            self.assertEqual(kwargs["source_entity"], None)
