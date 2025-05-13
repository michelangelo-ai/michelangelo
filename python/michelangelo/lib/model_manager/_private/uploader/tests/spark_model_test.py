from unittest.mock import patch
from uber.ai.michelangelo.shared.testing.env import EnvTestCase
from michelangelo.lib.model_manager._private.uploader import upload_spark_model
from .utils.env import mimic_local_env, mimic_remote_env


class SparkModelTest(EnvTestCase):
    @patch("michelangelo.lib.model_manager._private.uploader.spark_model.upload_to_terrablob")
    def test_upload_spark_model(
        self,
        mock_upload_to_terrablob,
    ):
        mimic_local_env()
        model_path = "model_path"
        project_name = "project_name"
        model_name = "model_name"
        model_revision = "0"

        tb_path = upload_spark_model(
            model_path,
            project_name,
            model_name,
            model_revision,
        )

        expected_upload_dest = "/prod/michelangelo/v2_projects/project_name/trained_models/model_name/0"
        expected_tb_path = f"{expected_upload_dest}/deploy_jar/model.jar.gz"

        mock_upload_to_terrablob.assert_called_once_with(
            model_path,
            expected_upload_dest,
            use_kraken=True,
            use_threads=False,
            multipart=True,
            concurrency=10,
            timeout=None,
            keepalive=True,
            source_entity=None,
        )

        self.assertEqual(tb_path, expected_tb_path)

    @patch("michelangelo.lib.model_manager._private.uploader.spark_model.upload_to_terrablob")
    def test_upload_spark_model_remote_env(
        self,
        mock_upload_to_terrablob,
    ):
        mimic_remote_env()
        model_path = "model_path"
        project_name = "project_name"
        model_name = "model_name"
        model_revision = "0"

        tb_path = upload_spark_model(
            model_path,
            project_name,
            model_name,
            model_revision,
        )

        expected_upload_dest = "/prod/michelangelo/v2_projects/project_name/trained_models/model_name/0"
        expected_tb_path = f"{expected_upload_dest}/deploy_jar/model.jar.gz"

        mock_upload_to_terrablob.assert_called_once_with(
            model_path,
            expected_upload_dest,
            use_kraken=True,
            use_threads=False,
            timeout=None,
            source_entity=None,
        )

        self.assertEqual(tb_path, expected_tb_path)
