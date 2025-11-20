"""Unit tests for pipeline dev_run plugin.
"""

from unittest import TestCase
from unittest.mock import patch

from michelangelo.cli.mactl.plugins.pipeline.dev_run import (
    _process_env_variables,
    generate_pipeline_dev_run_object,
)


class PipelineDevRunTest(TestCase):
    """Tests for pipeline dev_run plugin."""

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

    @patch("michelangelo.cli.mactl.plugins.pipeline.dev_run.generate_pipeline_run_name")
    @patch(
        "michelangelo.cli.mactl.plugins.pipeline.dev_run.generate_pipeline_run_object"
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

    @patch("michelangelo.cli.mactl.plugins.pipeline.dev_run.generate_pipeline_run_name")
    @patch(
        "michelangelo.cli.mactl.plugins.pipeline.dev_run.generate_pipeline_run_object"
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

    @patch("michelangelo.cli.mactl.plugins.pipeline.dev_run.generate_pipeline_run_name")
    @patch(
        "michelangelo.cli.mactl.plugins.pipeline.dev_run.generate_pipeline_run_object"
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

    @patch("michelangelo.cli.mactl.plugins.pipeline.dev_run.generate_pipeline_run_name")
    @patch(
        "michelangelo.cli.mactl.plugins.pipeline.dev_run.generate_pipeline_run_object"
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

    @patch("michelangelo.cli.mactl.plugins.pipeline.dev_run.generate_pipeline_run_name")
    @patch(
        "michelangelo.cli.mactl.plugins.pipeline.dev_run.generate_pipeline_run_object"
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
