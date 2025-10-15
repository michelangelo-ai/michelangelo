"""
Unit tests for mactl CLI functions.

Tests gRPC reflection services and service class creation logic.
"""

from unittest import TestCase
from unittest.mock import Mock, patch

from grpc_reflection.v1alpha.reflection_pb2 import ServerReflectionRequest

from michelangelo.cli.mactl.mactl import (
    list_services,
    create_serivce_classes,
    get_service_descriptors,
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


class GetServiceDescriptorsTest(TestCase):
    """
    Tests for get_service_descriptors function
    """

    def _get_project_svc_mock(self) -> Mock:
        """
        Helper to create a mock for `michelangelo/api/v2/project_svc.proto`
        """
        # Create mock fields for CreateProjectRequest
        mock_field_project = Mock()
        mock_field_project.name = "project"
        mock_field_project.number = 1
        mock_field_project.label = 1  # LABEL_OPTIONAL
        mock_field_project.type = 11  # TYPE_MESSAGE
        mock_field_project.type_name = ".michelangelo.api.v2.Project"
        mock_field_project.json_name = "project"

        mock_field_create_options = Mock()
        mock_field_create_options.name = "create_options"
        mock_field_create_options.number = 2
        mock_field_create_options.label = 1  # LABEL_OPTIONAL
        mock_field_create_options.type = 11  # TYPE_MESSAGE
        mock_field_create_options.type_name = (
            ".k8s.io.apimachinery.pkg.apis.meta.v1.CreateOptions"
        )
        mock_field_create_options.json_name = "createOptions"

        # Create mock fields for CreateProjectResponse
        mock_field_response_project = Mock()
        mock_field_response_project.name = "project"
        mock_field_response_project.number = 1
        mock_field_response_project.label = 1  # LABEL_OPTIONAL
        mock_field_response_project.type = 11  # TYPE_MESSAGE
        mock_field_response_project.type_name = ".michelangelo.api.v2.Project"
        mock_field_response_project.json_name = "project"

        # Create mock fields for GetProjectRequest
        mock_field_name = Mock()
        mock_field_name.name = "name"
        mock_field_name.number = 1
        mock_field_name.label = 1  # LABEL_OPTIONAL
        mock_field_name.type = 9  # TYPE_STRING
        mock_field_name.json_name = "name"

        mock_field_namespace = Mock()
        mock_field_namespace.name = "namespace"
        mock_field_namespace.number = 2
        mock_field_namespace.label = 1  # LABEL_OPTIONAL
        mock_field_namespace.type = 9  # TYPE_STRING
        mock_field_namespace.json_name = "namespace"

        mock_field_get_options = Mock()
        mock_field_get_options.name = "get_options"
        mock_field_get_options.number = 3
        mock_field_get_options.label = 1  # LABEL_OPTIONAL
        mock_field_get_options.type = 11  # TYPE_MESSAGE
        mock_field_get_options.type_name = (
            ".k8s.io.apimachinery.pkg.apis.meta.v1.GetOptions"
        )
        mock_field_get_options.json_name = "getOptions"

        # Create mock methods (mimicking actual protobuf MethodDescriptorProto)
        mock_method1 = Mock()
        mock_method1.name = "CreateProject"
        mock_method1.input_type = ".michelangelo.api.v2.CreateProjectRequest"
        mock_method1.output_type = ".michelangelo.api.v2.CreateProjectResponse"

        mock_method2 = Mock()
        mock_method2.name = "GetProject"
        mock_method2.input_type = ".michelangelo.api.v2.GetProjectRequest"
        mock_method2.output_type = ".michelangelo.api.v2.GetProjectResponse"

        # Create mock service descriptor (mimicking ServiceDescriptorProto)
        mock_service_descriptor = Mock()
        mock_service_descriptor.name = "ProjectService"
        mock_service_descriptor.method = [mock_method1, mock_method2]

        # Create mock message types (request/response schemas)
        mock_create_request_msg = Mock()
        mock_create_request_msg.name = "CreateProjectRequest"
        mock_create_request_msg.field = [mock_field_project, mock_field_create_options]

        mock_create_response_msg = Mock()
        mock_create_response_msg.name = "CreateProjectResponse"
        mock_create_response_msg.field = [mock_field_response_project]

        mock_get_request_msg = Mock()
        mock_get_request_msg.name = "GetProjectRequest"
        mock_get_request_msg.field = [
            mock_field_name,
            mock_field_namespace,
            mock_field_get_options,
        ]

        mock_get_response_msg = Mock()
        mock_get_response_msg.name = "GetProjectResponse"
        mock_get_response_msg.field = [mock_field_response_project]

        # Create mock FileDescriptorProto instance for project_svc.proto
        mock_fd_instance1 = Mock()
        mock_fd_instance1.name = "michelangelo/api/v2/project_svc.proto"
        mock_fd_instance1.package = "michelangelo.api.v2"
        mock_fd_instance1.dependency = [
            "k8s.io/apimachinery/pkg/apis/meta/v1/generated.proto",
            "michelangelo/api/list.proto",
            "michelangelo/api/v2/project.proto",
        ]
        mock_fd_instance1.message_type = [
            mock_create_request_msg,
            mock_create_response_msg,
            mock_get_request_msg,
            mock_get_response_msg,
        ]
        mock_fd_instance1.service = [mock_service_descriptor]

        return mock_fd_instance1

    @patch("michelangelo.cli.mactl.mactl.reflection_pb2_grpc.ServerReflectionStub")
    @patch("michelangelo.cli.mactl.mactl.FileDescriptorProto")
    def test_get_service_descriptors_success(
        self, mock_file_descriptor_proto_class, mock_stub_class
    ):
        """
        Test `get_service_descriptors()` function with valid service
        """
        # Create mock channel
        mock_channel = Mock()
        service_name = "michelangelo.api.v2.ModelService"

        # Create mock FileDescriptorProto instance
        mock_fd_instance = Mock()
        mock_fd_instance.name = "model_service.proto"
        mock_fd_instance.service = []
        mock_file_descriptor_proto_class.return_value = mock_fd_instance

        # Create mock file descriptor bytes
        mock_fd_bytes = b"mock_file_descriptor_data"

        # Create mock response
        mock_response = Mock()
        mock_response.file_descriptor_response.file_descriptor_proto = [mock_fd_bytes]

        # Create mock stub
        mock_stub = Mock()
        mock_stub.ServerReflectionInfo.return_value = [mock_response]
        mock_stub_class.return_value = mock_stub

        # Call the function
        result = list(get_service_descriptors(mock_channel, service_name))

        # Verify the result
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0], mock_fd_instance)

        # Verify stub was created with the channel
        mock_stub_class.assert_called_once_with(mock_channel)

        # Verify ServerReflectionInfo was called
        mock_stub.ServerReflectionInfo.assert_called_once()

        # Verify the request was created correctly
        call_args = mock_stub.ServerReflectionInfo.call_args
        request_iter = call_args[0][0]
        request_list = list(request_iter)
        self.assertEqual(len(request_list), 1)
        request = request_list[0]
        self.assertEqual(request.file_containing_symbol, service_name)

        # Verify ParseFromString was called with the correct bytes
        mock_fd_instance.ParseFromString.assert_called_once_with(mock_fd_bytes)

    @patch("michelangelo.cli.mactl.mactl.reflection_pb2_grpc.ServerReflectionStub")
    @patch("michelangelo.cli.mactl.mactl.FileDescriptorProto")
    def test_get_service_descriptors_multiple_descriptors(
        self, mock_file_descriptor_proto_class, mock_stub_class
    ):
        """
        Test `get_service_descriptors()` function with multiple file descriptors
        Mimics the actual protobuf structure with message types, fields, service descriptors, and methods
        """

        # Create mock channel
        mock_channel = Mock()
        service_name = "michelangelo.api.v2.ProjectService"

        mock_fd_instance1 = self._get_project_svc_mock()

        # Create second FileDescriptorProto (dependency file)
        mock_fd_instance2 = Mock()
        mock_fd_instance2.name = "michelangelo/api/v2/project.proto"
        mock_fd_instance2.package = "michelangelo.api.v2"
        mock_fd_instance2.dependency = []
        mock_fd_instance2.message_type = []
        mock_fd_instance2.service = []

        # Setup the class to return different instances on each call
        mock_file_descriptor_proto_class.side_effect = [
            mock_fd_instance1,
            mock_fd_instance2,
        ]

        # Create mock file descriptor bytes
        mock_fd_bytes1 = b"mock_file_descriptor_data1"
        mock_fd_bytes2 = b"mock_file_descriptor_data2"

        # Create mock response with multiple file descriptors
        mock_response = Mock()
        mock_response.file_descriptor_response.file_descriptor_proto = [
            mock_fd_bytes1,
            mock_fd_bytes2,
        ]

        # Create mock stub that only returns when called with the correct channel
        mock_stub = Mock()
        mock_stub.ServerReflectionInfo.return_value = [mock_response]

        # Set ServerReflectionStub mock.
        def stub_factory(channel):
            if channel == mock_channel:
                return mock_stub
            raise ValueError("Unexpected channel")

        mock_stub_class.side_effect = stub_factory

        ### Call the function
        result = list(get_service_descriptors(mock_channel, service_name))

        ### Verify the result
        self.assertEqual(len(result), 2)
        self.assertEqual(result[0], mock_fd_instance1)
        self.assertEqual(result[1], mock_fd_instance2)

        # Verify the first descriptor has the expected service structure
        self.assertEqual(len(result[0].service), 1)
        self.assertEqual(result[0].service[0].name, "ProjectService")
        self.assertEqual(len(result[0].service[0].method), 2)
        self.assertEqual(result[0].service[0].method[0].name, "CreateProject")
        self.assertEqual(result[0].service[0].method[1].name, "GetProject")

        # Verify the first descriptor has the expected message types with fields
        self.assertEqual(len(result[0].message_type), 4)
        self.assertEqual(result[0].message_type[0].name, "CreateProjectRequest")
        self.assertEqual(result[0].message_type[1].name, "CreateProjectResponse")
        self.assertEqual(result[0].message_type[2].name, "GetProjectRequest")
        self.assertEqual(result[0].message_type[3].name, "GetProjectResponse")

        # Verify the request was created correctly with service_name and host
        call_args = mock_stub.ServerReflectionInfo.call_args
        request_iter = call_args[0][0]
        request_list = list(request_iter)
        self.assertEqual(len(request_list), 1)
        request = request_list[0]
        self.assertIsInstance(request, ServerReflectionRequest)
        self.assertEqual(request.file_containing_symbol, service_name)
        self.assertEqual(request.host, "")

        # Verify ParseFromString was called for each descriptor
        mock_fd_instance1.ParseFromString.assert_called_once_with(mock_fd_bytes1)
        mock_fd_instance2.ParseFromString.assert_called_once_with(mock_fd_bytes2)

    @patch("michelangelo.cli.mactl.mactl.reflection_pb2_grpc.ServerReflectionStub")
    def test_get_service_descriptors_empty_response(self, mock_stub_class):
        """
        Test `get_service_descriptors()` function with empty response
        """
        # Create mock channel
        mock_channel = Mock()
        service_name = "michelangelo.api.v2.EmptyService"

        # Create mock stub with empty response
        mock_stub = Mock()
        mock_stub.ServerReflectionInfo.return_value = []
        mock_stub_class.return_value = mock_stub

        # Call the function
        result = list(get_service_descriptors(mock_channel, service_name))

        # Verify the result is empty
        self.assertEqual(len(result), 0)
