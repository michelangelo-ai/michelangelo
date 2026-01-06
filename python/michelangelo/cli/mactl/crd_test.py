"""Unit tests for CRD module."""

from datetime import datetime, timezone
from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from michelangelo.cli.mactl.crd import (
    CrdMethodInfo,
    apply_func_impl,
    create_func_impl,
    delete_func_impl,
    get_func_impl,
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


class ListFuncImplTest(TestCase):
    """Test cases for list_func_impl function."""

    @patch("michelangelo.cli.mactl.crd.crd_method_call_kwargs")
    def test_list_func_impl(self, mock_call_kwargs):
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
        mock_call_kwargs.return_value = mock_response

        # Execute - should not raise any exceptions
        list_func_impl(crd_method_info, Mock(arguments={"namespace": "test-namespace"}))

        # Verify crd_method_call_kwargs was called with correct arguments
        mock_call_kwargs.assert_called_once_with(
            crd_method_info, namespace="test-namespace"
        )


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
