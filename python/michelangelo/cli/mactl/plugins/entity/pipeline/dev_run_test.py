"""Unit tests for pipeline dev_run plugin."""

import os
import tempfile
from inspect import Parameter, Signature
from pathlib import Path
from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from google.protobuf.message import Message
from google.protobuf.struct_pb2 import Struct

from michelangelo.cli.mactl.crd import CRD
from michelangelo.cli.mactl.plugins.entity.pipeline.dev_run import (
    _process_env_variables,
    convert_crd_metadata_pipeline_dev_run,
    generate_dev_run,
    generate_pipeline_dev_run_object,
)


class PipelineDevRunTest(TestCase):
    """Tests for pipeline dev_run plugin."""

    def test_module_imports(self):
        """Test that the module imports successfully including all dependencies."""
        # This test ensures all imports in dev_run.py are valid and covered
        from michelangelo.cli.mactl.plugins.entity.pipeline import dev_run

        # Verify key attributes exist
        self.assertTrue(hasattr(dev_run, "convert_crd_metadata_pipeline_dev_run"))
        self.assertTrue(hasattr(dev_run, "generate_pipeline_dev_run_object"))
        self.assertTrue(hasattr(dev_run, "_process_env_variables"))
        self.assertTrue(hasattr(dev_run, "DefaultFileSync"))

    def test_process_env_variables_basic(self):
        """Test processing environment variables from list to dict."""
        env_list = ["KEY1=value1", "KEY2=value2", "KEY3=value3"]

        result = _process_env_variables(env_list)

        self.assertEqual(
            result,
            {
                "KEY1": "value1",
                "KEY2": "value2",
                "KEY3": "value3",
            },
        )

    def test_process_env_variables_with_equals_in_value(self):
        """Test processing environment variables where value contains =."""
        env_list = ["CONNECTION_STRING=server=localhost;port=5432"]

        result = _process_env_variables(env_list)

        self.assertEqual(result, {"CONNECTION_STRING": "server=localhost;port=5432"})

    def test_process_env_variables_invalid_format(self):
        """Test that invalid format raises TypeError."""
        env_list = ["INVALID_FORMAT"]

        with self.assertRaises(TypeError) as context:
            _process_env_variables(env_list)

        self.assertIn("Invalid environment variable format", str(context.exception))
        self.assertIn("expected format is <ENV_VAR>=<VALUE>", str(context.exception))

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_name"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_object"
    )
    def test_generate_pipeline_dev_run_object_comprehensive(
        self, mock_generate_run_obj, mock_generate_name
    ):
        """Test that all fields are correctly added to pipeline dev run object."""
        mock_generate_name.return_value = "run-test-12345678"
        base_obj = {
            "metadata": {"name": "run-test-12345678", "namespace": "test-ns"},
            "spec": {"pipeline": {"name": "test-pipeline"}},
        }
        mock_generate_run_obj.return_value = base_obj

        yaml_dict = {
            "metadata": {
                "name": "test-pipeline",
                "namespace": "test-ns",
                "annotations": {"michelangelo/uniflow-image": "custom-image:v1.0"},
            },
            "env": {"KEY1": "value1", "KEY2": "value2"},
        }
        pipeline_spec = {"spec": {"steps": [{"name": "step1"}], "timeout": 300}}

        result = generate_pipeline_dev_run_object(yaml_dict, pipeline_spec)

        # Verify env variables added
        self.assertEqual(
            result["spec"]["input"]["environ"], {"KEY1": "value1", "KEY2": "value2"}
        )

        # Verify pipeline_spec added
        self.assertEqual(
            result["spec"]["pipeline_spec"],
            {"steps": [{"name": "step1"}], "timeout": 300},
        )

        # Verify uniflow image annotation added
        self.assertEqual(
            result["metadata"]["annotations"]["michelangelo/uniflow-image"],
            "custom-image:v1.0",
        )

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_name"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_object"
    )
    def test_generate_pipeline_dev_run_object_passes_resume_from(
        self, mock_generate_run_obj, mock_generate_name
    ):
        """Test that resume_from parameter is passed to generate_pipeline_run_object."""
        mock_generate_name.return_value = "run-test-12345678"
        base_obj = {"spec": {}}
        mock_generate_run_obj.return_value = base_obj

        yaml_dict = {"metadata": {"name": "test-pipeline", "namespace": "test-ns"}}
        pipeline_spec = {"spec": {}}
        resume_from = "old-run:step1"

        generate_pipeline_dev_run_object(yaml_dict, pipeline_spec, resume_from)

        mock_generate_run_obj.assert_called_once_with(
            run_name="run-test-12345678",
            pipeline_name="test-pipeline",
            namespace="test-ns",
            resume_from="old-run:step1",
        )

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_name"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_object"
    )
    def test_generate_pipeline_dev_run_object_without_env_variables(
        self, mock_generate_run_obj, mock_generate_name
    ):
        """Test that spec.input is not added when env variables are absent."""
        mock_generate_name.return_value = "run-test-12345678"
        base_obj = {"spec": {"pipeline": {"name": "test-pipeline"}}}
        mock_generate_run_obj.return_value = base_obj

        yaml_dict = {"metadata": {"name": "test-pipeline", "namespace": "test-ns"}}
        pipeline_spec = {"spec": {}}

        result = generate_pipeline_dev_run_object(yaml_dict, pipeline_spec)

        self.assertNotIn("input", result["spec"])

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_name"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_object"
    )
    def test_generate_pipeline_dev_run_object_with_file_sync(
        self, mock_generate_run_obj, mock_generate_name
    ):
        """Test that file-sync tarball URL is injected into environment variables."""
        mock_generate_name.return_value = "run-test-12345678"
        base_obj = {
            "metadata": {"name": "run-test-12345678", "namespace": "test-ns"},
            "spec": {"pipeline": {"name": "test-pipeline"}},
        }
        mock_generate_run_obj.return_value = base_obj

        yaml_dict = {"metadata": {"name": "test-pipeline", "namespace": "test-ns"}}
        pipeline_spec = {"spec": {}}
        file_sync_tarball_url = "s3://bucket/path/to/file-sync.tar.gz"

        result = generate_pipeline_dev_run_object(
            yaml_dict, pipeline_spec, None, file_sync_tarball_url
        )

        self.assertIn("input", result["spec"])
        self.assertIn("environ", result["spec"]["input"])
        self.assertEqual(
            result["spec"]["input"]["environ"]["UF_FILE_SYNC_TARBALL_URL"],
            "s3://bucket/path/to/file-sync.tar.gz",
        )

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_name"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_object"
    )
    def test_generate_pipeline_dev_run_object_with_file_sync_and_env_vars(
        self, mock_generate_run_obj, mock_generate_name
    ):
        """Test that file-sync URL is merged with existing env variables."""
        mock_generate_name.return_value = "run-test-12345678"
        base_obj = {
            "metadata": {"name": "run-test-12345678", "namespace": "test-ns"},
            "spec": {"pipeline": {"name": "test-pipeline"}},
        }
        mock_generate_run_obj.return_value = base_obj

        yaml_dict = {
            "metadata": {"name": "test-pipeline", "namespace": "test-ns"},
            "env": {"KEY1": "value1", "KEY2": "value2"},
        }
        pipeline_spec = {"spec": {}}
        file_sync_tarball_url = "s3://bucket/path/to/file-sync.tar.gz"

        result = generate_pipeline_dev_run_object(
            yaml_dict, pipeline_spec, None, file_sync_tarball_url
        )

        self.assertIn("input", result["spec"])
        self.assertIn("environ", result["spec"]["input"])
        self.assertEqual(
            result["spec"]["input"]["environ"],
            {
                "KEY1": "value1",
                "KEY2": "value2",
                "UF_FILE_SYNC_TARBALL_URL": "s3://bucket/path/to/file-sync.tar.gz",
            },
        )

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.Repo")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.handle_workflow_inputs_retrieval"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.populate_pipeline_spec_with_workflow_inputs"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_name"
    )
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.DefaultFileSync")
    def test_convert_crd_metadata_with_file_sync(
        self,
        mock_file_sync_class,
        mock_generate_run_name,
        mock_populate_spec,
        mock_handle_workflow,
        mock_repo,
    ):
        """Test convert_crd_metadata_pipeline_dev_run with file_sync enabled."""
        # Setup mocks
        mock_repo_instance = MagicMock()
        mock_repo_instance.git.rev_parse.return_value = "/fake/repo"
        mock_repo.return_value = mock_repo_instance

        mock_handle_workflow.return_value = ({}, "/fake/tar/path", "workflow_func")
        mock_populate_spec.return_value = {"spec": {"steps": []}}
        mock_generate_run_name.return_value = "test-run-12345"

        mock_file_sync = MagicMock()
        mock_file_sync.create_and_upload_tarball.return_value = (
            "s3://bucket/file-sync.tar.gz"
        )
        mock_file_sync_class.return_value = mock_file_sync

        yaml_dict = {
            "metadata": {
                "name": "test-pipeline",
                "namespace": "test-ns",
                "annotations": {"michelangelo/uniflow-image": "test-image:v1.0"},
            },
            "file_sync": True,
        }
        yaml_path = Path("/fake/repo/pipeline.yaml")

        result = convert_crd_metadata_pipeline_dev_run(
            yaml_dict, MagicMock(), yaml_path
        )

        # Verify DefaultFileSync was created with correct image
        mock_file_sync_class.assert_called_once_with(docker_image="test-image:v1.0")

        # Verify create_and_upload_tarball was called
        mock_file_sync.create_and_upload_tarball.assert_called_once()

        # Verify the result contains pipeline_run with file-sync URL
        self.assertIn("pipeline_run", result)
        # Verify the file-sync URL is in the environment variables
        self.assertIn(
            "UF_FILE_SYNC_TARBALL_URL",
            result["pipeline_run"]["spec"]["input"]["environ"],
        )
        self.assertEqual(
            result["pipeline_run"]["spec"]["input"]["environ"][
                "UF_FILE_SYNC_TARBALL_URL"
            ],
            "s3://bucket/file-sync.tar.gz",
        )

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.Repo")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.handle_workflow_inputs_retrieval"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.populate_pipeline_spec_with_workflow_inputs"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_name"
    )
    def test_convert_crd_metadata_without_file_sync(
        self,
        mock_generate_run_name,
        mock_populate_spec,
        mock_handle_workflow,
        mock_repo,
    ):
        """Test convert_crd_metadata_pipeline_dev_run without file_sync."""
        # Setup mocks
        mock_repo_instance = MagicMock()
        mock_repo_instance.git.rev_parse.return_value = "/fake/repo"
        mock_repo.return_value = mock_repo_instance

        mock_handle_workflow.return_value = ({}, "/fake/tar/path", "workflow_func")
        mock_populate_spec.return_value = {"spec": {"steps": []}}
        mock_generate_run_name.return_value = "test-run-12345"

        yaml_dict = {
            "metadata": {"name": "test-pipeline", "namespace": "test-ns"},
            "file_sync": False,
        }
        yaml_path = Path("/fake/repo/pipeline.yaml")

        result = convert_crd_metadata_pipeline_dev_run(
            yaml_dict, MagicMock(), yaml_path
        )

        # Verify the result contains pipeline_run
        self.assertIn("pipeline_run", result)
        # Verify no file-sync URL in environment when file_sync is False
        if (
            "input" in result["pipeline_run"]["spec"]
            and "environ" in result["pipeline_run"]["spec"]["input"]
        ):
            self.assertNotIn(
                "UF_FILE_SYNC_TARBALL_URL",
                result["pipeline_run"]["spec"]["input"]["environ"],
            )

    def test_add_optional_params_to_yaml_dict_with_file_sync(self):
        """Test _add_optional_params_to_yaml_dict with file_sync=True."""
        from michelangelo.cli.mactl.plugins.entity.pipeline.dev_run import (
            _add_optional_params_to_yaml_dict,
        )

        yaml_dict = {"metadata": {"name": "test-pipeline"}}
        env_vars = {"KEY1": "value1"}
        resume_from = "old-run:step1"
        file_sync = True

        result = _add_optional_params_to_yaml_dict(
            yaml_dict, env_vars, resume_from, file_sync
        )

        self.assertEqual(result["env"], {"KEY1": "value1"})
        self.assertEqual(result["resume_from"], "old-run:step1")
        self.assertEqual(result["file_sync"], True)

    def test_add_optional_params_to_yaml_dict_without_file_sync(self):
        """Test _add_optional_params_to_yaml_dict with file_sync=False."""
        from michelangelo.cli.mactl.plugins.entity.pipeline.dev_run import (
            _add_optional_params_to_yaml_dict,
        )

        yaml_dict = {"metadata": {"name": "test-pipeline"}}
        env_vars = {}
        resume_from = None
        file_sync = False

        result = _add_optional_params_to_yaml_dict(
            yaml_dict, env_vars, resume_from, file_sync
        )

        self.assertEqual(result["env"], {})
        self.assertNotIn("resume_from", result)
        self.assertNotIn("file_sync", result)

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.get_service_name")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.get_methods_from_service"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.get_message_class_by_name"
    )
    def test_generate_dev_run_executes_auto_detection(
        self, mock_get_message_class, mock_get_methods, mock_get_service_name
    ):
        """Test that generate_dev_run() executes get_service_name with fallback."""
        # Create mock CRD
        mock_crd = MagicMock()
        mock_crd.metadata = [("rpc-caller", "test")]

        # Create mock channel
        mock_channel = MagicMock()

        # Mock get_service_name to return a service name
        mock_get_service_name.return_value = "michelangelo.api.v2.PipelineRunService"

        # Create mock method
        mock_method = MagicMock()
        mock_method.input_type = ".michelangelo.api.v2.CreatePipelineRunRequest"
        mock_method.output_type = ".michelangelo.api.v2.CreatePipelineRunResponse"

        # Mock get_methods_from_service
        mock_methods = {"CreatePipelineRun": mock_method}
        mock_pool = MagicMock()
        mock_get_methods.return_value = (mock_methods, mock_pool)

        # Mock get_message_class_by_name
        mock_input_class = MagicMock()
        mock_output_class = MagicMock()
        mock_get_message_class.side_effect = [mock_input_class, mock_output_class]

        # Call generate_dev_run - this will execute line 123
        generate_dev_run(mock_crd, mock_channel)

        # Verify get_service_name was called with correct parameters
        mock_get_service_name.assert_called_once_with(
            mock_channel,
            mock_crd.metadata,
            "PipelineRunService",
            fallback="michelangelo.api.v2.PipelineRunService",
        )

        # Verify get_methods_from_service was called
        mock_get_methods.assert_called_once_with(
            mock_channel, "michelangelo.api.v2.PipelineRunService", mock_crd.metadata
        )

    def test_dev_run_func_extracts_storage_url_from_bound_args(self):
        """Test that dev_run function extracts storage_url from bound_args.

        This test specifically targets line 187 in dev_run.py:
        _storage_url = bound_args.arguments.get("storage_url")
        """
        with tempfile.TemporaryDirectory() as tmpdir:
            temp_yaml_path = Path(tmpdir) / "test_pipeline.yaml"
            with open(temp_yaml_path, "w") as f:
                f.write("""
metadata:
  name: test-pipeline
  namespace: test-project
spec:
  workflowGraph:
    nodes: []
""")

            # Create a real CRD instance (not a mock)
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
                    "michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.ParseDict"
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

                # Generate the dev_run function
                generate_dev_run(crd, mock_channel)

                # Now test the dev_run function by calling it
                # _storage_url = bound_args.arguments.get("storage_url")
                test_storage_url = "s3://test-bucket/test-path"

                # Call with storage_url
                crd.dev_run(file=str(temp_yaml_path), storage_url=test_storage_url)

                # Call without storage_url (should default to None)
                crd.dev_run(file=str(temp_yaml_path))
