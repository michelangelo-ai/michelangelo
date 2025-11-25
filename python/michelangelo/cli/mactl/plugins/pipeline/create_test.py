"""Unit tests for pipeline create plugin.

Tests the convert_crd_metadata_pipeline_create and related functions.
"""

from pathlib import Path
from unittest import TestCase
from unittest.mock import Mock, patch

from google.protobuf.struct_pb2 import Struct

from michelangelo.cli.mactl.plugins.pipeline.create import (
    convert_crd_metadata_pipeline_create,
    handle_workflow_inputs_retrieval,
    populate_pipeline_spec_with_workflow_inputs,
)


class PipelineCreateTest(TestCase):
    """Tests for pipeline create plugin."""

    def test_convert_crd_metadata_pipeline_create_invalid_input(self):
        """Test that invalid input raises ValueError."""
        mock_crd_class = Mock()
        yaml_path = Path("/fake/path/pipeline.yaml")

        # Test with non-dict input
        with self.assertRaises(ValueError) as context:
            convert_crd_metadata_pipeline_create(
                "not a dict", mock_crd_class, yaml_path
            )

        self.assertIn("Expected a dictionary", str(context.exception))

    @patch("michelangelo.cli.mactl.plugins.pipeline.create.Repo")
    @patch(
        "michelangelo.cli.mactl.plugins.pipeline.create.handle_workflow_inputs_retrieval"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.pipeline.create.populate_pipeline_spec_with_workflow_inputs"
    )
    def test_convert_crd_metadata_pipeline_create_basic(
        self, mock_populate, mock_handle_workflow, mock_repo_class
    ):
        """Test basic conversion of CRD metadata for pipeline create."""
        # Mock input
        yaml_dict = {
            "apiVersion": "michelangelo.api/v2",
            "kind": "Pipeline",
            "metadata": {
                "name": "test-pipeline",
                "namespace": "test-project",
                "annotations": {"key": "value"},
                "labels": {"env": "prod"},
            },
            "spec": {
                "description": "Test pipeline",
                "environment": "production",
            },
        }
        mock_crd_class = Mock()
        yaml_path = Path("/fake/repo/pipelines/pipeline.yaml")

        # Mock git repo
        mock_repo = Mock()
        mock_repo.active_branch.name = "main"
        mock_repo.head.commit.hexsha = "abc123"
        mock_repo.git.rev_parse.return_value = "/fake/repo"
        mock_repo_class.return_value = mock_repo

        # Mock workflow inputs
        mock_workflow_inputs = Struct()
        mock_handle_workflow.return_value = (
            mock_workflow_inputs,
            "s3://path/to/tar",
            "workflow_fn",
        )

        # Mock populate function
        expected_result = {"metadata": {}, "spec": {}}
        mock_populate.return_value = expected_result

        result = convert_crd_metadata_pipeline_create(
            yaml_dict, mock_crd_class, yaml_path
        )

        # Verify git repo was accessed
        mock_repo_class.assert_called_once_with(".", search_parent_directories=True)

        # Verify workflow inputs were retrieved
        mock_handle_workflow.assert_called_once()
        call_args = mock_handle_workflow.call_args[0]
        self.assertEqual(
            call_args[1], "pipelines/pipeline.yaml"
        )  # config_file_relative_path
        self.assertEqual(call_args[2], "test-project")  # project
        self.assertEqual(call_args[3], "test-pipeline")  # pipeline

        # Verify populate was called with correct arguments
        mock_populate.assert_called_once()
        populate_call_args = mock_populate.call_args[0]
        # First arg should be res dict with metadata
        res_dict = populate_call_args[0]
        self.assertIn("metadata", res_dict)
        self.assertEqual(res_dict["metadata"]["name"], "test-pipeline")
        self.assertEqual(res_dict["metadata"]["namespace"], "test-project")
        # Second arg should be original yaml_dict
        self.assertIs(populate_call_args[1], yaml_dict)

        # Verify result is what populate returns
        self.assertIs(result, expected_result)

    def test_populate_pipeline_spec_with_workflow_inputs_basic(self):
        """Test populate_pipeline_spec_with_workflow_inputs basic functionality."""
        res = {}
        yaml_dict = {
            "spec": {
                "description": "Test pipeline",
                "environment": "production",
            },
        }

        # Mock repo
        mock_repo = Mock()
        mock_repo.active_branch.name = "main"
        mock_repo.head.commit.hexsha = "abc123"

        yaml_path = Path("/fake/repo/pipelines/pipeline.yaml")
        repo_root = Path("/fake/repo")
        config_file_relative_path = "pipelines/pipeline.yaml"
        uniflow_tar_path = "s3://bucket/path/to/tar"
        workflow_function_name = "my_workflow"

        with patch(
            "michelangelo.cli.mactl.plugins.pipeline.create.getenv"
        ) as mock_getenv:
            mock_getenv.return_value = "test-user"

            result = populate_pipeline_spec_with_workflow_inputs(
                res,
                yaml_dict,
                None,  # No workflow inputs
                mock_repo,
                yaml_path,
                repo_root,
                config_file_relative_path,
                uniflow_tar_path,
                workflow_function_name,
            )

        # Verify spec was populated
        self.assertIn("spec", result)
        self.assertEqual(result["spec"]["description"], "Test pipeline")
        self.assertEqual(result["spec"]["commit"]["branch"], "main")
        self.assertEqual(result["spec"]["commit"]["git_ref"], "abc123")
        self.assertEqual(
            result["spec"]["manifest"]["filePath"], config_file_relative_path
        )
        self.assertEqual(
            result["spec"]["manifest"]["type"], "PIPELINE_MANIFEST_TYPE_UNIFLOW"
        )
        self.assertEqual(result["spec"]["manifest"]["uniflowTar"], uniflow_tar_path)
        self.assertEqual(
            result["spec"]["manifest"]["uniflowFunction"], workflow_function_name
        )
        self.assertEqual(result["spec"]["owner"]["name"], "test-user")

    @patch("michelangelo.cli.mactl.plugins.pipeline.create.get_pipeline_config_and_tar")
    def test_handle_workflow_inputs_retrieval_success(self, mock_get_config):
        """Test successful workflow inputs retrieval."""
        repo_root = Path("/fake/repo")
        config_file_relative_path = "pipelines/pipeline.yaml"
        project = "test-project"
        pipeline = "test-pipeline"

        # Mock successful retrieval
        mock_workflow_inputs = Struct()
        mock_workflow_inputs.update({"key": "value"})
        mock_get_config.return_value = (
            mock_workflow_inputs,
            "s3://path/to/tar",
            "workflow_fn",
        )

        result = handle_workflow_inputs_retrieval(
            repo_root, config_file_relative_path, project, pipeline
        )

        # Verify result is the tuple returned by get_pipeline_config_and_tar
        self.assertIs(result[0], mock_workflow_inputs)
        self.assertEqual(result[1], "s3://path/to/tar")
        self.assertEqual(result[2], "workflow_fn")

        # Verify get_pipeline_config_and_tar was called correctly
        mock_get_config.assert_called_once_with(
            repo_root=repo_root,
            config_file_relative_path=config_file_relative_path,
            bazel_target="",
            project=project,
            pipeline=pipeline,
        )

    @patch("michelangelo.cli.mactl.plugins.pipeline.create.get_pipeline_config_and_tar")
    def test_handle_workflow_inputs_retrieval_file_not_found(self, mock_get_config):
        """Test workflow inputs retrieval when config file not found."""
        repo_root = Path("/fake/repo")
        config_file_relative_path = "pipelines/pipeline.yaml"
        project = "test-project"
        pipeline = "test-pipeline"

        # Mock FileNotFoundError
        mock_get_config.side_effect = FileNotFoundError("Config file not found")

        with self.assertRaises(ValueError) as context:
            handle_workflow_inputs_retrieval(
                repo_root, config_file_relative_path, project, pipeline
            )

        self.assertIn("Pipeline configuration file is missing", str(context.exception))

    @patch("michelangelo.cli.mactl.plugins.pipeline.create.get_pipeline_config_and_tar")
    def test_handle_workflow_inputs_retrieval_runtime_error_graceful_degradation(
        self, mock_get_config
    ):
        """Test graceful degradation when registration fails with generic RuntimeError."""
        repo_root = Path("/fake/repo")
        config_file_relative_path = "pipelines/pipeline.yaml"
        project = "test-project"
        pipeline = "test-pipeline"

        # Mock RuntimeError that doesn't match specific patterns
        mock_get_config.side_effect = RuntimeError("Some registration error")

        # Should return empty values instead of raising
        result = handle_workflow_inputs_retrieval(
            repo_root, config_file_relative_path, project, pipeline
        )

        self.assertIsNone(result[0])
        self.assertEqual(result[1], "")
        self.assertEqual(result[2], "")
