"""Unit tests for pipeline create plugin.

Tests the convert_crd_metadata_pipeline_create and related functions.
"""

import json
from pathlib import Path
from unittest import TestCase
from unittest.mock import Mock, patch

from google.protobuf.struct_pb2 import Struct

from michelangelo.cli.mactl.plugins.entity.pipeline.create import (
    convert_crd_metadata_pipeline_create,
    get_pipeline_config_and_tar,
    handle_workflow_inputs_retrieval,
    populate_pipeline_spec_with_trigger_configs,
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

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.create.Repo")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.create.handle_workflow_inputs_retrieval"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.create.populate_pipeline_spec_with_workflow_inputs"
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
            "michelangelo.cli.mactl.plugins.entity.pipeline.create.get_user_name"
        ) as mock_get_user_name:
            mock_get_user_name.return_value = "test-user"

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

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.create.get_pipeline_config_and_tar"
    )
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
            storage_url=None,
        )

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.create.get_pipeline_config_and_tar"
    )
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

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.create.get_pipeline_config_and_tar"
    )
    def test_handle_workflow_inputs_retrieval_runtime_error_graceful_degradation(
        self, mock_get_config
    ):
        """Graceful degradation when registration fails with generic RuntimeError."""
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

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.create.run_subprocess_registration"
    )
    def test_get_pipeline_config_and_tar_subprocess_exception(
        self, mock_run_subprocess
    ):
        """Test get_pipeline_config_and_tar when subprocess raises Exception (line 111).

        TODO(#940): This test is for coverage only and should be revised later.
        """
        repo_root = Path("/fake/repo")
        config_file_relative_path = "pipelines/pipeline.yaml"
        project = "test-project"
        pipeline = "test-pipeline"

        # Create a fake config file
        with patch("pathlib.Path.exists") as mock_exists:
            mock_exists.return_value = True

            # Mock subprocess to raise a generic Exception
            mock_run_subprocess.side_effect = Exception("Subprocess execution failed")

            with self.assertRaises(RuntimeError) as context:
                get_pipeline_config_and_tar(
                    repo_root,
                    config_file_relative_path,
                    "",
                    project,
                    pipeline,
                )

            self.assertIn("Error running pipeline registration", str(context.exception))

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.create.read_subprocess_outputs"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.create.run_subprocess_registration"
    )
    def test_get_pipeline_config_and_tar_file_not_found_with_fallback(
        self, mock_run_subprocess, mock_read_outputs
    ):
        """Test tar path FileNotFoundError with fallback to remote_path (line 127).

        TODO(#940): This test is for coverage only and should be revised later.
        """
        repo_root = Path("/fake/repo")
        config_file_relative_path = "pipelines/pipeline.yaml"
        project = "test-project"
        pipeline = "test-pipeline"

        with (
            patch("pathlib.Path.exists") as mock_exists,
            patch("pathlib.Path.read_text") as mock_read_text,
            patch("builtins.open", create=True),
            patch("json.loads") as mock_json_loads,
        ):
            mock_exists.return_value = True

            # Mock successful subprocess
            mock_result = Mock()
            mock_result.returncode = 0
            mock_result.stderr = ""
            mock_run_subprocess.return_value = mock_result

            # Mock read_subprocess_outputs to return success with remote_path
            mock_read_outputs.return_value = (True, "Success", "s3://remote/path.tar")

            # Mock tar path file read to raise FileNotFoundError
            # But input file read succeeds
            def read_text_side_effect():
                # First call is for tar_path_file (raises error)
                # Second call is for input_file_path (succeeds)
                # Third call is for function_name_file (succeeds)
                if not hasattr(read_text_side_effect, "call_count"):
                    read_text_side_effect.call_count = 0
                read_text_side_effect.call_count += 1

                if read_text_side_effect.call_count == 1:
                    raise FileNotFoundError("uniflow_tar_path.txt not found")
                elif read_text_side_effect.call_count == 2:
                    return '{"key": "value"}'
                else:
                    return "workflow_function"

            mock_read_text.side_effect = read_text_side_effect
            mock_json_loads.return_value = {"key": "value"}

            result = get_pipeline_config_and_tar(
                repo_root, config_file_relative_path, "", project, pipeline
            )

            # Should use remote_path from status file as fallback
            self.assertEqual(result[1], "s3://remote/path.tar")

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.create.read_subprocess_outputs"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.create.run_subprocess_registration"
    )
    def test_get_pipeline_config_and_tar_input_file_not_found(
        self, mock_run_subprocess, mock_read_outputs
    ):
        """Test input file FileNotFoundError (lines 142-143).

        TODO(#940): This test is for coverage only and should be revised later.
        """
        repo_root = Path("/fake/repo")
        config_file_relative_path = "pipelines/pipeline.yaml"
        project = "test-project"
        pipeline = "test-pipeline"

        with (
            patch("pathlib.Path.exists") as mock_exists,
            patch("pathlib.Path.read_text") as mock_read_text,
        ):
            mock_exists.return_value = True

            # Mock successful subprocess
            mock_result = Mock()
            mock_result.returncode = 0
            mock_result.stderr = ""
            mock_run_subprocess.return_value = mock_result

            # Mock read_subprocess_outputs
            mock_read_outputs.return_value = (True, "Success", "s3://remote/path.tar")

            # Mock read_text to succeed for tar file but fail for input file
            def read_text_side_effect():
                if not hasattr(read_text_side_effect, "call_count"):
                    read_text_side_effect.call_count = 0
                read_text_side_effect.call_count += 1

                if read_text_side_effect.call_count == 1:
                    return "s3://path/to/tar"
                else:
                    raise FileNotFoundError("uniflow_input.txt not found")

            mock_read_text.side_effect = read_text_side_effect

            with self.assertRaises(RuntimeError) as context:
                get_pipeline_config_and_tar(
                    repo_root, config_file_relative_path, "", project, pipeline
                )

            self.assertIn("Could not read uniflow input", str(context.exception))

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.create.read_subprocess_outputs"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.create.run_subprocess_registration"
    )
    def test_get_pipeline_config_and_tar_json_decode_error(
        self, mock_run_subprocess, mock_read_outputs
    ):
        """Test JSON parsing error (line 147).

        TODO(#940): This test is for coverage only and should be revised later.
        """
        repo_root = Path("/fake/repo")
        config_file_relative_path = "pipelines/pipeline.yaml"
        project = "test-project"
        pipeline = "test-pipeline"

        with (
            patch("pathlib.Path.exists") as mock_exists,
            patch("pathlib.Path.read_text") as mock_read_text,
            patch("json.loads") as mock_json_loads,
        ):
            mock_exists.return_value = True

            # Mock successful subprocess
            mock_result = Mock()
            mock_result.returncode = 0
            mock_result.stderr = ""
            mock_run_subprocess.return_value = mock_result

            # Mock read_subprocess_outputs
            mock_read_outputs.return_value = (True, "Success", "s3://remote/path.tar")

            # Mock read_text to succeed for both tar and input files
            def read_text_side_effect():
                if not hasattr(read_text_side_effect, "call_count"):
                    read_text_side_effect.call_count = 0
                read_text_side_effect.call_count += 1

                if read_text_side_effect.call_count == 1:
                    return "s3://path/to/tar"
                else:
                    return "invalid json {{"

            mock_read_text.side_effect = read_text_side_effect
            mock_json_loads.side_effect = json.JSONDecodeError(
                "Invalid JSON", "invalid json {{", 0
            )

            with self.assertRaises(RuntimeError) as context:
                get_pipeline_config_and_tar(
                    repo_root, config_file_relative_path, "", project, pipeline
                )

            self.assertIn("Error parsing uniflow input JSON", str(context.exception))

    def test_empty_trigger_map(self):
        """Test with empty trigger map returns empty dict."""
        result = populate_pipeline_spec_with_trigger_configs({})
        self.assertEqual(result, {})

    def test_none_trigger_map(self):
        """Test with None trigger map returns empty dict."""
        result = populate_pipeline_spec_with_trigger_configs(None)
        self.assertEqual(result, {})

    def test_trigger_with_cron_and_max_concurrency(self):
        """Test trigger with cron schedule and maxConcurrency."""
        trigger_map = {
            "daily-training": {
                "cronSchedule": {"cron": "0 8 * * *"},
                "maxConcurrency": 1,
            }
        }

        result = populate_pipeline_spec_with_trigger_configs(trigger_map)

        self.assertIn("daily-training", result)
        self.assertEqual(result["daily-training"]["cronSchedule"]["cron"], "0 8 * * *")
        self.assertEqual(result["daily-training"]["maxConcurrency"], 1)

    def test_trigger_with_cron_and_batch_policy(self):
        """Test trigger with cron schedule and batchPolicy."""
        trigger_map = {
            "every-minute": {
                "cronSchedule": {"cron": "* * * * *"},
                "batchPolicy": {"batchSize": 5, "wait": "60s"},
            }
        }

        result = populate_pipeline_spec_with_trigger_configs(trigger_map)

        self.assertIn("every-minute", result)
        self.assertEqual(result["every-minute"]["cronSchedule"]["cron"], "* * * * *")
        self.assertEqual(result["every-minute"]["batchPolicy"]["batchSize"], 5)
        self.assertEqual(result["every-minute"]["batchPolicy"]["wait"], "60s")

    def test_trigger_with_interval_schedule(self):
        """Test trigger with interval schedule."""
        trigger_map = {
            "hourly-job": {
                "intervalSchedule": {"interval": "1h"},
                "maxConcurrency": 2,
            }
        }

        result = populate_pipeline_spec_with_trigger_configs(trigger_map)

        self.assertIn("hourly-job", result)
        self.assertEqual(result["hourly-job"]["intervalSchedule"]["interval"], "1h")
        self.assertEqual(result["hourly-job"]["maxConcurrency"], 2)

    def test_trigger_with_parameters_map(self):
        """Test trigger with parametersMap."""
        trigger_map = {
            "training-with-params": {
                "cronSchedule": {"cron": "0 2 * * *"},
                "maxConcurrency": 3,
                "parametersMap": {
                    "sst2-test": {
                        "kwArgs": {
                            "path": "glue",
                            "name": "sst2",
                            "tokenizer_max_length": 256,
                        }
                    },
                    "cola-test": {
                        "kwArgs": {
                            "path": "glue",
                            "name": "cola",
                            "tokenizer_max_length": 128,
                        }
                    },
                },
            }
        }

        result = populate_pipeline_spec_with_trigger_configs(trigger_map)

        self.assertIn("training-with-params", result)
        self.assertIn("parametersMap", result["training-with-params"])
        self.assertIn("sst2-test", result["training-with-params"]["parametersMap"])
        self.assertIn("cola-test", result["training-with-params"]["parametersMap"])
        self.assertEqual(
            result["training-with-params"]["parametersMap"]["sst2-test"]["kwArgs"][
                "tokenizer_max_length"
            ],
            256,
        )

    def test_multiple_triggers(self):
        """Test multiple triggers in the map."""
        trigger_map = {
            "trigger-1": {
                "cronSchedule": {"cron": "* * * * *"},
                "maxConcurrency": 1,
            },
            "trigger-2": {
                "intervalSchedule": {"interval": "30m"},
                "batchPolicy": {"batchSize": 3, "wait": "30s"},
            },
        }

        result = populate_pipeline_spec_with_trigger_configs(trigger_map)

        self.assertEqual(len(result), 2)
        self.assertIn("trigger-1", result)
        self.assertIn("trigger-2", result)
        self.assertIn("cronSchedule", result["trigger-1"])
        self.assertIn("intervalSchedule", result["trigger-2"])

    def test_validation_missing_both_batch_policy_and_max_concurrency(self):
        """Test validation fails when neither batchPolicy nor maxConcurrency is present.

        Ensures proper validation error when trigger lacks execution control.
        """
        trigger_map = {
            "invalid-trigger": {
                "cronSchedule": {"cron": "0 0 * * *"},
            }
        }

        with self.assertRaises(ValueError) as context:
            populate_pipeline_spec_with_trigger_configs(trigger_map)

        self.assertIn("invalid-trigger", str(context.exception))
        self.assertIn("must specify at least one of", str(context.exception))
        self.assertIn("batchPolicy", str(context.exception))
        self.assertIn("maxConcurrency", str(context.exception))

    def test_validation_batch_policy_missing_batch_size(self):
        """Test validation fails when batchPolicy is missing batchSize."""
        trigger_map = {
            "incomplete-batch": {
                "cronSchedule": {"cron": "0 0 * * *"},
                "batchPolicy": {"wait": "60s"},
            }
        }

        with self.assertRaises(ValueError) as context:
            populate_pipeline_spec_with_trigger_configs(trigger_map)

        self.assertIn("incomplete-batch", str(context.exception))
        self.assertIn("batchPolicy must include 'batchSize'", str(context.exception))

    def test_validation_batch_policy_missing_wait(self):
        """Test validation fails when batchPolicy is missing wait."""
        trigger_map = {
            "incomplete-batch": {
                "cronSchedule": {"cron": "0 0 * * *"},
                "batchPolicy": {"batchSize": 5},
            }
        }

        with self.assertRaises(ValueError) as context:
            populate_pipeline_spec_with_trigger_configs(trigger_map)

        self.assertIn("incomplete-batch", str(context.exception))
        self.assertIn("batchPolicy must include 'wait'", str(context.exception))

    def test_trigger_with_both_batch_policy_and_max_concurrency(self):
        """Test trigger can have both batchPolicy and maxConcurrency."""
        trigger_map = {
            "dual-control": {
                "cronSchedule": {"cron": "0 0 * * *"},
                "batchPolicy": {"batchSize": 5, "wait": "60s"},
                "maxConcurrency": 3,
            }
        }

        result = populate_pipeline_spec_with_trigger_configs(trigger_map)

        self.assertIn("dual-control", result)
        self.assertIn("batchPolicy", result["dual-control"])
        self.assertIn("maxConcurrency", result["dual-control"])
        self.assertEqual(result["dual-control"]["batchPolicy"]["batchSize"], 5)
        self.assertEqual(result["dual-control"]["maxConcurrency"], 3)
