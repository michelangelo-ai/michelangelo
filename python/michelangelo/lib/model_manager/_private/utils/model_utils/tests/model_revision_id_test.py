from unittest import TestCase
from unittest.mock import patch
from michelangelo._internal.errors.terrablob_error import TerrablobFileNotFoundError, TerrablobFailedPreconditionError, TerrablobError
from michelangelo.gen.api.v2.model_pb2 import Model
from michelangelo.lib.model_manager._private.utils.model_utils import (
    get_latest_model_revision_id,
    get_latest_uploaded_model_revision,
)


class ModelRevisionIDTest(TestCase):
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelService.get_model")
    def test_get_latest_model_revision_id(
        self,
        mock_get_model,
    ):
        model = Model()
        model.spec.revision_id = 0
        mock_get_model.return_value = model

        revision_id = get_latest_model_revision_id(
            "test_project",
            "test_model",
        )

        self.assertEqual(revision_id, 0)

        model.spec.revision_id = 1
        mock_get_model.return_value = model

        revision_id = get_latest_model_revision_id(
            "test_project",
            "test_model",
        )

        self.assertEqual(revision_id, 1)

        mock_get_model.side_effect = Exception()

        revision_id = get_latest_model_revision_id(
            "test_project",
            "test_model",
        )

        self.assertEqual(revision_id, -1)

    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_terrablob_auth_mode")
    def test_get_latest_uploaded_model_revision(
        self,
        mock_get_terrablob_auth_mode,
        mock_path_exists,
        mock_get_latest_model_revision_id,
        mock_list_terrablob_dir,
    ):
        mock_get_terrablob_auth_mode.return_value = None
        mock_get_latest_model_revision_id.return_value = 0
        mock_path_exists.return_value = True
        mock_list_terrablob_dir.return_value = ["0", "1", "2"]

        revision = get_latest_uploaded_model_revision(
            "test_project",
            "test_model",
            lambda x: f"test_model/{x}",
            "test_model",
        )

        self.assertEqual(revision, "0")
        mock_list_terrablob_dir.assert_not_called()

        mock_get_latest_model_revision_id.return_value = 1

        revision = get_latest_uploaded_model_revision(
            "test_project",
            "test_model",
            lambda x: f"test_model/{x}",
            "test_model",
        )

        self.assertEqual(revision, "1")

        mock_path_exists.return_value = False

        revision = get_latest_uploaded_model_revision(
            "test_project",
            "test_model",
            lambda x: f"test_model/{x}",
            "test_model",
        )
        self.assertEqual(revision, "2")

        mock_path_exists.return_value = True
        mock_get_latest_model_revision_id.return_value = -1

        revision = get_latest_uploaded_model_revision(
            "test_project",
            "test_model",
            lambda x: f"test_model/{x}",
            "test_model",
        )

        self.assertEqual(revision, "2")

        mock_list_terrablob_dir.return_value = []

        revision = get_latest_uploaded_model_revision(
            "test_project",
            "test_model",
            lambda x: f"test_model/{x}",
            "test_model",
        )

        self.assertIsNone(revision)

        mock_list_terrablob_dir.return_value = ["a", "b"]
        revision = get_latest_uploaded_model_revision(
            "test_project",
            "test_model",
            lambda x: f"test_model/{x}",
            "test_model",
        )

        self.assertEqual(revision, "a")

    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_terrablob_auth_mode")
    def test_get_latest_uploaded_model_revision_with_params(
        self,
        mock_get_terrablob_auth_mode,
        mock_path_exists,
        mock_get_latest_model_revision_id,
        mock_list_terrablob_dir,
    ):
        mock_get_terrablob_auth_mode.return_value = None
        mock_get_latest_model_revision_id.return_value = -1
        mock_list_terrablob_dir.return_value = ["0", "1", "2"]

        revision = get_latest_uploaded_model_revision(
            "test_project",
            "test_model",
            lambda x: f"test_model/{x}",
            "test_model",
            timeout="timeout",
            source_entity="source_entity",
        )

        self.assertEqual(revision, "2")
        mock_list_terrablob_dir.assert_called_with(
            "test_model",
            output_relative_path=True,
            include_dir=True,
            timeout="timeout",
            source_entity="source_entity",
            auth_mode=None,
        )

        mock_get_latest_model_revision_id.return_value = 0

        revision = get_latest_uploaded_model_revision(
            "test_project",
            "test_model",
            lambda x: f"test_model/{x}",
            "test_model",
            timeout="timeout",
            source_entity="source_entity",
        )

        mock_path_exists.assert_called_with(
            "test_model/0",
            timeout="timeout",
            source_entity="source_entity",
            auth_mode=None,
        )

    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.list_terrablob_dir")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_latest_model_revision_id")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.path_exists")
    @patch("michelangelo.lib.model_manager._private.utils.model_utils.model_revision_id.get_terrablob_auth_mode")
    def test_get_latest_uploaded_model_revision_with_exception(
        self,
        mock_get_terrablob_auth_mode,
        mock_path_exists,
        mock_get_latest_model_revision_id,
        mock_list_terrablob_dir,
    ):
        mock_get_terrablob_auth_mode.return_value = None
        mock_get_latest_model_revision_id.return_value = -1

        mock_list_terrablob_dir.side_effect = TerrablobFileNotFoundError("error")

        revision = get_latest_uploaded_model_revision(
            "test_project",
            "test_model",
            lambda x: f"test_model/{x}",
            "test_model",
        )

        self.assertIsNone(revision)

        mock_list_terrablob_dir.side_effect = TerrablobFailedPreconditionError("error")

        revision = get_latest_uploaded_model_revision(
            "test_project",
            "test_model",
            lambda x: f"test_model/{x}",
            "test_model",
        )

        self.assertIsNone(revision)

        mock_list_terrablob_dir.side_effect = TerrablobError("error")

        with self.assertRaises(TerrablobError):
            revision = get_latest_uploaded_model_revision(
                "test_project",
                "test_model",
                lambda x: f"test_model/{x}",
                "test_model",
            )

        mock_get_latest_model_revision_id.return_value = 0
        mock_list_terrablob_dir.side_effect = None
        mock_path_exists.side_effect = TerrablobError("error")

        with self.assertRaises(TerrablobError):
            revision = get_latest_uploaded_model_revision(
                "test_project",
                "test_model",
                lambda x: f"test_model/{x}",
                "test_model",
            )
