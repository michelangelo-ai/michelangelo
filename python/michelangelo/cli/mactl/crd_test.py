"""Unit tests for CRD module."""

from unittest import TestCase
from unittest.mock import Mock

from michelangelo.cli.mactl.crd import prepare_column_info


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
