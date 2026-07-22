"""Unit tests for CRD module."""

from datetime import datetime, timezone
from inspect import Parameter, Signature
from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from michelangelo.cli.mactl.crd import (
    CRD,
    CrdMethodInfo,
    _get_func_impl,
    _list_func_impl,
    apply_func_impl,
    bind_signature,
    create_func_impl,
    delete_func_impl,
    get_func_impl,
    inject_func_signature,
    list_func_impl,
    prepare_column_info,
)


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

    @patch("michelangelo.cli.mactl.crd.ParseDict")
    def test_list_func_impl(self, mock_parse_dict):
        """Test list_func_impl calls _self._list and formats output."""
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="michelangelo.api.v2.ProjectService",
            method_name="List",
            input_class=Mock,
            output_class=Mock,
        )

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

        mock_crd = Mock()
        mock_crd._list.return_value = mock_response

        list_func_impl(
            crd_method_info,
            Mock(
                arguments={
                    "self": mock_crd,
                    "namespace": "test-namespace",
                    "limit": 100,
                }
            ),
        )

        mock_crd._list.assert_called_once_with(namespace="test-namespace", limit=100)

    @patch("michelangelo.cli.mactl.crd.ParseDict")
    def test_list_func_impl_with_limit_warning(self, mock_parse_dict):
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

        mock_crd = Mock()
        mock_crd._list.return_value = mock_response

        list_func_impl(
            crd_method_info,
            Mock(
                arguments={"self": mock_crd, "namespace": "test-namespace", "limit": 10}
            ),
        )

        mock_crd._list.assert_called_once_with(namespace="test-namespace", limit=10)


class ListFuncImplRawTest(TestCase):
    """Test cases for _list_func_impl function."""

    @patch("michelangelo.cli.mactl.crd.crd_method_call")
    @patch("michelangelo.cli.mactl.crd.ParseDict")
    def test_list_func_impl_raw(self, mock_parse_dict, mock_call):
        """Test _list_func_impl builds request and returns raw response.

        It tests `_list` func without printing.
        """
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="michelangelo.api.v2.ProjectService",
            method_name="List",
            input_class=Mock,
            output_class=Mock,
        )
        mock_response = Mock()
        mock_call.return_value = mock_response

        result = _list_func_impl(
            crd_method_info,
            Mock(arguments={"namespace": "test-namespace", "limit": 100}),
        )

        call_args = mock_parse_dict.call_args
        request_dict = call_args[0][0]
        self.assertEqual(request_dict["namespace"], "test-namespace")
        self.assertEqual(request_dict["list_options_ext"]["pagination"]["limit"], 100)
        self.assertEqual(result, mock_response)

    @patch("michelangelo.cli.mactl.crd.crd_method_call")
    @patch("michelangelo.cli.mactl.crd.ParseDict")
    def test_list_func_impl_raw_defaults_to_desc_creation_sort(
        self, mock_parse_dict, mock_call
    ):
        """`_list_func_impl` requests DESC-by-creation ordering by default.

        Matches Go mactl autogen behavior (main.go:141-147) so `<crd> get`
        returns newest-first without callers having to opt in.
        """
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="michelangelo.api.v2.ProjectService",
            method_name="List",
            input_class=Mock,
            output_class=Mock,
        )
        mock_call.return_value = Mock()

        _list_func_impl(
            crd_method_info,
            Mock(arguments={"namespace": "test-namespace", "limit": 100}),
        )

        request_dict = mock_parse_dict.call_args[0][0]
        order_by = request_dict["list_options_ext"]["order_by"]
        self.assertEqual(len(order_by), 1)
        self.assertEqual(order_by[0]["field"], "metadata.creation_timestamp")
        self.assertEqual(order_by[0]["dir"], "SORT_ORDER_DESC")

    @patch("michelangelo.cli.mactl.crd.crd_method_call")
    @patch("michelangelo.cli.mactl.crd.ParseDict")
    def test_list_func_impl_raw_all_namespaces_blanks_namespace(
        self, mock_parse_dict, mock_call
    ):
        """`all_namespaces=True` sends namespace='' on the wire regardless of arg."""
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="michelangelo.api.v2.ProjectService",
            method_name="List",
            input_class=Mock,
            output_class=Mock,
        )
        mock_call.return_value = Mock()

        _list_func_impl(
            crd_method_info,
            Mock(
                arguments={
                    "namespace": "ignored-ns",
                    "limit": 100,
                    "all_namespaces": True,
                }
            ),
        )

        request_dict = mock_parse_dict.call_args[0][0]
        self.assertEqual(request_dict["namespace"], "")


class RenderHelpersTest(TestCase):
    """Test cases for _render_list_items and _render_single_item helpers."""

    def _mock_item(self, ns: str, name: str) -> Mock:
        m = Mock()
        m.metadata = Mock()
        m.metadata.namespace = ns
        m.metadata.name = name
        return m

    @patch("michelangelo.cli.mactl.crd.MessageToDict")
    def test_render_list_items_yaml(self, mock_to_dict):
        """Yaml output emits a mapping under `items:` with proto field names."""
        from michelangelo.cli.mactl.crd import _render_list_items

        mock_to_dict.side_effect = [
            {"metadata": {"name": "a"}},
            {"metadata": {"name": "b"}},
        ]

        import io
        from contextlib import redirect_stdout

        buf = io.StringIO()
        with redirect_stdout(buf):
            _render_list_items(
                [self._mock_item("ns", "a"), self._mock_item("ns", "b")], "yaml"
            )

        out = buf.getvalue()
        self.assertIn("items:", out)
        self.assertIn("name: a", out)
        self.assertIn("name: b", out)
        # yaml.safe_dump must have been called with preserving_proto_field_name=True
        # via MessageToDict — verified by side_effect being consumed twice
        self.assertEqual(mock_to_dict.call_count, 2)

    @patch("michelangelo.cli.mactl.crd.MessageToDict")
    def test_render_list_items_json(self, mock_to_dict):
        """Json output emits valid JSON with items array."""
        import json as _json

        from michelangelo.cli.mactl.crd import _render_list_items

        mock_to_dict.side_effect = [{"name": "a"}, {"name": "b"}]

        import io
        from contextlib import redirect_stdout

        buf = io.StringIO()
        with redirect_stdout(buf):
            _render_list_items(
                [self._mock_item("ns", "a"), self._mock_item("ns", "b")], "json"
            )

        parsed = _json.loads(buf.getvalue())
        self.assertEqual(parsed, {"items": [{"name": "a"}, {"name": "b"}]})

    @patch("michelangelo.cli.mactl.crd.print_list_formatted")
    def test_render_list_items_table_defaults_to_current_impl(self, mock_print):
        """Table output delegates to print_list_formatted (unchanged behavior)."""
        from michelangelo.cli.mactl.crd import _render_list_items

        items = [self._mock_item("ns", "a")]
        _render_list_items(items, "table")

        mock_print.assert_called_once_with(items)

    @patch("michelangelo.cli.mactl.crd.MessageToJson")
    def test_render_single_item_json(self, mock_to_json):
        """Json output for a single item uses MessageToJson."""
        from michelangelo.cli.mactl.crd import _render_single_item

        mock_to_json.return_value = '{"name": "x"}'

        import io
        from contextlib import redirect_stdout

        buf = io.StringIO()
        with redirect_stdout(buf):
            _render_single_item(Mock(), "json")

        self.assertIn('"name": "x"', buf.getvalue())
        mock_to_json.assert_called_once()

    @patch("michelangelo.cli.mactl.crd.MessageToDict")
    def test_render_single_item_yaml(self, mock_to_dict):
        """Yaml output for a single item uses MessageToDict + yaml_safe_dump."""
        from michelangelo.cli.mactl.crd import _render_single_item

        mock_to_dict.return_value = {"metadata": {"name": "x"}}

        import io
        from contextlib import redirect_stdout

        buf = io.StringIO()
        with redirect_stdout(buf):
            _render_single_item(Mock(), "yaml")

        self.assertIn("name: x", buf.getvalue())


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

    def test_get_func_impl_with_name_calls_get(self):
        """Test get_func_impl with name calls _self._get and prints result."""
        mock_crd = Mock()
        mock_response = Mock()
        mock_crd._get.return_value = mock_response

        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Get",
            input_class=Mock,
            output_class=Mock,
        )
        result = get_func_impl(
            crd_method_info,
            Mock(arguments={"self": mock_crd, "namespace": "ns", "name": "proj"}),
        )

        mock_crd._get.assert_called_once_with(namespace="ns", name="proj")
        self.assertEqual(result, mock_response)

    def test_get_func_impl_with_name_flag_calls_get(self):
        """`--name X` (dest=name_flag) routes through _get just like positional."""
        mock_crd = Mock()
        mock_response = Mock()
        mock_crd._get.return_value = mock_response

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
                    "name": "",
                    "name_flag": "proj",
                }
            ),
        )

        mock_crd._get.assert_called_once_with(namespace="ns", name="proj")
        self.assertEqual(result, mock_response)

    def test_get_func_impl_positional_overrides_name_flag(self):
        """Positional `name` wins when both are supplied."""
        mock_crd = Mock()
        mock_crd._get.return_value = Mock()

        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Get",
            input_class=Mock,
            output_class=Mock,
        )
        get_func_impl(
            crd_method_info,
            Mock(
                arguments={
                    "self": mock_crd,
                    "namespace": "ns",
                    "name": "from-positional",
                    "name_flag": "from-flag",
                }
            ),
        )

        mock_crd._get.assert_called_once_with(namespace="ns", name="from-positional")

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
                    "name": "",
                    "name_flag": "",
                    "limit": 50,
                }
            ),
        )

        mock_crd.generate_list.assert_called_once_with(crd_method_info.channel)
        mock_crd.list.assert_called_once_with(
            namespace="ns",
            limit=50,
            all_namespaces=False,
            output="table",
        )
        self.assertEqual(result, "list_result")

    def test_get_func_impl_all_namespaces_lists_with_empty_namespace(self):
        """`-A` with no name lists across all namespaces (namespace='' on wire)."""
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

        get_func_impl(
            crd_method_info,
            Mock(
                arguments={
                    "self": mock_crd,
                    "namespace": "",
                    "name": "",
                    "name_flag": "",
                    "limit": 100,
                    "all_namespaces": True,
                    "output": "table",
                }
            ),
        )

        mock_crd.list.assert_called_once_with(
            namespace="",
            limit=100,
            all_namespaces=True,
            output="table",
        )

    def test_get_func_impl_all_namespaces_wins_over_provided_namespace(self):
        """When `-A` is set, --namespace value is ignored (mirrors Go mactl)."""
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

        get_func_impl(
            crd_method_info,
            Mock(
                arguments={
                    "self": mock_crd,
                    "namespace": "ignored-ns",
                    "name": "",
                    "name_flag": "",
                    "limit": 100,
                    "all_namespaces": True,
                    "output": "table",
                }
            ),
        )

        mock_crd.list.assert_called_once_with(
            namespace="",
            limit=100,
            all_namespaces=True,
            output="table",
        )

    def test_get_func_impl_all_namespaces_with_name_errors(self):
        """`-A` combined with a resource name raises ValueError."""
        mock_crd = Mock()
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Get",
            input_class=Mock,
            output_class=Mock,
        )

        with self.assertRaisesRegex(ValueError, "all-namespaces"):
            get_func_impl(
                crd_method_info,
                Mock(
                    arguments={
                        "self": mock_crd,
                        "namespace": "",
                        "name": "my-resource",
                        "name_flag": "",
                        "all_namespaces": True,
                    }
                ),
            )

    def test_get_func_impl_no_namespace_no_all_namespaces_errors(self):
        """Missing both --namespace and --all-namespaces raises ValueError."""
        mock_crd = Mock()
        mock_crd.generate_list = Mock()
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Get",
            input_class=Mock,
            output_class=Mock,
        )

        with self.assertRaisesRegex(ValueError, "namespace"):
            get_func_impl(
                crd_method_info,
                Mock(
                    arguments={
                        "self": mock_crd,
                        "namespace": "",
                        "name": "",
                        "name_flag": "",
                        "all_namespaces": False,
                    }
                ),
            )

    def test_get_func_impl_name_without_namespace_errors(self):
        """Fetching by name without --namespace raises ValueError."""
        mock_crd = Mock()
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Get",
            input_class=Mock,
            output_class=Mock,
        )

        with self.assertRaisesRegex(ValueError, "namespace"):
            get_func_impl(
                crd_method_info,
                Mock(
                    arguments={
                        "self": mock_crd,
                        "namespace": "",
                        "name": "my-resource",
                        "name_flag": "",
                        "all_namespaces": False,
                    }
                ),
            )


class GetFuncImplRawTest(TestCase):
    """Test cases for _get_func_impl function."""

    @patch("michelangelo.cli.mactl.crd.crd_method_call_kwargs")
    def test_get_func_impl_raw(self, mock_call_kwargs):
        """Test _get_func_impl calls crd_method_call_kwargs and returns result."""
        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Get",
            input_class=Mock,
            output_class=Mock,
        )
        mock_response = Mock()
        mock_call_kwargs.return_value = mock_response

        result = _get_func_impl(
            crd_method_info,
            Mock(arguments={"namespace": "ns", "name": "proj"}),
        )

        mock_call_kwargs.assert_called_once_with(
            crd_method_info, namespace="ns", name="proj"
        )
        self.assertEqual(result, mock_response)


class ApplyFuncImplTest(TestCase):
    """Test cases for apply_func_impl function."""

    @patch("michelangelo.cli.mactl.crd.crd_method_call")
    @patch("michelangelo.cli.mactl.crd.get_crd_namespace_and_name_from_yaml")
    def test_apply_func_impl_update(self, mock_get_ns: MagicMock, _):
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
        mock_crd._get.return_value = Mock()
        mock_crd.read_yaml_and_update_crd_request.return_value = Mock()
        mock_get_ns.return_value = ("ns", "name")

        apply_func_impl(
            crd_method_info, Mock(arguments={"self": mock_crd, "file": "f.yaml"})
        )

        mock_crd._get.assert_called_once_with("ns", "name")
        mock_crd.read_yaml_and_update_crd_request.assert_called_once()

    @patch("michelangelo.cli.mactl.crd.crd_method_call")
    @patch("michelangelo.cli.mactl.crd.get_crd_namespace_and_name_from_yaml")
    def test_apply_func_impl_invokes_pre_apply_hook(self, mock_get_ns: MagicMock, _):
        """apply_func_impl runs registered pre-apply checks with the CRD full name."""
        from michelangelo.cli.mactl import apply_hooks

        received: list[str] = []
        apply_hooks._pre_apply_checks.clear()
        apply_hooks.register_pre_apply_check(received.append)
        self.addCleanup(apply_hooks._pre_apply_checks.clear)

        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Apply",
            input_class=Mock,
            output_class=Mock,
        )
        mock_crd = Mock()
        mock_crd.full_name = "test.Service"
        mock_crd._get.return_value = Mock()
        mock_crd.read_yaml_and_update_crd_request.return_value = Mock()
        mock_get_ns.return_value = ("ns", "name")

        apply_func_impl(
            crd_method_info, Mock(arguments={"self": mock_crd, "file": "f.yaml"})
        )

        self.assertEqual(received, ["test.Service"])

    def test_apply_func_impl_pre_apply_hook_can_abort(self):
        """A raising pre-apply check halts apply_func_impl before any gRPC call."""
        from michelangelo.cli.mactl import apply_hooks

        def reject(_: str) -> None:
            raise RuntimeError("blocked by hook")

        apply_hooks._pre_apply_checks.clear()
        apply_hooks.register_pre_apply_check(reject)
        self.addCleanup(apply_hooks._pre_apply_checks.clear)

        crd_method_info = CrdMethodInfo(
            channel=Mock(),
            crd_full_name="test.Service",
            method_name="Apply",
            input_class=Mock,
            output_class=Mock,
        )
        mock_crd = Mock()

        with self.assertRaisesRegex(RuntimeError, "blocked by hook"):
            apply_func_impl(
                crd_method_info,
                Mock(arguments={"self": mock_crd, "file": "f.yaml"}),
            )

        mock_crd._get.assert_not_called()


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
        """Test generate_get creates both get and _get methods on CRD instance."""
        mock_channel = Mock()
        mock_extract_method_info.return_value = ("GetTestCrd", Mock, Mock)

        crd = CRD(name="test_crd", full_name="test.service.TestCrd", metadata=[])
        crd.generate_get(mock_channel)

        self.assertTrue(hasattr(crd, "get"))
        self.assertTrue(callable(crd.get))
        self.assertTrue(hasattr(crd, "_get"))
        self.assertTrue(callable(crd._get))

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

    @patch("michelangelo.cli.mactl.crd.crd_method_call_kwargs")
    @patch.object(CRD, "_extract_method_info")
    def test_generate_get_execution_via_name_flag(
        self, mock_extract_method_info, mock_crd_method_call_kwargs
    ):
        """Generated `get` resolves the --name flag (dest=name_flag) like positional.

        Exercises the full bind_signature path with `name=""` and `name_flag="X"`,
        the binding state that argparse produces when the user supplies --name only.
        """
        mock_channel = Mock()
        mock_extract_method_info.return_value = ("GetTestCrd", Mock, Mock)
        mock_response = Mock()
        mock_crd_method_call_kwargs.return_value = mock_response

        crd = CRD(name="test_crd", full_name="test.service.TestCrd", metadata=[])
        crd.generate_get(mock_channel)

        result = crd.get(namespace="test-ns", name_flag="test-name")

        self.assertEqual(result, mock_response)
        call_args = mock_crd_method_call_kwargs.call_args
        self.assertEqual(call_args.kwargs["namespace"], "test-ns")
        self.assertEqual(call_args.kwargs["name"], "test-name")


class GenerateListTest(TestCase):
    """Test cases for CRD.generate_list method."""

    @patch.object(CRD, "_extract_method_info")
    def test_generate_list(self, mock_extract_method_info):
        """Test generate_list creates both list and _list methods on CRD instance."""
        mock_channel = Mock()
        mock_extract_method_info.return_value = ("ListTestCrd", Mock, Mock)

        crd = CRD(name="test_crd", full_name="test.service.TestCrd", metadata=[])
        crd.generate_list(mock_channel)

        self.assertTrue(hasattr(crd, "list"))
        self.assertTrue(callable(crd.list))
        self.assertTrue(hasattr(crd, "_list"))
        self.assertTrue(callable(crd._list))

    @patch("michelangelo.cli.mactl.crd.crd_method_call")
    @patch("michelangelo.cli.mactl.crd.ParseDict")
    @patch.object(CRD, "_extract_method_info")
    def test_generate_list_raw_execution(
        self, mock_extract_method_info, mock_parse_dict, mock_crd_method_call
    ):
        """Test the generated _list method returns raw response without printing."""
        mock_channel = Mock()
        mock_extract_method_info.return_value = ("ListTestCrd", Mock, Mock)
        mock_response = Mock()
        mock_crd_method_call.return_value = mock_response

        crd = CRD(name="test_crd", full_name="test.service.TestCrd", metadata=[])
        crd.generate_list(mock_channel)

        result = crd._list(namespace="test-ns")

        self.assertEqual(result, mock_response)
        request_dict = mock_parse_dict.call_args[0][0]
        self.assertEqual(request_dict["namespace"], "test-ns")
