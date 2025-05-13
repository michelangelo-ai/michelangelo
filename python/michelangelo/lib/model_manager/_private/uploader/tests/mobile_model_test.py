import os
import tempfile
from unittest import TestCase
from unittest.mock import patch
from michelangelo.lib.model_manager._private.uploader import upload_mobile_model


class MobileModelTest(TestCase):
    @patch("michelangelo.lib.model_manager._private.uploader.mobile_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    def test_upload_mobile_model_with_one_file(
        self,
        mock_upload_tar_to_terrablob,
        mock_upload_gz_to_terrablob,
    ):
        with tempfile.TemporaryDirectory() as temp_dir:
            with open(os.path.join(temp_dir, "file.txt"), "w") as f:
                f.write("content")

            result = upload_mobile_model(
                model_path=temp_dir,
                project_name="project_name",
                model_name="model_name",
                model_revision="0",
            )

            self.assertEqual(
                result,
                "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/mobile/deploy_gz/model.gz",
            )

            mock_upload_gz_to_terrablob.assert_called()
            args, kwargs = mock_upload_gz_to_terrablob.call_args

            self.assertTrue(args[0].endswith("model.gz"))
            self.assertEqual(
                args[1],
                "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/mobile/deploy_gz/model.gz",
            )
            self.assertIsNone(kwargs["timeout"])
            self.assertIsNone(kwargs["source_entity"])

            mock_upload_tar_to_terrablob.assert_not_called()

    @patch("michelangelo.lib.model_manager._private.uploader.mobile_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    def test_upload_mobile_model_with_multiple_files(
        self,
        mock_upload_tar_to_terrablob,
        mock_upload_gz_to_terrablob,
    ):
        with tempfile.TemporaryDirectory() as temp_dir:
            with open(os.path.join(temp_dir, "file.txt"), "w") as f:
                f.write("content")
            with open(os.path.join(temp_dir, "file1.txt"), "w") as f:
                f.write("content")

            result = upload_mobile_model(
                model_path=temp_dir,
                project_name="project_name",
                model_name="model_name",
                model_revision="0",
            )

            self.assertEqual(
                result,
                "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/mobile/deploy_tar/model.tar",
            )
            mock_upload_tar_to_terrablob.assert_called()
            mock_upload_gz_to_terrablob.assert_not_called()

    @patch("michelangelo.lib.model_manager._private.uploader.mobile_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    def test_upload_mobile_model_with_one_dir(
        self,
        mock_upload_tar_to_terrablob,
        mock_upload_gz_to_terrablob,
    ):
        with tempfile.TemporaryDirectory() as temp_dir:
            os.makedirs(os.path.join(temp_dir, "folder"))

            result = upload_mobile_model(
                model_path=temp_dir,
                project_name="project_name",
                model_name="model_name",
                model_revision="0",
            )

            self.assertEqual(
                result,
                "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/mobile/deploy_tar/model.tar",
            )
            mock_upload_tar_to_terrablob.assert_called()
            mock_upload_gz_to_terrablob.assert_not_called()

    @patch("michelangelo.lib.model_manager._private.uploader.mobile_model.upload_to_terrablob")
    @patch("michelangelo.lib.model_manager._private.uploader.generic_deployable_model.upload_to_terrablob")
    def test_upload_mobile_model_with_empty_dir(
        self,
        mock_upload_tar_to_terrablob,
        mock_upload_gz_to_terrablob,
    ):
        with tempfile.TemporaryDirectory() as temp_dir:
            result = upload_mobile_model(
                model_path=temp_dir,
                project_name="project_name",
                model_name="model_name",
                model_revision="0",
            )

            self.assertEqual(
                result,
                "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/mobile/deploy_tar/model.tar",
            )
            mock_upload_tar_to_terrablob.assert_called()
            mock_upload_gz_to_terrablob.assert_not_called()
