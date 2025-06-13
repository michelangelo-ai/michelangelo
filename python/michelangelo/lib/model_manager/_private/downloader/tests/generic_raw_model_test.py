import os
import tempfile
from unittest.mock import patch, call
from michelangelo._internal.testing.env import EnvTestCase
from michelangelo.lib.model_manager._private.downloader import download_generic_raw_model


def download_from_terrablob(
    src_path,  # noqa: ARG001
    dest_path,
    multipart=None,  # noqa: ARG001
    timeout=None,  # noqa: ARG001
    keepalive=None,  # noqa: ARG001
    source_entity=None,  # noqa: ARG001
    auth_mode=None,  # noqa: ARG001
):
    os.makedirs(dest_path, exist_ok=True)
    with open(os.path.join(dest_path, "file.txt"), "w") as f:
        f.write("file_content")


class GenericRawModelTest(EnvTestCase):
    @patch("michelangelo.lib.model_manager._private.downloader.generic_raw_model.download_from_terrablob", wraps=download_from_terrablob)
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.terrablob_utils.get_terrablob_auth_mode", return_value=None)
    @patch.dict(os.environ, {})
    def test_download_generic_raw_model_local_env(
        self,
        mock_get_terrablob_auth_mode,
        mock_list_terrablob_dir,
        mock_path_exists_revision_id,
        mock_get_latest_model_revision_id,
        mock_download_from_terrablob,
    ):
        mock_get_latest_model_revision_id.return_value = 0
        mock_path_exists_revision_id.return_value = True
        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")
            download_generic_raw_model("test_project", "model_name", None, dest_model_path)

            with open(os.path.join(dest_model_path, "file.txt")) as f:
                self.assertEqual(f.read(), "file_content")

        mock_list_terrablob_dir.assert_not_called()
        mock_get_terrablob_auth_mode.has_calls([call(), call()])
        mock_download_from_terrablob.assert_called_once_with(
            "/prod/michelangelo/raw_models/projects/test_project/models/model_name/revisions/0/main",
            dest_model_path,
            multipart=True,
            timeout=None,
            keepalive=True,
            source_entity=None,
            auth_mode=None,
        )

    @patch("michelangelo.lib.model_manager._private.downloader.generic_raw_model.download_from_terrablob", wraps=download_from_terrablob)
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.terrablob_utils.get_terrablob_auth_mode", return_value=None)
    @patch.dict(os.environ, {"UF_TASK_IMAGE": "image"})
    def test_download_generic_raw_model_remote_env(
        self,
        mock_get_terrablob_auth_mode,
        mock_list_terrablob_dir,
        mock_path_exists_revision_id,
        mock_get_latest_model_revision_id,
        mock_download_from_terrablob,
    ):
        mock_get_latest_model_revision_id.return_value = 0
        mock_path_exists_revision_id.return_value = True
        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")
            download_generic_raw_model("test_project", "model_name", None, dest_model_path)

            with open(os.path.join(dest_model_path, "file.txt")) as f:
                self.assertEqual(f.read(), "file_content")

        mock_list_terrablob_dir.assert_not_called()
        mock_get_terrablob_auth_mode.has_calls([call(), call()])
        mock_download_from_terrablob.assert_called_once_with(
            "/prod/michelangelo/raw_models/projects/test_project/models/model_name/revisions/0/main",
            dest_model_path,
            timeout=None,
            source_entity=None,
            auth_mode=None,
        )

    @patch("michelangelo.lib.model_manager._private.downloader.generic_raw_model.download_from_terrablob", wraps=download_from_terrablob)
    @patch("michelangelo.lib.model_manager._private.downloader.generic_raw_model.get_terrablob_auth_mode", return_value=None)
    @patch.dict(os.environ, {"UF_TASK_IMAGE": "image"})
    def test_download_generic_raw_model_with_model_revision(self, mock_get_terrablob_auth_model, mock_download_from_terrablob):
        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")
            download_generic_raw_model("test_project", "model_name", "1", dest_model_path)

            with open(os.path.join(dest_model_path, "file.txt")) as f:
                self.assertEqual(f.read(), "file_content")

        mock_get_terrablob_auth_model.assert_called_once()
        mock_download_from_terrablob.assert_called_once_with(
            "/prod/michelangelo/raw_models/projects/test_project/models/model_name/revisions/1/main",
            dest_model_path,
            timeout=None,
            source_entity=None,
            auth_mode=None,
        )

    @patch("michelangelo.lib.model_manager._private.downloader.generic_raw_model.download_from_terrablob", wraps=download_from_terrablob)
    @patch("michelangelo.lib.model_manager._private.downloader.generic_raw_model.get_latest_model_revision", return_value=None)
    def test_download_generic_raw_model_with_invalid_model_revision(self, mock_get_latest_model_revision, mock_download_from_terrablob):
        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")
            with self.assertRaises(ValueError):
                download_generic_raw_model("test_project", "model_name", None, dest_model_path)

        mock_get_latest_model_revision.assert_called_once()
        mock_download_from_terrablob.assert_not_called()
