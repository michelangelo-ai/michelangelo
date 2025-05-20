from unittest import TestCase
from unittest.mock import patch
from michelangelo.lib.model_manager._private.uploader import upload_model_crd
from michelangelo.gen.api.v2.model_pb2 import (
    Model,
    MODEL_KIND_CUSTOM,
    DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON,
)


class ModelCrdTest(TestCase):
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelService.create_model")
    @patch("michelangelo.lib.model_manager._private.uploader.model_crd.create_model_family")
    def test_upload_model_crd(
        self,
        mock_create_model_family,
        mock_create_model,
    ):
        model = upload_model_crd(
            project_name="project_name",
            model_name="model_name",
        )

        expected_model = Model()
        expected_model.metadata.namespace = "project_name"
        expected_model.metadata.name = "model_name"
        expected_model.spec.kind = MODEL_KIND_CUSTOM
        expected_model.spec.package_type = DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON
        expected_model.spec.sealed = True
        expected_model.spec.deployable_artifact_uri.append(
            "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/triton/deploy_tar/model.tar"
        )

        mock_create_model.assert_called_once_with(expected_model)
        mock_create_model_family.assert_not_called()
        self.assertEqual(model, expected_model)

    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelService.create_model")
    @patch("michelangelo.lib.model_manager._private.uploader.model_crd.create_model_family")
    def test_upload_model_crd_with_training_framework(
        self,
        mock_create_model_family,
        mock_create_model,
    ):
        model = upload_model_crd(
            project_name="project_name",
            model_name="model_name",
            training_framework="pytorch",
        )

        expected_model = Model()
        expected_model.metadata.namespace = "project_name"
        expected_model.metadata.name = "model_name"
        expected_model.spec.kind = MODEL_KIND_CUSTOM
        expected_model.spec.package_type = DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON
        expected_model.spec.sealed = True
        expected_model.spec.deployable_artifact_uri.append(
            "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/triton/deploy_tar/model.tar"
        )
        expected_model.spec.training_framework = "pytorch"

        mock_create_model.assert_called_once_with(expected_model)
        mock_create_model_family.assert_not_called()
        self.assertEqual(model, expected_model)

    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelService.create_model")
    @patch("michelangelo.lib.model_manager._private.uploader.model_crd.create_model_family")
    def test_upload_model_crd_with_model_family(
        self,
        mock_create_model_family,
        mock_create_model,
    ):
        model = upload_model_crd(
            project_name="project_name",
            model_name="model_name",
            model_family="model_family_name",
        )

        expected_model = Model()
        expected_model.metadata.namespace = "project_name"
        expected_model.metadata.name = "model_name"
        expected_model.spec.kind = MODEL_KIND_CUSTOM
        expected_model.spec.package_type = DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON
        expected_model.spec.model_family.namespace = "project_name"
        expected_model.spec.model_family.name = "model_family_name"
        expected_model.spec.sealed = True
        expected_model.spec.deployable_artifact_uri.append(
            "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/0/package/triton/deploy_tar/model.tar"
        )

        mock_create_model_family.assert_called_once_with("project_name", "model_family_name")
        mock_create_model.assert_called_once_with(expected_model)
        self.assertEqual(model, expected_model)

    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelService.create_model")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelService.get_model")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelService.update_model")
    def test_upload_model_crd_with_update_model(
        self,
        mock_update_model,
        mock_get_model,
        mock_create_model,
    ):
        previous_model = Model()
        previous_model.metadata.resourceVersion = "1"
        mock_get_model.return_value = previous_model

        model = upload_model_crd(
            project_name="project_name",
            model_name="model_name",
            revision_id=1,
            sealed=False,
        )

        expected_model = Model()
        expected_model.metadata.namespace = "project_name"
        expected_model.metadata.name = "model_name"
        expected_model.spec.kind = MODEL_KIND_CUSTOM
        expected_model.spec.package_type = DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON
        expected_model.spec.sealed = False
        expected_model.spec.revision_id = 1
        expected_model.spec.deployable_artifact_uri.append(
            "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/1/package/triton/deploy_tar/model.tar"
        )
        expected_model.metadata.resourceVersion = "1"

        mock_update_model.assert_called_once_with(expected_model)
        mock_create_model.assert_not_called()
        self.assertEqual(model, expected_model)

    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelService.create_model")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelService.get_model")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelService.update_model")
    def test_upload_model_crd_with_missing_previous_model(
        self,
        mock_update_model,
        mock_get_model,
        mock_create_model,
    ):
        mock_get_model.side_effect = Exception("error")

        model = upload_model_crd(
            project_name="project_name",
            model_name="model_name",
            revision_id=1,
            sealed=False,
        )

        expected_model = Model()
        expected_model.metadata.namespace = "project_name"
        expected_model.metadata.name = "model_name"
        expected_model.spec.kind = MODEL_KIND_CUSTOM
        expected_model.spec.package_type = DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON
        expected_model.spec.sealed = False
        expected_model.spec.revision_id = 1
        expected_model.spec.deployable_artifact_uri.append(
            "/prod/michelangelo/deployable_models/projects/project_name/models/model_name/revisions/1/package/triton/deploy_tar/model.tar"
        )

        mock_create_model.assert_called_once_with(expected_model)
        mock_update_model.assert_not_called()
        self.assertEqual(model, expected_model)
