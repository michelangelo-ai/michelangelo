import os
import tempfile
from unittest import TestCase
from unittest.mock import patch
from michelangelo.lib.model_manager.uploader import upload_deployable_model
from michelangelo.lib.model_manager.constants import PackageType


class DeployableModelTest(TestCase):
    @patch("michelangelo.lib.model_manager._private.uploader.spark_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager.uploader.deployable_model.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager.uploader.deployable_model.infer_model_package_type")
    @patch.dict(os.environ, {"UF_TASK_IMAGE": "image"})
    def test_upload_spark_deployable_model(
        self,
        mock_infer_model_package_type,
        mock_get_latest_model_revision_id,
        mock_upload_to_terrablob,
    ):
        model_path = "model_path"
        project_name = "project_name"
        model_name = "model_name"
        mock_infer_model_package_type.return_value = PackageType.SPARK
        mock_get_latest_model_revision_id.return_value = -1

        tb_path = upload_deployable_model(
            model_path,
            project_name,
            model_name,
        )

        expected_upload_dest = "/prod/michelangelo/v2_projects/project_name/trained_models/model_name/0"
        expected_tb_path = f"{expected_upload_dest}/deploy_jar/model.jar.gz"

        mock_upload_to_terrablob.assert_called_once_with(
            model_path,
            expected_upload_dest,
            use_kraken=True,
            use_threads=False,
            timeout=None,
            source_entity="michelangelo-apiserver",
        )

        self.assertEqual(tb_path, expected_tb_path)

    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager.uploader.deployable_model.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager.uploader.deployable_model.infer_model_package_type")
    def test_upload_triton_and_raw_deployable_model(
        self,
        mock_infer_model_package_type,
        mock_get_latest_model_revision_id,
        mock_upload_to_terrablob,
    ):
        for pkg_type in [PackageType.TRITON, PackageType.RAW]:
            project_name = "project_name"
            model_name = "model_name"
            mock_infer_model_package_type.return_value = pkg_type
            mock_get_latest_model_revision_id.return_value = -1

            with tempfile.TemporaryDirectory() as temp_dir:
                model_path = os.path.join(temp_dir, "model")
                os.makedirs(model_path)
                tb_path = upload_deployable_model(
                    model_path,
                    project_name,
                    model_name,
                    timeout="2h",
                )

                expected_tb_path = (
                    f"/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/{pkg_type}/deploy_tar/model.tar"
                )

                mock_upload_to_terrablob.assert_called()

                args, kwargs = mock_upload_to_terrablob.call_args
                self.assertTrue(args[0].endswith("model.tar"))
                self.assertEqual(args[1], expected_tb_path)
                self.assertEqual(kwargs["timeout"], "2h")
                self.assertEqual(kwargs["source_entity"], "michelangelo-apiserver")

                self.assertEqual(tb_path, expected_tb_path)

    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager.uploader.deployable_model.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager.uploader.deployable_model.infer_model_package_type")
    def test_upload_deployable_model_with_package_type_param(
        self,
        mock_infer_model_package_type,
        mock_get_latest_model_revision_id,
        mock_upload_to_terrablob,
    ):
        project_name = "project_name"
        model_name = "model_name"
        mock_infer_model_package_type.return_value = PackageType.SPARK
        mock_get_latest_model_revision_id.return_value = 1

        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            tb_path = upload_deployable_model(
                model_path,
                project_name,
                model_name,
                package_type=PackageType.TRITON,
            )

            expected_tb_path = (
                "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/2/package/triton/deploy_tar/model.tar"
            )

            mock_upload_to_terrablob.assert_called()

            args, kwargs = mock_upload_to_terrablob.call_args
            self.assertTrue(args[0].endswith("model.tar"))
            self.assertEqual(args[1], expected_tb_path)
            self.assertIsNone(kwargs["timeout"])
            self.assertEqual(kwargs["source_entity"], "michelangelo-apiserver")

            self.assertEqual(tb_path, expected_tb_path)

    @patch("michelangelo.lib.model_manager._private.uploader.mobile_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager.uploader.deployable_model.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager.uploader.deployable_model.infer_model_package_type")
    def test_upload_mobile_deployable_model(
        self,
        mock_infer_model_package_type,
        mock_get_latest_model_revision_id,
        mock_upload_tar_to_terrablob,
        mock_upload_gz_to_terrablob,
    ):
        project_name = "project_name"
        model_name = "model_name"
        mock_infer_model_package_type.return_value = PackageType.RAW
        mock_get_latest_model_revision_id.return_value = -1

        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            with open(os.path.join(model_path, "file.txt"), "w") as f:
                f.write("content")

            tb_path = upload_deployable_model(
                model_path,
                project_name,
                model_name,
                package_type=PackageType.MOBILE,
                timeout="2h",
            )

            expected_tb_path = "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/mobile/deploy_gz/model.gz"

            mock_upload_gz_to_terrablob.assert_called()

            args, kwargs = mock_upload_gz_to_terrablob.call_args
            self.assertTrue(args[0].endswith("model.gz"))
            self.assertEqual(args[1], expected_tb_path)
            self.assertEqual(kwargs["timeout"], "2h")
            self.assertEqual(kwargs["source_entity"], "michelangelo-apiserver")

            self.assertEqual(tb_path, expected_tb_path)

            mock_upload_tar_to_terrablob.assert_not_called()

    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager.uploader.deployable_model.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager.uploader.deployable_model.infer_model_package_type")
    def test_upload_deployable_model_with_revision_id_param(
        self,
        mock_infer_model_package_type,
        mock_get_latest_model_revision_id,
        mock_upload_to_terrablob,
    ):
        project_name = "project_name"
        model_name = "model_name"
        mock_infer_model_package_type.return_value = PackageType.TRITON
        revision_id = 0

        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            tb_path = upload_deployable_model(
                model_path,
                project_name,
                model_name,
                revision_id=revision_id,
            )

            expected_tb_path = (
                "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/triton/deploy_tar/model.tar"
            )

            mock_upload_to_terrablob.assert_called()
            mock_get_latest_model_revision_id.assert_not_called()

            args, kwargs = mock_upload_to_terrablob.call_args
            self.assertTrue(args[0].endswith("model.tar"))
            self.assertEqual(args[1], expected_tb_path)
            self.assertEqual(kwargs["timeout"], None)
            self.assertEqual(kwargs["source_entity"], "michelangelo-apiserver")

            self.assertEqual(tb_path, expected_tb_path)

    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager.uploader.deployable_model.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager.uploader.deployable_model.infer_model_package_type")
    def test_upload_deployable_model_with_source_entity_param(
        self,
        mock_infer_model_package_type,
        mock_get_latest_model_revision_id,
        mock_upload_to_terrablob,
    ):
        project_name = "project_name"
        model_name = "model_name"
        mock_infer_model_package_type.return_value = PackageType.TRITON
        mock_get_latest_model_revision_id.return_value = -1

        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            tb_path = upload_deployable_model(model_path, project_name, model_name, source_entity="source_entity")

            expected_tb_path = (
                "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/triton/deploy_tar/model.tar"
            )

            mock_upload_to_terrablob.assert_called()

            args, kwargs = mock_upload_to_terrablob.call_args
            self.assertTrue(args[0].endswith("model.tar"))
            self.assertEqual(args[1], expected_tb_path)
            self.assertEqual(kwargs["timeout"], None)
            self.assertEqual(kwargs["source_entity"], "source_entity")

            self.assertEqual(tb_path, expected_tb_path)
