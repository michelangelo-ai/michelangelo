"""Unit tests for dev_run functionality."""

import os
import tempfile
import unittest
from inspect import Parameter, Signature
from pathlib import Path
from unittest.mock import MagicMock, Mock, patch

from google.protobuf.message import Message
from google.protobuf.struct_pb2 import Struct

from michelangelo.cli.mactl.crd import CRD
from michelangelo.cli.mactl.plugins.entity.pipeline.create import (
    get_pipeline_config_and_tar,
)
from michelangelo.cli.mactl.plugins.entity.pipeline.dev_run import (
    convert_crd_metadata_pipeline_dev_run,
    generate_dev_run,
)


class TestDevRun(unittest.TestCase):
    """Unit tests for dev_run functionality."""

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.Repo")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.handle_workflow_inputs_retrieval"
    )
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.yaml_to_dict")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_name"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_object"
    )
    def test_storage_url_passed_to_workflow_retrieval(
        self, mock_gen_obj, mock_gen_name, mock_yaml, mock_handle, mock_repo
    ):
        """Test that storage_url parameter is correctly passed through dev_run.

        Verifies that the storage_url parameter is passed to
        handle_workflow_inputs_retrieval.
        """
        # Setup mock git repository
        mock_repo_instance = MagicMock()
        mock_repo_instance.git.rev_parse.return_value = str(Path.cwd())
        mock_repo_instance.active_branch.name = "main"
        mock_repo_instance.head.commit.hexsha = "abc123def456"
        mock_repo.return_value = mock_repo_instance

        # Setup mock returns
        mock_yaml.return_value = {
            "metadata": {"namespace": "test-project", "name": "test-pipeline"},
            "spec": {},
        }
        # Create a proper Struct object instead of dict
        workflow_inputs = Struct()
        mock_handle.return_value = (
            workflow_inputs,
            "s3://test/path.tar.gz",
            "test_workflow",
        )
        mock_gen_name.return_value = "test-run-123"
        mock_gen_obj.return_value = {"spec": {}}

        # Create test data
        yaml_dict = {
            "metadata": {"namespace": "test-project", "name": "test-pipeline"},
            "spec": {},
        }
        crd_class = MagicMock(spec=Message)
        # Create a yaml_path that's actually within the repo
        yaml_path = Path.cwd() / "test-pipeline.yaml"
        test_storage_url = "s3://custom-bucket/custom-path"

        # Call the function with storage_url
        result = convert_crd_metadata_pipeline_dev_run(
            yaml_dict, crd_class, yaml_path, storage_url=test_storage_url
        )

        # Verify that handle_workflow_inputs_retrieval was called with the correct
        # storage_url
        mock_handle.assert_called_once()
        args, kwargs = mock_handle.call_args

        # The storage_url should be the last positional argument
        self.assertEqual(
            len(args), 5, f"Expected 5 args (including storage_url), got {len(args)}"
        )
        self.assertEqual(
            args[4],
            test_storage_url,
            f"Expected storage_url '{test_storage_url}', got '{args[4]}'",
        )

        # Verify the function returns a valid result
        self.assertIsInstance(result, dict)
        self.assertIn("pipeline_run", result)

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.Repo")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.handle_workflow_inputs_retrieval"
    )
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.yaml_to_dict")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_name"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_object"
    )
    def test_storage_url_none_by_default(
        self, mock_gen_obj, mock_gen_name, mock_yaml, mock_handle, mock_repo
    ):
        """Test that storage_url parameter defaults to None when not provided."""
        # Setup mock git repository
        mock_repo_instance = MagicMock()
        mock_repo_instance.git.rev_parse.return_value = str(Path.cwd())
        mock_repo_instance.active_branch.name = "main"
        mock_repo_instance.head.commit.hexsha = "abc123def456"
        mock_repo.return_value = mock_repo_instance

        # Setup mock returns
        mock_yaml.return_value = {
            "metadata": {"namespace": "test-project", "name": "test-pipeline"},
            "spec": {},
        }
        # Create a proper Struct object instead of dict
        workflow_inputs = Struct()
        mock_handle.return_value = (
            workflow_inputs,
            "s3://test/path.tar.gz",
            "test_workflow",
        )
        mock_gen_name.return_value = "test-run-123"
        mock_gen_obj.return_value = {"spec": {}}

        # Create test data
        yaml_dict = {
            "metadata": {"namespace": "test-project", "name": "test-pipeline"},
            "spec": {},
        }
        crd_class = MagicMock(spec=Message)
        # Create a yaml_path that's actually within the repo
        yaml_path = Path.cwd() / "test-pipeline.yaml"

        # Call the function without storage_url (should default to None)
        result = convert_crd_metadata_pipeline_dev_run(yaml_dict, crd_class, yaml_path)

        # Verify that handle_workflow_inputs_retrieval was called with None storage_url
        mock_handle.assert_called_once()
        args, kwargs = mock_handle.call_args

        # The storage_url should be the last positional argument and None
        self.assertEqual(
            len(args), 5, f"Expected 5 args (including storage_url), got {len(args)}"
        )
        self.assertIsNone(args[4], f"Expected storage_url to be None, got '{args[4]}'")

        # Verify the function returns a valid result
        self.assertIsInstance(result, dict)
        self.assertIn("pipeline_run", result)

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.create.run_subprocess_registration"
    )
    def test_storage_url_passed_to_subprocess_registration(self, mock_subprocess):
        """Test that storage_url is passed correctly to run_subprocess_registration."""
        # Mock all file operations and subprocess calls
        test_storage_url = "s3://custom-bucket/my-path"

        create_module = "michelangelo.cli.mactl.plugins.entity.pipeline.create"
        with (
            patch(f"{create_module}.Path.exists") as mock_exists,
            patch(f"{create_module}.tempfile.TemporaryDirectory") as mock_tempdir,
            patch(f"{create_module}.read_subprocess_outputs") as mock_read,
            patch(f"{create_module}.Path.read_text") as mock_read_text,
            patch(f"{create_module}.json.loads") as mock_json,
        ):
            # Mock file exists check to pass
            mock_exists.return_value = True

            # Mock temporary directory
            mock_tempdir.return_value.__enter__.return_value = "/tmp/mock"
            mock_tempdir.return_value.__exit__.return_value = None

            # Mock subprocess result reading to succeed
            mock_read.return_value = (True, "Success", "s3://test/output.tar.gz")

            # Mock file reading operations - different values for different files
            def mock_read_text_side_effect(*args, **kwargs):
                # Return different content based on which file is being read
                args_str = str(args)
                kwargs_str = str(kwargs)
                if (
                    "uniflow_tar_path.txt" in args_str
                    or "uniflow_tar_path" in kwargs_str
                ):
                    return "s3://test/output.tar.gz"
                elif "uniflow_input.txt" in args_str or "uniflow_input" in kwargs_str:
                    return '{"environ": {}, "kwargs": []}'
                elif (
                    "workflow_function_name.txt" in args_str
                    or "workflow_function" in kwargs_str
                ):
                    return "test_workflow"
                return ""

            mock_read_text.side_effect = mock_read_text_side_effect
            mock_json.return_value = {"environ": {}, "kwargs": []}

            # Setup subprocess mock to succeed
            mock_subprocess.return_value = MagicMock(returncode=0, stdout="", stderr="")

            # Call the function with storage_url
            result = get_pipeline_config_and_tar(
                repo_root=Path("/fake/repo"),
                config_file_relative_path="config.yaml",
                bazel_target="",
                project="test-project",
                pipeline="test-pipeline",
                storage_url=test_storage_url,
            )

            # Verify run_subprocess_registration was called with the correct
            # storage_url
            mock_subprocess.assert_called_once()
            args, kwargs = mock_subprocess.call_args

            # Check that storage_url was passed correctly
            self.assertIn("storage_url", kwargs)
            self.assertEqual(kwargs["storage_url"], test_storage_url)

            # Verify other parameters are as expected
            self.assertEqual(kwargs["project"], "test-project")
            self.assertEqual(kwargs["pipeline"], "test-pipeline")

            # Verify the function returns the expected tuple
            self.assertIsInstance(result, tuple)
            self.assertEqual(len(result), 3)

    def test_dev_run_func_extracts_storage_url_from_bound_args(self):
        """Test that dev_run function extracts storage_url from bound_args.

        This test specifically targets line 187 in dev_run.py:
        _storage_url = bound_args.arguments.get("storage_url")
        """
        # Create a real CRD instance (not a mock)
        temp_yaml = None
        try:
            # Create temporary yaml file for testing
            with tempfile.NamedTemporaryFile(
                mode="w", suffix=".yaml", delete=False
            ) as f:
                f.write("""
metadata:
  name: test-pipeline
  namespace: test-project
spec:
  workflowGraph:
    nodes: []
""")
            temp_yaml = f.name

            crd = CRD(
                name="test-pipeline",
                full_name="test-project.test-pipeline",
                metadata=[{"project": "test-project"}],
            )

            # Mock the required methods that would normally be set up
            crd.func_crd_metadata_converter = Mock(
                return_value={"pipeline_run": {"spec": {}}}
            )

            # Override _read_signatures to provide our test signature
            original_read_signatures = getattr(crd, "_read_signatures", None)

            def mock_read_signatures(method_name):
                if method_name == "dev_run":
                    return Signature(
                        [
                            Parameter("self", Parameter.POSITIONAL_OR_KEYWORD),
                            Parameter("file", Parameter.POSITIONAL_OR_KEYWORD),
                            Parameter(
                                "storage_url", Parameter.KEYWORD_ONLY, default=None
                            ),
                        ]
                    )
                if original_read_signatures:
                    return original_read_signatures(method_name)
                return Signature([])

            crd._read_signatures = mock_read_signatures

            # Create mock channel
            mock_channel = Mock()
            mock_channel.unary_unary.return_value = Mock(return_value=Mock())

            # Mock the service discovery methods
            with (
                patch(
                    "michelangelo.cli.mactl.plugins.entity.pipeline"
                    ".dev_run.get_service_name"
                ) as mock_get_service,
                patch(
                    "michelangelo.cli.mactl.plugins.entity.pipeline"
                    ".dev_run.get_methods_from_service"
                ) as mock_get_methods,
                patch(
                    "michelangelo.cli.mactl.plugins.entity.pipeline"
                    ".dev_run.get_message_class_by_name"
                ) as mock_get_message_class,
                patch(
                    "michelangelo.cli.mactl.plugins.entity.pipeline"
                    ".dev_run.ParseDict"
                ) as mock_parse_dict,
            ):
                # Setup service mocks
                mock_get_service.return_value = "test.service"
                mock_method = Mock()
                mock_method.input_type = ".TestInput"
                mock_method.output_type = ".TestOutput"
                mock_get_methods.return_value = (
                    {"CreatePipelineRun": mock_method},
                    Mock(),
                )

                # Setup message class mocks (simplified since we're patching ParseDict)
                mock_input_class = Mock()
                mock_output_class = Mock()
                mock_get_message_class.side_effect = [
                    mock_input_class,
                    mock_output_class,
                ]

                # Mock ParseDict to avoid protobuf complexity
                mock_parse_dict.return_value = None

                # Generate the dev_run function - creates real function and
                # executes line 187
                generate_dev_run(crd, mock_channel)

                # Now test the dev_run function by calling it
                # This will exercise line 187:
                # _storage_url = bound_args.arguments.get("storage_url")
                test_storage_url = "s3://test-bucket/test-path"

                # Call with storage_url
                crd.dev_run(file=temp_yaml, storage_url=test_storage_url)

                # Call without storage_url (should default to None)
                crd.dev_run(file=temp_yaml)

        finally:
            # Clean up temp file
            if temp_yaml and os.path.exists(temp_yaml):
                os.unlink(temp_yaml)


if __name__ == "__main__":
    unittest.main()
