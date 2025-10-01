"""
Unit tests for mactl CLI functions.

Tests gRPC reflection services and service class creation logic.
"""

from unittest import TestCase
from unittest.mock import Mock, patch

from michelangelo.cli.mactl.mactl import (
    list_services,
    create_serivce_classes,
)


class GrpcReflectionTest(TestCase):
    """
    MaCTL gRPC Reflection feature related Tests
    """

    def test_list_services(self):
        """
        Test `list_services()` function
        """
        # Create mock channel
        mock_channel = Mock()

        # Create mock services that would be returned by the server
        # In actual server, more services can be present.
        # Here we create only 3 for testing.
        mock_service1 = Mock()
        mock_service1.name = "grpc.reflection.v1alpha.ServerReflection"

        mock_service2 = Mock()
        mock_service2.name = "michelangelo.api.v2.ProjectService"

        mock_service3 = Mock()
        mock_service3.name = "michelangelo.api.v2.ModelService"

        # Create mock list_services_response
        mock_list_services_response = Mock()
        mock_list_services_response.service = [
            mock_service1,
            mock_service2,
            mock_service3,
        ]

        # Create mock response
        mock_response = Mock()
        mock_response.list_services_response = mock_list_services_response

        # Create mock stub
        mock_stub = Mock()
        mock_stub.ServerReflectionInfo.return_value = [mock_response]

        # Patch the ServerReflectionStub to return our mock
        with patch(
            "michelangelo.cli.mactl.mactl.reflection_pb2_grpc.ServerReflectionStub",
            return_value=mock_stub,
        ):
            result = list_services(mock_channel)

        expected_services = [
            "grpc.reflection.v1alpha.ServerReflection",
            "michelangelo.api.v2.ProjectService",
            "michelangelo.api.v2.ModelService",
        ]
        self.assertEqual(result, expected_services)

        # The stub creation verification is already covered by the fact that
        # our mock was used inside the context manager

        # Verify that ServerReflectionInfo was called correctly
        mock_stub.ServerReflectionInfo.assert_called_once()

        # Get the call arguments to verify the request
        call_args = mock_stub.ServerReflectionInfo.call_args
        request_iter = call_args[0][0]
        request_list = list(request_iter)

        # Verify the request was created correctly
        self.assertEqual(len(request_list), 1)
        request = request_list[0]
        self.assertEqual(request.list_services, "")

        # Verify metadata was passed
        call_kwargs = call_args[1]
        self.assertIn("metadata", call_kwargs)

    def test_list_services_no_services_found(self):
        """
        Test `list_services()` function when no services are found
        """
        # Create mock channel
        mock_channel = Mock()

        # Create mock stub that returns empty response
        mock_stub = Mock()
        mock_stub.ServerReflectionInfo.return_value = []  # Empty iterator

        # Patch the ServerReflectionStub to return our mock
        with patch(
            "michelangelo.cli.mactl.mactl.reflection_pb2_grpc.ServerReflectionStub",
            return_value=mock_stub,
        ):
            with self.assertRaises(ValueError) as context:
                list_services(mock_channel)

        # Verify the error message
        self.assertEqual(str(context.exception), "No services found")


class ServiceClassCreationTest(TestCase):
    """
    Tests for create_serivce_classes function
    """

    @patch("michelangelo.cli.mactl.mactl.CRD")
    def test_create_serivce_classes_with_various_service_lists(self, mock_crd_class):
        """
        Test `create_serivce_classes()` function with both v2 and v2beta1 service lists
        """
        services = [
            "grpc.health.v1.Health",
            "grpc.reflection.v1alpha.ServerReflection",
            "michelangelo.api.v2.CachedOutputService",
            "michelangelo.api.v2.ModelFamilyService",
            "michelangelo.api.v2.ModelService",
            "michelangelo.api.v2.PipelineRunService",
            "michelangelo.api.v2.PipelineService",
            "michelangelo.api.v2.ProjectService",
            "michelangelo.api.v2.RayClusterService",
            "michelangelo.api.v2.RayJobService",
            "michelangelo.api.v2.SparkJobService",
            "michelangelo.api.v2.TriggerRunService"
            "michelangelo.api.v2beta1.AgentExtService",
            "michelangelo.api.v2beta1.AlertService",
            "michelangelo.api.v2beta1.FeatureGroupService",
            "michelangelo.api.v2beta1.GenerativeAiApplicationService",
            "michelangelo.api.v2beta1.ModelExtService",
            "michelangelo.api.v2beta1.ProjectService",
            "uber.infra.capeng.consgraph.provider.Provider",
        ]
        expected_sample_crds = [
            "alert",
            "cached_output",
            "feature_group",
            "generative_ai_application",
            "model_family",
            "model",
            "pipeline",
            "pipeline_run",
            "project",
            "ray_cluster",
            "ray_job",
            "spark_job",
        ]
        mock_crd_instance = Mock()
        mock_crd_class.return_value = mock_crd_instance

        result = create_serivce_classes(services)

        # Verify the result structure
        # Verify sample expected CRD names are present
        self.assertIsInstance(result, dict)
        self.assertEqual(sorted(result), sorted(expected_sample_crds))

        # Check that CRD was called for each expected service
        # Some duplicated calls may happen due to filtering
        self.assertEqual(mock_crd_class.call_count, 13)

    def test_create_serivce_classes_filters_out_non_service_entries(self):
        """
        Test that non-Service entries are filtered out correctly
        """
        services = [
            "grpc.reflection.v1alpha.ServerReflection",  # Should be filtered out (not ending with Service)
            "michelangelo.api.v2.ProjectService",  # Should be included
            "michelangelo.api.v2.SomeExtService",  # Should be filtered out (ends with ExtService)
            "michelangelo.api.v2.ModelService",  # Should be included
            "some.random.endpoint",  # Should be filtered out (not ending with Service)
        ]

        with patch("michelangelo.cli.mactl.mactl.CRD") as mock_crd_class:
            result = create_serivce_classes(services)

        # Should only include ProjectService and ModelService
        self.assertEqual(len(result), 2)
        self.assertIn("project", result)
        self.assertIn("model", result)

        # Verify CRD was called twice
        self.assertEqual(mock_crd_class.call_count, 2)

    def test_create_serivce_classes_empty_list(self):
        """
        Test `create_serivce_classes()` function with empty service list
        """
        services = []

        result = create_serivce_classes(services)

        # Should return empty dict
        self.assertEqual(result, {})
        self.assertEqual(len(result), 0)

    def test_create_serivce_classes_camel_to_snake_conversion(self):
        """
        Test that camel case to snake case conversion works correctly
        """
        services = [
            "test.api.ComplexServiceNameService",  # Should become 'complex_service_name'
            "test.api.SimpleService",  # Should become 'simple'
            "test.api.XMLHttpRequestService",  # Should become 'xml_http_request'
        ]

        with patch("michelangelo.cli.mactl.mactl.CRD") as mock_crd_class:
            result = create_serivce_classes(services)

        expected_names = ["complex_service_name", "simple", "xml_http_request"]

        self.assertEqual(len(result), 3)
        for expected_name in expected_names:
            self.assertIn(expected_name, result)
