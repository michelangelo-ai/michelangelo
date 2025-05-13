from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.sdk.model_manager.constants import PackageType
from uber.ai.michelangelo.sdk.model_manager.downloader import download_deployable_model


class DeployableModelTest(TestCase):
    @patch("uber.ai.michelangelo.sdk.model_manager.downloader.deployable_model.download_spark_pipeline_model")
    def test_download_spark_pipeline_model(
        self,
        mock_download_spark_pipeline_model,
    ):
        mock_download_spark_pipeline_model.return_value = None
        download_deployable_model(
            "test_project",
            "test_model",
            "0",
            "output",
        )
        mock_download_spark_pipeline_model.assert_called_once_with(
            "test_project",
            "test_model",
            "0",
            "output",
            None,
            None,
        )

        download_deployable_model(
            "test_project",
            "test_model",
        )

        args = mock_download_spark_pipeline_model.call_args.args

        self.assertEqual(args[0], "test_project")
        self.assertEqual(args[1], "test_model")
        self.assertEqual(args[2], None)
        self.assertTrue("/tmp" in args[3])
        self.assertEqual(args[4], None)
        self.assertEqual(args[5], None)

    @patch("uber.ai.michelangelo.sdk.model_manager.downloader.deployable_model.download_generic_deployable_model")
    def test_download_generic_deployable_model(
        self,
        mock_download_generic_deployable_model,
    ):
        download_deployable_model(
            "test_project",
            "test_model",
            "0",
            "output",
            PackageType.TRITON,
        )

        mock_download_generic_deployable_model.assert_called_once_with(
            "test_project",
            "test_model",
            "0",
            PackageType.TRITON,
            "output",
            None,
            None,
        )

        download_deployable_model(
            "test_project",
            "test_model",
            package_type=PackageType.TRITON,
        )

        args = mock_download_generic_deployable_model.call_args.args

        self.assertEqual(args[0], "test_project")
        self.assertEqual(args[1], "test_model")
        self.assertEqual(args[2], None)
        self.assertEqual(args[3], PackageType.TRITON)
        self.assertTrue("/tmp" in args[4])
        self.assertEqual(args[5], None)
        self.assertEqual(args[6], None)
