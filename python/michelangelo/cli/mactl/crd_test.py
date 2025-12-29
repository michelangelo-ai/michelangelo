"""Unit tests for CRD module."""

from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from michelangelo.cli.mactl.crd import (
    CrdMethodInfo,
    delete_func_impl,
    list_func_impl,
    prepare_column_info,
)


class PrepareColumnInfoTest(TestCase):
    """Test cases for prepare_column_info function."""

    def test_prepare_column_info(self):
        """Test prepare_column_info returns correct structure.

        column structure and retrieve functions work.
        """
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
                "2021-12-20_03:33:20",
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
