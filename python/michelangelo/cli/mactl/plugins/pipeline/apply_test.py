"""Unit tests for pipeline apply plugin.

Tests the convert_crd_metadata_pipeline_apply function.
"""

from pathlib import Path
from unittest import TestCase
from unittest.mock import Mock, patch

from michelangelo.cli.mactl.plugins.pipeline.apply import (
    convert_crd_metadata_pipeline_apply,
)


class PipelineApplyTest(TestCase):
    """Tests for pipeline apply plugin."""

    def test_convert_crd_metadata_pipeline_apply_basic(self):
        """Test basic conversion of CRD metadata for pipeline apply."""
        # Mock input
        yaml_dict = {
            "apiVersion": "michelangelo.api/v2",
            "kind": "Pipeline",
            "metadata": {"name": "test-pipeline", "namespace": "test-ns"},
            "spec": {
                "description": "Test pipeline",
                "environment": "production",
            },
        }
        mock_crd_class = Mock()
        yaml_path = Path("/fake/path/pipeline.yaml")

        # Mock git repo
        mock_repo = Mock()
        mock_repo.active_branch.name = "main"
        mock_repo.head.commit.hexsha = "abc123"

        with patch(
            "michelangelo.cli.mactl.plugins.pipeline.apply.Repo"
        ) as mock_repo_class:
            mock_repo_class.return_value = mock_repo

            result = convert_crd_metadata_pipeline_apply(
                yaml_dict, mock_crd_class, yaml_path
            )

        # Verify result structure
        self.assertIn("spec", result)
        self.assertEqual(result["spec"]["description"], "Test pipeline")
        self.assertEqual(result["spec"]["environment"], "production")

        # Verify git repo was accessed
        mock_repo_class.assert_called_once_with(".", search_parent_directories=True)

    def test_convert_crd_metadata_pipeline_apply_invalid_input(self):
        """Test that invalid input raises ValueError."""
        mock_crd_class = Mock()
        yaml_path = Path("/fake/path/pipeline.yaml")

        # Test with non-dict input
        with self.assertRaises(ValueError) as context:
            convert_crd_metadata_pipeline_apply("not a dict", mock_crd_class, yaml_path)

        self.assertIn("Expected a dictionary", str(context.exception))

    def test_convert_crd_metadata_pipeline_apply_copies_spec(self):
        """Test that spec is deep copied (not referenced)."""
        yaml_dict = {
            "spec": {"nested": {"value": "original"}},
        }
        mock_crd_class = Mock()
        yaml_path = Path("/fake/path/pipeline.yaml")

        mock_repo = Mock()
        mock_repo.active_branch.name = "main"
        mock_repo.head.commit.hexsha = "abc123"

        with patch(
            "michelangelo.cli.mactl.plugins.pipeline.apply.Repo"
        ) as mock_repo_class:
            mock_repo_class.return_value = mock_repo

            result = convert_crd_metadata_pipeline_apply(
                yaml_dict, mock_crd_class, yaml_path
            )

        # Modify original
        yaml_dict["spec"]["nested"]["value"] = "modified"

        # Result should not be affected
        self.assertEqual(result["spec"]["nested"]["value"], "original")
