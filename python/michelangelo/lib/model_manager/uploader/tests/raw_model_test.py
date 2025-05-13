from unittest.mock import patch
from michelangelo._internal.testing.env import EnvTestCase
from michelangelo.lib.model_manager.uploader import upload_raw_model
from .utils.env import mimic_local_env, mimic_remote_env


class RawModelTest(EnvTestCase):
    @patch("michelangelo.lib.model_manager.uploader.raw_model.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager.uploader.raw_model.upload_to_terrablob")
    def test_upload_raw_model_local_env(self, mock_upload_to_terrablob, mock_get_latest_model_revision_id):
        mimic_local_env()
        mock_get_latest_model_revision_id.return_value = 0

        tb_model_path = upload_raw_model(
            "model_path",
            "test_project",
            "test_model",
        )

        mock_upload_to_terrablob.assert_called_once_with(
            "model_path",
            "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/1/main",
            multipart=True,
            concurrency=10,
            timeout=None,
            keepalive=True,
            source_entity="michelangelo-apiserver",
            auth_mode=None,
        )
        mock_get_latest_model_revision_id.assert_called_once_with(
            "test_project",
            "test_model",
        )
        self.assertEqual(tb_model_path, "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/1/main")

    @patch("michelangelo.lib.model_manager.uploader.raw_model.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager.uploader.raw_model.upload_to_terrablob")
    def test_upload_raw_model_remote_env(self, mock_upload_to_terrablob, mock_get_latest_model_revision_id):
        mimic_remote_env()
        mock_get_latest_model_revision_id.return_value = 0

        tb_model_path = upload_raw_model(
            "model_path",
            "test_project",
            "test_model",
        )

        mock_upload_to_terrablob.assert_called_once_with(
            "model_path",
            "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/1/main",
            timeout=None,
            source_entity="michelangelo-apiserver",
            auth_mode=None,
        )
        mock_get_latest_model_revision_id.assert_called_once_with(
            "test_project",
            "test_model",
        )
        self.assertEqual(tb_model_path, "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/1/main")

    @patch("michelangelo.lib.model_manager.uploader.raw_model.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager.uploader.raw_model.upload_to_terrablob")
    def test_upload_raw_model_with_revision_id(self, mock_upload_to_terrablob, mock_get_latest_model_revision_id):
        tb_model_path = upload_raw_model(
            "model_path",
            "test_project",
            "test_model",
            revision_id=0,
        )

        mock_upload_to_terrablob.assert_called_once_with(
            "model_path",
            "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/0/main",
            timeout=None,
            source_entity="michelangelo-apiserver",
            auth_mode=None,
        )

        mock_get_latest_model_revision_id.assert_not_called()
        self.assertEqual(tb_model_path, "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/0/main")

    @patch("michelangelo.lib.model_manager.uploader.raw_model.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager.uploader.raw_model.upload_to_terrablob")
    def test_upload_raw_model_with_source_entity(self, mock_upload_to_terrablob, mock_get_latest_model_revision_id):
        mock_get_latest_model_revision_id.return_value = 0

        tb_model_path = upload_raw_model(
            "model_path",
            "test_project",
            "test_model",
            source_entity="test_entity",
        )

        mock_upload_to_terrablob.assert_called_once_with(
            "model_path",
            "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/1/main",
            timeout=None,
            source_entity="test_entity",
            auth_mode=None,
        )
        mock_get_latest_model_revision_id.assert_called_once_with(
            "test_project",
            "test_model",
        )
        self.assertEqual(tb_model_path, "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/1/main")
