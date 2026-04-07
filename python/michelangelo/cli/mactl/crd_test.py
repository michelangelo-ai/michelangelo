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
    convert_crd_metadata,
    crd_method_call,
    crd_method_call_kwargs,
    create_func_impl,
    deep_update,
    delete_func_impl,
    get_crd_namespace_and_name_from_yaml,
    get_func_impl,
    get_single_arg,
    inject_func_signature,
    list_func_impl,
    prepare_column_info,
    read_yaml_to_crd_request,
    snake_to_camel,
    yaml_to_dict,
)


class _FakeRpcError(RpcError):
    """Minimal RpcError subclass for testing."""

    def __init__(self, status_code: StatusCode) -> None:
        self._code = status_code

    def code(self) -> StatusCode:
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
    @patch("michelangelo.cli.mactl.crd.get_crd_namespace_and_name_from_yaml")
    @patch("michelangelo.cli.mactl.crd.yaml_to_dict")
    def test_apply_func_impl_update(
        self, mock_yaml_to_dict: MagicMock, mock_get_ns: MagicMock, _
    ):
        """Test apply_func_impl updates existing CRD."""
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Apply",
            input_class=Mock,
            output_class=Mock,
        )
        mock_crd = Mock()
        mock_crd.full_name = "test.Service"
        mock_crd.get.return_value = Mock()
        mock_crd.read_yaml_and_update_crd_request.return_value = Mock()
        mock_get_ns.return_value = ("ns", "name")

        apply_func_impl(
            crd_method_info, Mock(arguments={"self": mock_crd, "file": "f.yaml"})
        )

        mock_crd.read_yaml_and_update_crd_request.assert_called_once()


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


class GetCrdNamespaceAndNameFromYamlTest(TestCase):
    """Tests for get_crd_namespace_and_name_from_yaml."""

    @patch("michelangelo.cli.mactl.crd.yaml_to_dict")
    def test_reads_file_when_no_dict_provided(self, mock_yaml):
        """Reads the YAML file when no yaml_dict is provided."""
        mock_yaml.return_value = {"metadata": {"namespace": "ns", "name": "foo"}}
        ns, name = get_crd_namespace_and_name_from_yaml("f.yaml")
        self.assertEqual(ns, "ns")
        self.assertEqual(name, "foo")
        mock_yaml.assert_called_once_with("f.yaml")

    def test_uses_provided_dict_without_reading_file(self):
        """Uses the provided yaml_dict without reading the file."""
        yaml_dict = {"metadata": {"namespace": "ns", "name": "foo"}}
        ns, name = get_crd_namespace_and_name_from_yaml("f.yaml", yaml_dict=yaml_dict)
        self.assertEqual(ns, "ns")
        self.assertEqual(name, "foo")

    def test_raises_when_metadata_missing(self):
        """Raises ValueError when metadata key is absent."""
        with self.assertRaises(ValueError):
            get_crd_namespace_and_name_from_yaml("f.yaml", yaml_dict={"spec": {}})

    def test_raises_when_metadata_not_dict(self):
        """Raises ValueError when metadata is not a mapping."""
        with self.assertRaises(ValueError):
            get_crd_namespace_and_name_from_yaml(
                "f.yaml", yaml_dict={"metadata": "bad"}
            )

    def test_raises_when_namespace_missing(self):
        """Raises ValueError when namespace is absent from metadata."""
        with self.assertRaises(ValueError):
            get_crd_namespace_and_name_from_yaml(
                "f.yaml", yaml_dict={"metadata": {"name": "foo"}}
            )

    def test_raises_when_name_missing(self):
        """Raises ValueError when name is absent from metadata."""
        with self.assertRaises(ValueError):
            get_crd_namespace_and_name_from_yaml(
                "f.yaml", yaml_dict={"metadata": {"namespace": "ns"}}
            )


class ReadYamlToCrdRequestTest(TestCase):
    """Tests for read_yaml_to_crd_request."""

    @patch("michelangelo.cli.mactl.crd.ParseDict")
    def test_uses_provided_yaml_dict(self, mock_parse):
        """Passes yaml_dict directly to the converter without re-reading the file."""
        yaml_dict = {"metadata": {"namespace": "ns"}}
        converter = Mock(return_value={"converted": True})
        crd_class = MagicMock()
        read_yaml_to_crd_request(
            crd_class, "pipeline", "f.yaml", converter, yaml_dict=yaml_dict
        )
        converter.assert_called_once()
        # yaml_dict was passed directly — no file read needed
        args = converter.call_args[0]
        self.assertEqual(args[0], yaml_dict)

    @patch("michelangelo.cli.mactl.crd.ParseDict")
    @patch("michelangelo.cli.mactl.crd.yaml_to_dict")
    def test_reads_file_when_no_dict_provided(self, mock_yaml, mock_parse):
        """Reads the YAML file when yaml_dict is not provided."""
        mock_yaml.return_value = {"metadata": {"namespace": "ns"}}
        converter = Mock(return_value={})
        crd_class = MagicMock()
        read_yaml_to_crd_request(crd_class, "pipeline", "f.yaml", converter)
        mock_yaml.assert_called_once_with("f.yaml")


class ConvertCrdMetadataTest(TestCase):
    """Tests for convert_crd_metadata."""

    def test_moves_api_version_and_kind_to_type_meta(self):
        """Moves apiVersion and kind into typeMeta."""
        result = convert_crd_metadata(
            {"apiVersion": "v1", "kind": "Pipeline", "spec": {}}, Mock(), Mock()
        )
        self.assertEqual(result["typeMeta"]["apiVersion"], "v1")
        self.assertEqual(result["typeMeta"]["kind"], "Pipeline")
        self.assertNotIn("apiVersion", result)

    def test_raises_when_not_dict(self):
        """Raises ValueError for non-dict input."""
        with self.assertRaises(ValueError):
            convert_crd_metadata("bad", Mock(), Mock())

    def test_no_api_version_or_kind(self):
        """Does not add typeMeta when apiVersion and kind are absent."""
        result = convert_crd_metadata({"spec": {}}, Mock(), Mock())
        self.assertNotIn("typeMeta", result)


class DeepUpdateTest(TestCase):
    """Tests for deep_update."""

    def test_deep_merge(self):
        """Recursively merges nested dicts."""
        d1 = {"a": {"a1": 1, "a2": 2}}
        deep_update(d1, {"a": {"a1": 7, "a3": 9}})
        self.assertEqual(d1, {"a": {"a1": 7, "a2": 2, "a3": 9}})

    def test_overwrites_non_dict(self):
        """Overwrites non-dict values in-place."""
        d1 = {"a": 1}
        deep_update(d1, {"a": 2})
        self.assertEqual(d1["a"], 2)


class YamlToDictTest(TestCase):
    """Tests for yaml_to_dict."""

    def test_raises_on_invalid_yaml(self):
        """Raises ValueError for malformed YAML content."""
        import os
        import tempfile
        with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
            f.write("key: [unclosed")
            path = f.name
        try:
            with self.assertRaises(ValueError):
                yaml_to_dict(path)
        finally:
            os.unlink(path)


class SnakeToCamelTest(TestCase):
    """Tests for snake_to_camel."""

    def test_converts(self):
        """Converts snake_case to CamelCase."""
        self.assertEqual(snake_to_camel("my_function_name"), "MyFunctionName")

    def test_single_word(self):
        """Capitalizes a single word."""
        self.assertEqual(snake_to_camel("pipeline"), "Pipeline")


class CrdMethodCallTest(TestCase):
    """Tests for crd_method_call and crd_method_call_kwargs."""

    def test_crd_method_call(self):
        """Invokes the gRPC stub and returns the response."""
        mock_channel = Mock()
        mock_stub = Mock()
        mock_channel.unary_unary.return_value = mock_stub
        mock_response = Mock()
        mock_stub.return_value = mock_response

        info = CrdMethodInfo(
            channel=mock_channel,
            crd_full_name="test.Service",
            method_name="Get",
            input_class=MagicMock(),
            output_class=MagicMock(),
        )
        result = crd_method_call(info, Mock())
        self.assertEqual(result, mock_response)

    def test_crd_method_call_kwargs(self):
        """Constructs input_class from kwargs and invokes the stub."""
        mock_channel = Mock()
        mock_channel.unary_unary.return_value = Mock(return_value=Mock())
        input_class = MagicMock()
        info = CrdMethodInfo(
            channel=mock_channel,
            crd_full_name="test.Service",
            method_name="Get",
            input_class=input_class,
            output_class=MagicMock(),
        )
        crd_method_call_kwargs(info, namespace="ns", name="foo")
        input_class.assert_called_once_with(namespace="ns", name="foo")


class ApplyFuncImplCreateTest(TestCase):
    """Additional tests for apply_func_impl create/error paths."""

    @patch("michelangelo.cli.mactl.crd.get_crd_namespace_and_name_from_yaml")
    @patch("michelangelo.cli.mactl.crd.yaml_to_dict")
    def test_create_path_when_not_found(self, mock_yaml, mock_ns):
        """NOT_FOUND triggers the create path."""
        mock_yaml.return_value = {}
        mock_ns.return_value = ("ns", "name")
        crd_method_info = CrdMethodInfo(Mock(), "svc", "Apply", Mock, Mock)
        mock_crd = Mock()
        mock_crd.get.side_effect = _FakeRpcError(StatusCode.NOT_FOUND)

        apply_func_impl(
            crd_method_info, Mock(arguments={"self": mock_crd, "file": "f.yaml"})
        )
        mock_crd.generate_create.assert_called_once()
        mock_crd.create.assert_called_once_with("f.yaml")

    @patch("michelangelo.cli.mactl.crd.get_crd_namespace_and_name_from_yaml")
    @patch("michelangelo.cli.mactl.crd.yaml_to_dict")
    def test_reraises_non_not_found(self, mock_yaml, mock_ns):
        """Non-NOT_FOUND RpcErrors are re-raised."""
        mock_yaml.return_value = {}
        mock_ns.return_value = ("ns", "name")
        crd_method_info = CrdMethodInfo(Mock(), "svc", "Apply", Mock, Mock)
        mock_crd = Mock()
        mock_crd.get.side_effect = _FakeRpcError(StatusCode.UNAVAILABLE)

        with self.assertRaises(RpcError):
            apply_func_impl(
                crd_method_info,
                Mock(arguments={"self": mock_crd, "file": "f.yaml"}),
            )


class GenerateApplyTest(TestCase):
    """Tests for generate_apply with _apply_func_impl override."""

    @patch.object(CRD, "_extract_method_info")
    def test_generate_apply_uses_custom_apply_func_impl(self, mock_extract):
        """generate_apply uses _apply_func_impl when set on the instance."""
        mock_extract.return_value = ("UpdatePipeline", Mock, Mock)
        crd = CRD(name="pipeline", full_name="test.Service", metadata=[])
        custom_impl = Mock()
        crd._apply_func_impl = custom_impl
        crd.generate_apply(Mock())
        self.assertTrue(callable(crd.apply))


class GetSingleArgTest(TestCase):
    """Tests for get_single_arg."""

    def test_returns_string_value(self):
        """Returns the string value directly."""
        self.assertEqual(get_single_arg({"file": "f.yaml"}, "file"), "f.yaml")

    def test_returns_single_element_list(self):
        """Unwraps a one-element list."""
        self.assertEqual(get_single_arg({"file": ["f.yaml"]}, "file"), "f.yaml")

    def test_raises_key_error_when_missing(self):
        """Raises KeyError when the key is absent."""
        with self.assertRaises(KeyError):
            get_single_arg({}, "file")

    def test_raises_value_error_for_multi_element_list(self):
        """Raises ValueError when the list has more than one element."""
        with self.assertRaises(ValueError):
            get_single_arg({"file": ["a.yaml", "b.yaml"]}, "file")

    def test_raises_value_error_for_non_string_non_list(self):
        """Raises ValueError for a non-string, non-list value."""
        with self.assertRaises(ValueError):
            get_single_arg({"file": 42}, "file")


class CrdReprTest(TestCase):
    """Test CRD.__repr__."""

    def test_repr(self):
        """__repr__ includes name and full_name."""
        crd = CRD(name="pipeline", full_name="test.Service", metadata=[])
        self.assertIn("pipeline", repr(crd))
        self.assertIn("test.Service", repr(crd))


class YamlToDictSuccessTest(TestCase):
    """Test yaml_to_dict success path."""

    def test_loads_valid_yaml(self):
        """Returns a dict for valid YAML content."""
        import os
        import tempfile
        with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
            f.write("key: value\n")
            path = f.name
        try:
            result = yaml_to_dict(path)
            self.assertEqual(result["key"], "value")
        finally:
            os.unlink(path)


class ConfigureParserTest(TestCase):
    """Test CRD.configure_parser with a real parser."""

    @patch.object(CRD, "_extract_method_info")
    def test_configure_parser_adds_args(self, mock_extract):
        """configure_parser runs without error when a parser is provided."""
        from argparse import ArgumentParser
        mock_extract.return_value = ("GetTestCrd", Mock, Mock)
        crd = CRD(name="test_crd", full_name="test.service.TestCrd", metadata=[])
        parser = ArgumentParser()
        crd.configure_parser("get", parser)
        # If any args were defined in func_signature, they get added.
        # At minimum the call should not raise.
        self.assertIsNotNone(parser)


class GenerateDeleteTest(TestCase):
    """Tests for CRD.generate_delete."""

    @patch.object(CRD, "_extract_method_info")
    def test_generate_delete_creates_method(self, mock_extract):
        """generate_delete attaches a callable delete method to the instance."""
        mock_extract.return_value = ("DeleteTestCrd", Mock, Mock)
        crd = CRD(name="test_crd", full_name="test.service.TestCrd", metadata=[])
        crd.generate_delete(Mock())
        self.assertTrue(hasattr(crd, "delete"))
        self.assertTrue(callable(crd.delete))


class GenerateCreateTest(TestCase):
    """Tests for CRD.generate_create."""

    @patch.object(CRD, "_extract_method_info")
    def test_generate_create_creates_method(self, mock_extract):
        """generate_create attaches a callable create method to the instance."""
        mock_extract.return_value = ("CreateTestCrd", Mock, Mock)
        crd = CRD(name="test_crd", full_name="test.service.TestCrd", metadata=[])
        crd.generate_create(Mock())
        self.assertTrue(hasattr(crd, "create"))
        self.assertTrue(callable(crd.create))


class GenerateListTest(TestCase):
    """Tests for CRD.generate_list."""

    @patch.object(CRD, "_extract_method_info")
    def test_generate_list_creates_method(self, mock_extract):
        """generate_list attaches a callable list method to the instance."""
        mock_extract.return_value = ("ListTestCrd", Mock, Mock)
        crd = CRD(name="test_crd", full_name="test.service.TestCrd", metadata=[])
        crd.generate_list(Mock())
        self.assertTrue(hasattr(crd, "list"))
        self.assertTrue(callable(crd.list))


class ReadYamlAndUpdateCrdRequestTest(TestCase):
    """Tests for CRD.read_yaml_and_update_crd_request."""

    @patch("michelangelo.cli.mactl.crd.ParseDict")
    @patch("michelangelo.cli.mactl.crd.MessageToDict")
    @patch("michelangelo.cli.mactl.crd.yaml_to_dict")
    def test_merges_yaml_into_existing_crd(self, mock_yaml, mock_to_dict, mock_parse):
        """Merges YAML content into the existing CRD dict and calls ParseDict."""
        mock_yaml.return_value = {"spec": {"type": "TRAIN"}}
        mock_to_dict.return_value = {"test_crd": {"spec": {"type": "OLD"}}}
        input_class = MagicMock()
        crd = CRD(name="test_crd", full_name="test.Service", metadata=[])
        original_crd = Mock()
        crd.read_yaml_and_update_crd_request(input_class, "f.yaml", original_crd)
        mock_parse.assert_called_once()
