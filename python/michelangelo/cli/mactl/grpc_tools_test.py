"""Unit tests for grpc_tools package."""

from unittest import TestCase
from unittest.mock import Mock, patch

from grpc_reflection.v1alpha.reflection_pb2 import ServerReflectionRequest

from michelangelo.cli.mactl.grpc_tools import (
    get_all_file_descriptors_by_filename,
    get_methods_from_service,
    get_service_descriptors,
    get_service_name,
    list_services,
)


class GrpcReflectionTest(TestCase):
    """MaCTL gRPC Reflection feature related Tests."""

    def setUp(self):
        """Set up common test data."""
        self.metadata: list = [
            ("rpc-caller", "grpcurl"),
            ("rpc-service", "ma-apiserver"),
            ("rpc-encoding", "proto"),
        ]

    def test_list_services(self):
        """Test `list_services()` function."""
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
            "michelangelo.cli.mactl.grpc_tools.reflection_pb2_grpc.ServerReflectionStub",
            return_value=mock_stub,
        ):
            result = list_services(mock_channel, self.metadata)

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
        """Test `list_services()` function when no services are found."""
        # Create mock channel
        mock_channel = Mock()

        # Create mock stub that returns empty response
        mock_stub = Mock()
        mock_stub.ServerReflectionInfo.return_value = []  # Empty iterator

        # Patch the ServerReflectionStub to return our mock
        with (
            patch(
                "michelangelo.cli.mactl.grpc_tools.reflection_pb2_grpc.ServerReflectionStub",
                return_value=mock_stub,
            ),
            self.assertRaises(ValueError) as context,
        ):
            list_services(mock_channel, self.metadata)

        # Verify the error message
        self.assertEqual(str(context.exception), "No services found")

    def test_get_service_name_found(self):
        """Test `get_service_name()` when service is found."""
        mock_channel = Mock()

        # Mock list_services to return services with different API versions
        with patch(
            "michelangelo.cli.mactl.grpc_tools.list_services",
            return_value=[
                "grpc.reflection.v1alpha.ServerReflection",
                "michelangelo.api.v2beta1.ProjectService",
                "michelangelo.api.v2beta1.PipelineRunService",
            ],
        ):
            result = get_service_name(mock_channel, self.metadata, "PipelineRunService")

        self.assertEqual(result, "michelangelo.api.v2beta1.PipelineRunService")

    def test_get_service_name_with_fallback(self):
        """Test `get_service_name()` when service not found but fallback provided."""
        mock_channel = Mock()

        # Mock list_services to return services without PipelineRunService
        with patch(
            "michelangelo.cli.mactl.grpc_tools.list_services",
            return_value=[
                "grpc.reflection.v1alpha.ServerReflection",
                "michelangelo.api.v2.ProjectService",
            ],
        ):
            result = get_service_name(
                mock_channel,
                self.metadata,
                "PipelineRunService",
                fallback="michelangelo.api.v2.PipelineRunService",
            )

        self.assertEqual(result, "michelangelo.api.v2.PipelineRunService")

    def test_get_service_name_not_found_no_fallback(self):
        """Test `get_service_name()` when service not found and no fallback."""
        mock_channel = Mock()

        # Mock list_services to return services without PipelineRunService
        with (
            patch(
                "michelangelo.cli.mactl.grpc_tools.list_services",
                return_value=[
                    "grpc.reflection.v1alpha.ServerReflection",
                    "michelangelo.api.v2.ProjectService",
                ],
            ),
            self.assertRaises(ValueError) as context,
        ):
            get_service_name(mock_channel, self.metadata, "PipelineRunService")

        self.assertIn("PipelineRunService", str(context.exception))
        self.assertIn("not found", str(context.exception))


def _get_project_svc_mock() -> Mock:
    """Helper to create a mock for `michelangelo/api/v2/project_svc.proto`."""
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


class GetServiceDescriptorsTest(TestCase):
    """Tests for get_service_descriptors function."""

    def setUp(self):
        """Set up common test data."""
        self.metadata: list = [
            ("rpc-caller", "grpcurl"),
            ("rpc-service", "ma-apiserver"),
            ("rpc-encoding", "proto"),
        ]

    @patch("michelangelo.cli.mactl.grpc_tools.reflection_pb2_grpc.ServerReflectionStub")
    @patch("michelangelo.cli.mactl.grpc_tools.FileDescriptorProto")
    def test_get_service_descriptors_success(
        self, mock_file_descriptor_proto_class, mock_stub_class
    ):
        """Test `get_service_descriptors()` function with valid service."""
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
        result = list(
            get_service_descriptors(mock_channel, service_name, self.metadata)
        )

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

    @patch("michelangelo.cli.mactl.grpc_tools.reflection_pb2_grpc.ServerReflectionStub")
    @patch("michelangelo.cli.mactl.grpc_tools.FileDescriptorProto")
    def test_get_service_descriptors_multiple_descriptors(
        self, mock_file_descriptor_proto_class, mock_stub_class
    ):
        """Test `get_service_descriptors()` with multiple file descriptors.

        Mimics the actual protobuf structure with message types, fields,
        service descriptors, and methods.
        """
        # Create mock channel
        mock_channel = Mock()
        service_name = "michelangelo.api.v2.ProjectService"

        mock_fd_instance1 = _get_project_svc_mock()

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
        result = list(
            get_service_descriptors(mock_channel, service_name, self.metadata)
        )

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

    @patch("michelangelo.cli.mactl.grpc_tools.reflection_pb2_grpc.ServerReflectionStub")
    def test_get_service_descriptors_empty_response(self, mock_stub_class):
        """Test `get_service_descriptors()` function with empty response."""
        # Create mock channel
        mock_channel = Mock()
        service_name = "michelangelo.api.v2.EmptyService"

        # Create mock stub with empty response
        mock_stub = Mock()
        mock_stub.ServerReflectionInfo.return_value = []
        mock_stub_class.return_value = mock_stub

        # Call the function
        result = list(
            get_service_descriptors(mock_channel, service_name, self.metadata)
        )

        # Verify the result is empty
        self.assertEqual(len(result), 0)


class GetAllFileDescriptorsByFilenameTest(TestCase):
    """Tests for `get_all_file_descriptors_by_filename()` function."""

    def setUp(self):
        """Set up common test data."""
        self.metadata: list = [
            ("rpc-caller", "grpcurl"),
            ("rpc-service", "ma-apiserver"),
            ("rpc-encoding", "proto"),
        ]

    @patch("michelangelo.cli.mactl.grpc_tools.reflection_pb2_grpc.ServerReflectionStub")
    @patch("michelangelo.cli.mactl.grpc_tools.FileDescriptorProto")
    def test_no_dependencies(self, mock_fd_proto_class, mock_stub_class):
        """Test `get_all_file_descriptors_by_filename()` with no dependencies.

        Tests a file that has no dependencies.
        """
        # Create mock channel
        mock_channel = Mock()
        filename = "simple.proto"

        # Create mock file descriptor with no dependencies
        mock_fd = Mock()
        mock_fd.name = filename
        mock_fd.dependency = []
        mock_fd_proto_class.return_value = mock_fd

        # Create mock response
        mock_fd_bytes = b"mock_fd_bytes"
        mock_response = Mock()
        mock_response.file_descriptor_response.file_descriptor_proto = [mock_fd_bytes]

        # Create mock stub
        mock_stub = Mock()
        mock_stub.ServerReflectionInfo.return_value = [mock_response]
        mock_stub_class.return_value = mock_stub

        # Call the function
        result = list(
            get_all_file_descriptors_by_filename(mock_channel, filename, self.metadata)
        )

        # Verify result
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0], mock_fd)

        # Verify request was created correctly
        call_args = mock_stub.ServerReflectionInfo.call_args
        request_iter = call_args[0][0]
        request_list = list(request_iter)
        self.assertEqual(len(request_list), 1)
        self.assertEqual(request_list[0].file_by_filename, filename)

        # Verify ParseFromString was called
        mock_fd.ParseFromString.assert_called_once_with(mock_fd_bytes)

    @patch("michelangelo.cli.mactl.grpc_tools.reflection_pb2_grpc.ServerReflectionStub")
    @patch("michelangelo.cli.mactl.grpc_tools.FileDescriptorProto")
    def test_with_dependencies(self, mock_fd_proto_class, mock_stub_class):
        """Test `get_all_file_descriptors_by_filename()` with dependencies.

        Mimics the actual behavior from the log where:
        - project_svc.proto depends on: k8s generated.proto, list.proto,
          project.proto
        - k8s generated.proto depends on: runtime/generated.proto,
          runtime/schema/generated.proto
        - list.proto depends on: google/protobuf/any.proto
        - project.proto depends on: timestamp.proto, any.proto (visited),
          k8s generated.proto (visited), options.proto, git.proto.
        """
        # Create mock channel
        mock_channel = Mock()
        main_filename = "michelangelo/api/v2/project_svc.proto"

        # Create mock file descriptors with dependency tree
        # Main file: project_svc.proto
        mock_fd_main = _get_project_svc_mock()

        # Dependency 1: k8s generated.proto
        mock_fd_k8s = Mock()
        mock_fd_k8s.name = "k8s.io/apimachinery/pkg/apis/meta/v1/generated.proto"
        mock_fd_k8s.dependency = [
            "k8s.io/apimachinery/pkg/runtime/generated.proto",
            "k8s.io/apimachinery/pkg/runtime/schema/generated.proto",
        ]

        # Dependency 1.1: runtime/generated.proto (no deps)
        mock_fd_runtime = Mock()
        mock_fd_runtime.name = "k8s.io/apimachinery/pkg/runtime/generated.proto"
        mock_fd_runtime.dependency = []

        # Dependency 1.2: runtime/schema/generated.proto (no deps)
        mock_fd_schema = Mock()
        mock_fd_schema.name = "k8s.io/apimachinery/pkg/runtime/schema/generated.proto"
        mock_fd_schema.dependency = []

        # Dependency 2: list.proto
        mock_fd_list = Mock()
        mock_fd_list.name = "michelangelo/api/list.proto"
        mock_fd_list.dependency = ["google/protobuf/any.proto"]

        # Dependency 2.1: any.proto (no deps)
        mock_fd_any = Mock()
        mock_fd_any.name = "google/protobuf/any.proto"
        mock_fd_any.dependency = []

        # Dependency 3: project.proto
        mock_fd_project = Mock()
        mock_fd_project.name = "michelangelo/api/v2/project.proto"
        mock_fd_project.dependency = [
            "google/protobuf/timestamp.proto",
            "google/protobuf/any.proto",  # Already visited
            "k8s.io/apimachinery/pkg/apis/meta/v1/generated.proto",  # Already visited
            "michelangelo/api/options.proto",
            "michelangelo/api/v2/git.proto",
        ]

        # Dependency 3.1: timestamp.proto (no deps)
        mock_fd_timestamp = Mock()
        mock_fd_timestamp.name = "google/protobuf/timestamp.proto"
        mock_fd_timestamp.dependency = []

        # Dependency 3.2: options.proto
        mock_fd_options = Mock()
        mock_fd_options.name = "michelangelo/api/options.proto"
        mock_fd_options.dependency = ["google/protobuf/descriptor.proto"]

        # Dependency 3.2.1: descriptor.proto (no deps)
        mock_fd_descriptor = Mock()
        mock_fd_descriptor.name = "google/protobuf/descriptor.proto"
        mock_fd_descriptor.dependency = []

        # Dependency 3.3: git.proto
        mock_fd_git = Mock()
        mock_fd_git.name = "michelangelo/api/v2/git.proto"
        mock_fd_git.dependency = ["michelangelo/api/options.proto"]  # Already visited

        # Create a mapping of filenames to their mock file descriptors
        mock_fd_map = {
            "google/protobuf/any.proto": mock_fd_any,
            "google/protobuf/descriptor.proto": mock_fd_descriptor,
            "google/protobuf/timestamp.proto": mock_fd_timestamp,
            "k8s.io/apimachinery/pkg/apis/meta/v1/generated.proto": mock_fd_k8s,
            "k8s.io/apimachinery/pkg/runtime/generated.proto": mock_fd_runtime,
            "k8s.io/apimachinery/pkg/runtime/schema/generated.proto": mock_fd_schema,
            "michelangelo/api/list.proto": mock_fd_list,
            "michelangelo/api/options.proto": mock_fd_options,
            "michelangelo/api/v2/git.proto": mock_fd_git,
            "michelangelo/api/v2/project.proto": mock_fd_project,
            main_filename: mock_fd_main,
        }

        # Create mock responses for each file
        mock_responses = {
            "google/protobuf/any.proto": b"any_bytes",
            "google/protobuf/descriptor.proto": b"descriptor_bytes",
            "google/protobuf/timestamp.proto": b"timestamp_bytes",
            "k8s.io/apimachinery/pkg/apis/meta/v1/generated.proto": b"k8s_bytes",
            "k8s.io/apimachinery/pkg/runtime/generated.proto": b"runtime_bytes",
            "k8s.io/apimachinery/pkg/runtime/schema/generated.proto": b"schema_bytes",
            "michelangelo/api/list.proto": b"list_bytes",
            "michelangelo/api/options.proto": b"options_bytes",
            "michelangelo/api/v2/git.proto": b"git_bytes",
            "michelangelo/api/v2/project.proto": b"project_bytes",
            main_filename: b"main_bytes",
        }

        # Track requested filenames to verify correct requests
        requested_filenames = []

        # Setup FileDescriptorProto to return the correct mock based on context
        # We need to track which file is being requested in the stub call
        current_filename = [None]  # Use list to allow modification in nested function

        def fd_proto_factory():
            """Return the appropriate FileDescriptorProto mock.

            Based on current request.
            """
            filename = current_filename[0]
            if filename in mock_fd_map:
                return mock_fd_map[filename]
            raise ValueError(f"Unexpected filename: {filename}")

        mock_fd_proto_class.side_effect = lambda: fd_proto_factory()

        # Create mock stub
        mock_stub = Mock()

        def create_response(filename):
            """Helper to create a mock response for a given filename."""
            response = Mock()
            response.file_descriptor_response.file_descriptor_proto = [
                mock_responses[filename]
            ]
            return [response]

        # Setup stub to return appropriate responses based on request
        def stub_info_side_effect(request_iter, metadata):
            del metadata  # Unused in this mock
            request_list = list(request_iter)
            self.assertEqual(len(request_list), 1)

            # Extract and verify the filename from the request
            filename = request_list[0].file_by_filename
            self.assertIn(filename, mock_responses.keys())

            # Track the request
            requested_filenames.append(filename)

            # Set current filename so FileDescriptorProto factory knows which
            # mock to return
            current_filename[0] = filename

            return create_response(filename)

        mock_stub.ServerReflectionInfo.side_effect = stub_info_side_effect
        mock_stub_class.return_value = mock_stub

        ### Call the function
        result = list(
            get_all_file_descriptors_by_filename(
                mock_channel, main_filename, self.metadata
            )
        )

        ### Verification
        # Verify result - should return 11 file descriptors (matching the log)
        self.assertEqual(len(result), 11)

        # Verify the order: dependencies are yielded before their dependents
        result_names = [fd.name for fd in result]

        # runtime/generated.proto should come before k8s generated.proto
        self.assertLess(
            result_names.index("k8s.io/apimachinery/pkg/runtime/generated.proto"),
            result_names.index("k8s.io/apimachinery/pkg/apis/meta/v1/generated.proto"),
        )

        # any.proto should come before list.proto
        self.assertLess(
            result_names.index("google/protobuf/any.proto"),
            result_names.index("michelangelo/api/list.proto"),
        )

        # All dependencies should come before main file
        self.assertEqual(result_names[-1], main_filename)

        # Verify that visited set prevented duplicate processing
        # any.proto and k8s generated.proto appear in multiple dependency lists
        # but should only be in result once
        self.assertEqual(result_names.count("google/protobuf/any.proto"), 1)
        self.assertEqual(
            result_names.count("k8s.io/apimachinery/pkg/apis/meta/v1/generated.proto"),
            1,
        )

        # Verify all expected files are in the result
        expected_files = set(mock_responses.keys())
        actual_files = set(result_names)
        self.assertEqual(actual_files, expected_files)

        # Verify that each requested filename corresponds to an expected file
        self.assertEqual(len(requested_filenames), 11)
        for filename in requested_filenames:
            self.assertIn(filename, mock_responses.keys())

        # Verify that all expected files were requested
        self.assertEqual(set(requested_filenames), expected_files)

    @patch("michelangelo.cli.mactl.grpc_tools.reflection_pb2_grpc.ServerReflectionStub")
    @patch("michelangelo.cli.mactl.grpc_tools.FileDescriptorProto")
    def test_recursion_depth_limit(self, mock_fd_proto_class, mock_stub_class):
        """Test `get_all_file_descriptors_by_filename()` raises RecursionError.

        Tests that RecursionError is raised when depth exceeds limit.
        Starts at deps=999 and verifies it fails when processing
        a dependency (depth becomes 1000).
        """
        # Create mock channel
        mock_channel = Mock()
        filename = "deep.proto"

        # Create mock file descriptor with one dependency
        mock_fd = Mock()
        mock_fd.name = filename
        mock_fd.dependency = [
            "dependency.proto"
        ]  # This will cause depth to increase to 1000
        mock_fd_proto_class.return_value = mock_fd

        # Create mock response
        mock_fd_bytes = b"mock_fd_bytes"
        mock_response = Mock()
        mock_response.file_descriptor_response.file_descriptor_proto = [mock_fd_bytes]

        # Create mock stub
        mock_stub = Mock()
        mock_stub.ServerReflectionInfo.return_value = [mock_response]
        mock_stub_class.return_value = mock_stub

        # Call with deps=999, which should process fine initially,
        # but fail when trying to process the dependency at depth 1000
        with self.assertRaises(RecursionError) as context:
            list(
                get_all_file_descriptors_by_filename(
                    mock_channel, filename, self.metadata, deps=999
                )
            )

        # Verify the error message
        self.assertIn("Maximum recursion depth exceeded", str(context.exception))


class GetMethodsFromServiceTest(TestCase):
    """Tests for get_methods_from_service function."""

    def setUp(self):
        """Set up common test data."""
        self.metadata: list = [
            ("rpc-caller", "grpcurl"),
            ("rpc-service", "ma-apiserver"),
            ("rpc-encoding", "proto"),
        ]

    def test_get_methods_from_service_success(self):
        """Test get_methods_from_service returns methods and pool."""
        # Don't mock internal functions to get code coverage
        mock_channel = Mock()
        service_name = "michelangelo.api.v2beta1.PipelineRunService"

        # Create mock method
        mock_method = Mock()
        mock_method.name = "CreatePipelineRun"
        mock_method.input_type = ".michelangelo.api.v2beta1.CreatePipelineRunRequest"
        mock_method.output_type = ".michelangelo.api.v2beta1.PipelineRun"

        # Create mock service with methods
        mock_service = Mock()
        mock_service.name = "PipelineRunService"
        mock_service.method = [mock_method]

        # Create mock file descriptor
        mock_fd = Mock()
        mock_fd.name = "pipeline_run_svc.proto"
        mock_fd.service = [mock_service]

        # Mock only the dependencies, not the function under test
        with patch(
            "michelangelo.cli.mactl.grpc_tools.get_service_descriptors"
        ) as mock_get_descriptors, patch(
            "michelangelo.cli.mactl.grpc_tools.retrieve_full_descriptor_pool"
        ) as mock_retrieve_pool:
            mock_get_descriptors.return_value = [mock_fd]
            mock_pool = Mock()
            mock_retrieve_pool.return_value = mock_pool

            # Call the function - this will execute the actual code
            methods, pool = get_methods_from_service(
                mock_channel, service_name, self.metadata
            )

            # Verify results
            self.assertEqual(len(methods), 1)
            self.assertIn("CreatePipelineRun", methods)
            self.assertEqual(methods["CreatePipelineRun"], mock_method)
            self.assertEqual(pool, mock_pool)
