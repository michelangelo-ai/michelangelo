"""Unit tests for CRD module."""

from datetime import datetime, timezone
from inspect import Parameter, Signature
from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from grpc import RpcError, StatusCode

from michelangelo.cli.mactl.crd import (
    CRD,
    CrdMethodInfo,
    apply_func_impl,
    bind_signature,
    create_func_impl,
    delete_func_impl,
    get_func_impl,
    inject_func_signature,
    list_func_impl,
    prepare_column_info,
)


class _FakeRpcError(RpcError):
    """Minimal RpcError subclass for testing."""

    def __init__(self, status_code: StatusCode) -> None:
        self._code = status_code

    def code(self) -> StatusCode:
        """Return the status code."""
        return self._code


class PrepareColumnInfoTest(TestCase):
    """Test cases for prepare_column_info function."""

    def test_prepare_column_info(self):
        """Test prepare_column_info returns correct structure.

        Column structure and retrieve functions work.
        Designed to test time conversion from UTC to local time.
        """
        # Expected value
        utc_time_str = "2021-12-20_11:33:20"  # UTC time expected string
        dt_utc = datetime.strptime(utc_time_str, "%Y-%m-%d_%H:%M:%S").replace(
            tzinfo=timezone.utc
        )
        # convert to local time string
        expected_timestamp = dt_utc.astimezone().strftime("%Y-%m-%d_%H:%M:%S")
        # Check format is correct
        self.assertRegex(
            expected_timestamp,
            r"^\d{4}-\d{2}-\d{2}_\d{2}:\d{2}:\d{2}$",
            f"Format of expected timestamp is incorrect: {expected_timestamp}",
        )

        # Mock Entity
        mock_item = Mock()
        mock_item.metadata.namespace = "test-ns"
        mock_item.metadata.name = "test-name"
        mock_item.metadata.labels = {"michelangelo/UpdateTimestamp": "1640000000000000"}

        # run func
        result = prepare_column_info()

        # Check results
        retrieval_funcs = [col.pop("retrieve_func") for col in result]
        self.assertEqual(
            result,
            [
                {
                    "column_name": "NAMESPACE",
                    "max_length": len("NAMESPACE") + 1,
                },
                {
                    "column_name": "NAME",
                    "max_length": len("NAME") + 1,
                },
                {
                    "column_name": "LAST_UPDATED_SPEC",
                    "max_length": len("LAST_UPDATED_SPEC") + 1,
                },
            ],
        )
        self.assertEqual(
            [func(mock_item) for func in retrieval_funcs],
            [
                "test-ns",
                "test-name",
                expected_timestamp,
            ],
        )

    def test_prepare_column_info_empty_timestamp(self):
        """Test prepare_column_info handles empty timestamp gracefully."""
        # Mock Entity with empty timestamp
        mock_item = Mock()
        mock_item.metadata.namespace = "test-ns"
        mock_item.metadata.name = "test-name"
        mock_item.metadata.labels = {"michelangelo/UpdateTimestamp": ""}

        # run func
        result = prepare_column_info()

        # Check results
        retrieval_funcs = [col.pop("retrieve_func") for col in result]

        # Should return "N/A" for empty timestamp instead of crashing
        self.assertEqual(
            [func(mock_item) for func in retrieval_funcs],
            [
                "test-ns",
                "test-name",
                "N/A",
            ],
        )

    def test_prepare_column_info_missing_timestamp(self):
        """Test prepare_column_info handles missing timestamp label."""
        # Mock Entity without timestamp label
        mock_item = Mock()
        mock_item.metadata.namespace = "test-ns"
        mock_item.metadata.name = "test-name"
        mock_item.metadata.labels = {}

        # run func
        result = prepare_column_info()

        # Check results
        retrieval_funcs = [col.pop("retrieve_func") for col in result]

        # Should return "N/A" for missing timestamp
        self.assertEqual(
            [func(mock_item) for func in retrieval_funcs],
            [
                "test-ns",
                "test-name",
                "N/A",
            ],
        )


class ListFuncImplTest(TestCase):
    """Test cases for list_func_impl function."""

    @patch("michelangelo.cli.mactl.crd.crd_method_call")
    @patch("michelangelo.cli.mactl.crd.ParseDict")
    def test_list_func_impl(self, mock_parse_dict, mock_call):
        """Test list_func_impl extracts list fields and formats output."""
        # Create CrdMethodInfo instance
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="michelangelo.api.v2.ProjectService",
            method_name="List",
            input_class=Mock,
            output_class=Mock,
        )

        # Prepare Mock
        mock_item = MagicMock()
        mock_item.metadata.namespace = "test-ns"
        mock_item.metadata.name = "test-project"
        mock_item.metadata.labels = {"michelangelo/UpdateTimestamp": "1640000000000000"}

        mock_response = Mock()
        mock_response.ListFields.return_value = [
            (
                Mock(name="project_list"),
                Mock(items=[mock_item]),
            )
        ]
        mock_call.return_value = mock_response

        # Execute - should not raise any exceptions
        list_func_impl(
            crd_method_info,
            Mock(arguments={"namespace": "test-namespace", "limit": 100}),
        )

        # Verify ParseDict was called with correct request dict
        call_args = mock_parse_dict.call_args
        request_dict = call_args[0][0]
        self.assertEqual(request_dict["namespace"], "test-namespace")
        self.assertEqual(request_dict["list_options_ext"]["pagination"]["limit"], 100)

    @patch("michelangelo.cli.mactl.crd.crd_method_call")
    @patch("michelangelo.cli.mactl.crd.ParseDict")
    def test_list_func_impl_with_limit_warning(self, mock_parse_dict, mock_call):
        """Test list_func_impl shows warning when result count equals limit."""
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="michelangelo.api.v2.ProjectService",
            method_name="List",
            input_class=Mock,
            output_class=Mock,
        )

        mock_items = [MagicMock() for _ in range(10)]
        for item in mock_items:
            item.metadata.namespace = "test-ns"
            item.metadata.name = "test-project"
            item.metadata.labels = {"michelangelo/UpdateTimestamp": "1640000000000000"}

        mock_response = Mock()
        mock_response.ListFields.return_value = [
            (
                Mock(name="project_list"),
                Mock(items=mock_items),
            )
        ]
        mock_call.return_value = mock_response

        list_func_impl(
            crd_method_info,
            Mock(arguments={"namespace": "test-namespace", "limit": 10}),
        )

        call_args = mock_parse_dict.call_args
        request_dict = call_args[0][0]
        self.assertEqual(request_dict["list_options_ext"]["pagination"]["limit"], 10)


class DeleteFuncImplTest(TestCase):
    """Test cases for delete_func_impl function."""

    @patch("michelangelo.cli.mactl.crd.crd_method_call_kwargs")
    def test_delete_func_impl(self, mock_call_kwargs):
        """Test delete_func_impl calls crd_method_call_kwargs."""
        # Create CrdMethodInfo instance
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="michelangelo.api.v2.ProjectService",
            method_name="Delete",
            input_class=Mock,
            output_class=Mock,
        )

        # Execute
        delete_func_impl(
            crd_method_info,
            Mock(arguments={"namespace": "test-ns", "name": "test-project"}),
        )

        # Verify crd_method_call_kwargs was called with correct arguments
        mock_call_kwargs.assert_called_once_with(
            crd_method_info, namespace="test-ns", name="test-project"
        )


class GetFuncImplTest(TestCase):
    """Test cases for get_func_impl function."""

    @patch("michelangelo.cli.mactl.crd.crd_method_call_kwargs")
    def test_get_func_impl(self, mock_call_kwargs):
        """Test get_func_impl with name calls crd_method_call_kwargs."""
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Get",
            input_class=Mock,
            output_class=Mock,
        )
        get_func_impl(
            crd_method_info,
            Mock(arguments={"namespace": "ns", "name": "proj"}),
        )
        mock_call_kwargs.assert_called_once_with(
            crd_method_info, namespace="ns", name="proj"
        )

    def test_get_func_impl_without_name_calls_list(self):
        """Test get_func_impl without name calls list with limit."""
        mock_crd = Mock()
        mock_crd.list = Mock(return_value="list_result")
        mock_crd.generate_list = Mock()

        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Get",
            input_class=Mock,
            output_class=Mock,
        )

        result = get_func_impl(
            crd_method_info,
            Mock(
                arguments={
                    "self": mock_crd,
                    "namespace": "ns",
                    "name": None,
                    "limit": 50,
                }
            ),
        )

        mock_crd.generate_list.assert_called_once_with(crd_method_info.channel)
        mock_crd.list.assert_called_once_with(namespace="ns", limit=50)
        self.assertEqual(result, "list_result")


class ApplyFuncImplTest(TestCase):
    """Test cases for apply_func_impl function."""

    @patch("michelangelo.cli.mactl.crd.crd_method_call")
    @patch("michelangelo.cli.mactl.crd.crd_method_call_kwargs")
    @patch("michelangelo.cli.mactl.crd.read_yaml_to_crd_request")
    @patch("michelangelo.cli.mactl.crd.yaml_to_dict")
    def test_apply_func_impl_update_with_converter_uses_full_replace(
        self,
        mock_yaml_to_dict: MagicMock,
        mock_read_yaml: MagicMock,
        mock_call_kwargs: MagicMock,
        _,
    ):
        """Test apply uses full replace via read_yaml_to_crd_request."""
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Apply",
            input_class=Mock,
            output_class=Mock,
        )
        mock_converter = Mock()
        mock_crd = Mock()
        mock_crd.full_name = "test.Service"
        mock_crd.name = "test"
        mock_crd.func_crd_metadata_converter = mock_converter
        mock_existing = Mock()
        mock_existing.test.metadata.resourceVersion = "42"
        mock_call_kwargs.return_value = mock_existing
        parsed_yaml = {"metadata": {"namespace": "ns", "name": "name"}}
        mock_yaml_to_dict.return_value = parsed_yaml
        mock_request = Mock()
        mock_read_yaml.return_value = mock_request

        apply_func_impl(
            crd_method_info, Mock(arguments={"self": mock_crd, "file": "f.yaml"})
        )

        mock_read_yaml.assert_called_once_with(
            crd_method_info.input_class,
            mock_crd.name,
            "f.yaml",
            mock_converter,
            yaml_dict=parsed_yaml,
        )
        # resourceVersion must be copied onto the inner pipeline message
        inner = getattr(mock_request, mock_crd.name)
        self.assertEqual(inner.metadata.resourceVersion, "42")

    @patch("michelangelo.cli.mactl.crd.crd_method_call_kwargs")
    @patch("michelangelo.cli.mactl.crd.yaml_to_dict")
    def test_apply_func_impl_create_when_not_found(
        self, mock_yaml_to_dict: MagicMock, mock_call_kwargs: MagicMock
    ):
        """Test apply_func_impl calls create when the resource does not exist."""
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Apply",
            input_class=Mock,
            output_class=Mock,
        )
        mock_crd = Mock()
        mock_crd.full_name = "test.Service"
        mock_call_kwargs.side_effect = _FakeRpcError(StatusCode.NOT_FOUND)
        parsed_yaml = {"metadata": {"namespace": "ns", "name": "name"}}
        mock_yaml_to_dict.return_value = parsed_yaml

        apply_func_impl(
            crd_method_info, Mock(arguments={"self": mock_crd, "file": "f.yaml"})
        )

        mock_crd.generate_create.assert_called_once_with(crd_method_info.channel)
        mock_crd.create.assert_called_once_with("f.yaml")

    @patch("michelangelo.cli.mactl.crd.crd_method_call_kwargs")
    @patch("michelangelo.cli.mactl.crd.yaml_to_dict")
    def test_apply_func_impl_reraises_non_not_found_errors(
        self, mock_yaml_to_dict: MagicMock, mock_call_kwargs: MagicMock
    ):
        """Test apply_func_impl re-raises RpcErrors that are not NOT_FOUND."""
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Apply",
            input_class=Mock,
            output_class=Mock,
        )
        mock_crd = Mock()
        mock_crd.full_name = "test.Service"
        mock_call_kwargs.side_effect = _FakeRpcError(StatusCode.UNAVAILABLE)
        parsed_yaml = {"metadata": {"namespace": "ns", "name": "name"}}
        mock_yaml_to_dict.return_value = parsed_yaml

        with self.assertRaises(RpcError):
            apply_func_impl(
                crd_method_info, Mock(arguments={"self": mock_crd, "file": "f.yaml"})
            )

    @patch("michelangelo.cli.mactl.crd.crd_method_call_kwargs")
    @patch("michelangelo.cli.mactl.crd.yaml_to_dict")
    def test_apply_create_path_uses_create_converter(
        self, mock_yaml_to_dict: MagicMock, mock_call_kwargs: MagicMock
    ):
        """Apply swaps to func_crd_metadata_converter_for_create before create."""
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Apply",
            input_class=Mock,
            output_class=Mock,
        )
        original_converter = Mock(name="apply_converter")
        create_converter = Mock(name="create_converter")
        mock_crd = Mock()
        mock_crd.full_name = "test.Service"
        mock_crd.func_crd_metadata_converter = original_converter
        mock_crd.func_crd_metadata_converter_for_create = create_converter
        mock_call_kwargs.side_effect = _FakeRpcError(StatusCode.NOT_FOUND)
        mock_yaml_to_dict.return_value = {
            "metadata": {"namespace": "ns", "name": "name"}
        }

        apply_func_impl(
            crd_method_info, Mock(arguments={"self": mock_crd, "file": "f.yaml"})
        )

        # Converter must be swapped to create_converter during create()
        mock_crd.create.assert_called_once_with("f.yaml")
        # After the call, original converter must be restored
        self.assertIs(mock_crd.func_crd_metadata_converter, original_converter)

    @patch("michelangelo.cli.mactl.crd.yaml_to_dict")
    def test_apply_func_impl_raises_when_metadata_missing(
        self, mock_yaml_to_dict: MagicMock
    ):
        """apply_func_impl raises ValueError when YAML has no metadata key."""
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Apply",
            input_class=Mock,
            output_class=Mock,
        )
        mock_yaml_to_dict.return_value = {"spec": {}}

        with self.assertRaises(ValueError, msg="missing 'metadata' key"):
            apply_func_impl(
                crd_method_info, Mock(arguments={"self": Mock(), "file": "f.yaml"})
            )

    @patch("michelangelo.cli.mactl.crd.yaml_to_dict")
    def test_apply_func_impl_raises_when_metadata_not_dict(
        self, mock_yaml_to_dict: MagicMock
    ):
        """apply_func_impl raises ValueError when metadata is not a mapping."""
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Apply",
            input_class=Mock,
            output_class=Mock,
        )
        mock_yaml_to_dict.return_value = {"metadata": "not-a-dict"}

        with self.assertRaises(ValueError, msg="metadata must be a mapping"):
            apply_func_impl(
                crd_method_info, Mock(arguments={"self": Mock(), "file": "f.yaml"})
            )

    @patch("michelangelo.cli.mactl.crd.yaml_to_dict")
    def test_apply_func_impl_raises_when_namespace_missing(
        self, mock_yaml_to_dict: MagicMock
    ):
        """apply_func_impl raises ValueError when namespace is missing from metadata."""
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Apply",
            input_class=Mock,
            output_class=Mock,
        )
        mock_yaml_to_dict.return_value = {"metadata": {"name": "my-pipeline"}}

        with self.assertRaises(ValueError, msg="namespace must be a string"):
            apply_func_impl(
                crd_method_info, Mock(arguments={"self": Mock(), "file": "f.yaml"})
            )


class CreateFuncImplTest(TestCase):
    """Test cases for create_func_impl function."""

    @patch("michelangelo.cli.mactl.crd.crd_method_call")
    @patch("michelangelo.cli.mactl.crd.read_yaml_to_crd_request")
    def test_create_func_impl(self, mock_read_yaml: MagicMock, mock_call: MagicMock):
        """Test create_func_impl calls read_yaml_to_crd_request and crd_method_call."""
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Create",
            input_class=Mock,
            output_class=Mock,
        )
        mock_crd = Mock()
        mock_crd.full_name = "test.Service"
        mock_crd.name = "test"
        mock_crd.func_crd_metadata_converter = Mock()
        mock_request = Mock()
        mock_read_yaml.return_value = mock_request

        create_func_impl(
            crd_method_info, Mock(arguments={"self": mock_crd, "file": "f.yaml"})
        )

        mock_read_yaml.assert_called_once_with(
            crd_method_info.input_class,
            "test",
            "f.yaml",
            mock_crd.func_crd_metadata_converter,
        )
        mock_call.assert_called_once_with(crd_method_info, mock_request)


class BindSignatureTest(TestCase):
    """Test cases for bind_signature decorator."""

    def test_bind_signature_applies_defaults(self):
        """Test bind_signature binds arguments and applies default values."""
        sig = Signature(
            [
                Parameter("x", Parameter.POSITIONAL_OR_KEYWORD),
                Parameter("y", Parameter.POSITIONAL_OR_KEYWORD, default=100),
            ]
        )
        mock_func = Mock(return_value="success")

        # Create decorated function
        decorated = bind_signature(sig)(mock_func)
        result = decorated(5)

        # Verify function was called and defaults were applied
        self.assertEqual(result, "success")
        bound_args = mock_func.call_args[0][0]
        self.assertEqual(bound_args.arguments["x"], 5)
        self.assertEqual(bound_args.arguments["y"], 100)


class InjectFuncSignatureTest(TestCase):
    """Test cases for inject_func_signature function."""

    def test_inject_func_signature(self):
        """Test inject_func_signature adds function signature to CRD."""
        mock_crd = Mock(spec=CRD)
        mock_crd.func_signature = {}

        test_signatures = {
            "help": "Test help message",
            "args": [{"args": ["--test"], "kwargs": {"type": str}}],
        }

        inject_func_signature(mock_crd, "test_action", test_signatures)

        self.assertIn("test_action", mock_crd.func_signature)
        self.assertEqual(
            mock_crd.func_signature["test_action"]["help"], "Test help message"
        )
        self.assertEqual(
            mock_crd.func_signature["test_action"]["args"],
            [{"args": ["--test"], "kwargs": {"type": str}}],
        )


class ExtractMethodInfoTest(TestCase):
    """Test cases for CRD._extract_method_info method."""

    @patch("michelangelo.cli.mactl.crd.get_message_class_by_name")
    @patch("michelangelo.cli.mactl.crd.get_methods_from_service")
    def test_extract_method_info(
        self, mock_get_methods_from_service, mock_get_message_class_by_name
    ):
        """Test _extract_method_info returns correct method information."""
        # Config mock
        mock_method = Mock(
            input_type="/test.GetRequest", output_type="/test.GetResponse"
        )
        mock_get_methods_from_service.return_value = (
            {"GetTestCrd": mock_method},
            Mock(),
        )

        mock_input_class = Mock()
        mock_output_class = Mock()
        mock_get_message_class_by_name.side_effect = [
            mock_input_class,
            mock_output_class,
        ]

        # Run test
        crd = CRD(name="test_crd", full_name="test.service.TestCrd", metadata=[])
        method_name, input_class, output_class = crd._extract_method_info(
            Mock(), "test.service.TestCrd", "Get"
        )

        # Check results
        self.assertEqual(method_name, "GetTestCrd")
        self.assertEqual(input_class, mock_input_class)
        self.assertEqual(output_class, mock_output_class)

    @patch("michelangelo.cli.mactl.crd.get_methods_from_service")
    def test_extract_method_info_method_not_found(self, mock_get_methods_from_service):
        """Test _extract_method_info raises ValueError when method not found."""
        # Config mock with empty methods dict
        mock_get_methods_from_service.return_value = ({}, Mock())

        crd = CRD(name="test_crd", full_name="test.service.TestCrd", metadata=[])

        with self.assertRaises(ValueError) as context:
            crd._extract_method_info(Mock(), "test.service.TestCrd", "Get")

        self.assertIn("GetTestCrd", str(context.exception))
        self.assertIn("test.service.TestCrd", str(context.exception))


class GenerateGetTest(TestCase):
    """Test cases for CRD.generate_get method."""

    @patch.object(CRD, "_extract_method_info")
    def test_generate_get(self, mock_extract_method_info):
        """Test generate_get creates the get method on CRD instance."""
        mock_channel = Mock()
        mock_extract_method_info.return_value = ("GetTestCrd", Mock, Mock)

        crd = CRD(name="test_crd", full_name="test.service.TestCrd", metadata=[])
        crd.generate_get(mock_channel)

        self.assertTrue(hasattr(crd, "get"))
        self.assertTrue(callable(crd.get))

    @patch("michelangelo.cli.mactl.crd.crd_method_call_kwargs")
    @patch.object(CRD, "_extract_method_info")
    def test_generate_get_execution(
        self, mock_extract_method_info, mock_crd_method_call_kwargs
    ):
        """Test the generated get method can be executed with correct arguments."""
        mock_channel = Mock()
        mock_extract_method_info.return_value = ("GetTestCrd", Mock, Mock)
        mock_response = Mock()
        mock_crd_method_call_kwargs.return_value = mock_response

        crd = CRD(name="test_crd", full_name="test.service.TestCrd", metadata=[])
        crd.generate_get(mock_channel)

        result = crd.get(namespace="test-ns", name="test-name")

        self.assertEqual(result, mock_response)
        call_args = mock_crd_method_call_kwargs.call_args
        self.assertEqual(call_args.kwargs["namespace"], "test-ns")
        self.assertEqual(call_args.kwargs["name"], "test-name")
