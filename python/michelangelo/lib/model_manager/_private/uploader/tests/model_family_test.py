import grpc
from unittest import TestCase
from unittest.mock import patch
from michelangelo.lib.model_manager._private.uploader import create_model_family
from michelangelo.gen.api.v2.model_family_pb2 import ModelFamily


class ModelFamilyTest(TestCase):
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelFamilyService.create_model_family")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelFamilyService.get_model_family")
    def test_create_model_family(
        self,
        mock_get_model_family,
        mock_create_model_family,
    ):
        model_family = create_model_family("project_name", "model_family_name")

        expected_model_family = ModelFamily()
        expected_model_family.metadata.namespace = "project_name"
        expected_model_family.metadata.name = "model_family_name"
        expected_model_family.spec.name = "model_family_name"

        mock_create_model_family.assert_called_once_with(expected_model_family)
        mock_get_model_family.assert_not_called()
        self.assertEqual(model_family, expected_model_family)

    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelFamilyService.create_model_family")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelFamilyService.get_model_family")
    def test_create_model_family_already_exists(
        self,
        mock_get_model_family,
        mock_create_model_family,
    ):
        e = grpc.RpcError()
        e.code = lambda: grpc.StatusCode.ALREADY_EXISTS
        mock_create_model_family.side_effect = e

        model_family = create_model_family("project_name", "model_family_name")
        mock_create_model_family.assert_called_once()
        mock_get_model_family.assert_called_once_with(
            namespace="project_name",
            name="model_family_name",
        )

        self.assertIsNone(model_family)

    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelFamilyService.create_model_family")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelFamilyService.get_model_family")
    def test_create_model_family_already_exists_in_different_namespace(
        self,
        mock_get_model_family,
        mock_create_model_family,
    ):
        e = grpc.RpcError()
        e.code = lambda: grpc.StatusCode.ALREADY_EXISTS
        mock_create_model_family.side_effect = e

        ge = grpc.RpcError()
        ge.code = lambda: grpc.StatusCode.NOT_FOUND
        mock_get_model_family.side_effect = ge

        with self.assertRaises(RuntimeError):
            create_model_family("project_name", "model_family_name")

    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelFamilyService.create_model_family")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelFamilyService.get_model_family")
    def test_create_model_family_get_error(
        self,
        mock_get_model_family,
        mock_create_model_family,
    ):
        e = grpc.RpcError()
        e.code = lambda: grpc.StatusCode.ALREADY_EXISTS
        mock_create_model_family.side_effect = e

        ge = grpc.RpcError()
        ge.code = lambda: grpc.StatusCode.UNKNOWN
        mock_get_model_family.side_effect = ge

        with self.assertRaises(RuntimeError):
            create_model_family("project_name", "model_family_name")

    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelFamilyService.create_model_family")
    @patch("michelangelo.lib.model_manager._private.utils.api_client.APIClient.ModelFamilyService.get_model_family")
    def test_create_model_family_other_error(
        self,
        mock_get_model_family,
        mock_create_model_family,
    ):
        e = grpc.RpcError()
        e.code = lambda: grpc.StatusCode.UNKNOWN
        mock_create_model_family.side_effect = e

        mock_get_model_family.assert_not_called()

        with self.assertRaises(grpc.RpcError):
            create_model_family("project_name", "model_family_name")
