"""Unit tests for pipeline run plugin.

Tests helper functions for pipeline run generation.
"""

from unittest import TestCase
from unittest.mock import MagicMock, patch

from michelangelo.cli.mactl.plugins.pipeline.run import (
    convert_crd_metadata_pipeline_run,
    generate_pipeline_run_name,
    generate_pipeline_run_object,
    generate_run,
    parse_resume_from,
)


class PipelineRunTest(TestCase):
    """Tests for pipeline run plugin."""

    @patch("michelangelo.cli.mactl.plugins.pipeline.run.time")
    @patch("michelangelo.cli.mactl.plugins.pipeline.run.uuid")
    def test_generate_pipeline_run_name(self, mock_uuid, mock_time):
        """Test pipeline run name generation."""
        mock_time.time.return_value = 1705152000  # 2024-01-13 12:00:00
        mock_uuid.uuid4.return_value = MagicMock()
        mock_uuid.uuid4.return_value.__str__ = (
            lambda x: "abc123de-f456-7890-1234-567890abcdef"
        )

        result = generate_pipeline_run_name()

        self.assertEqual(result, "run-1705152000-abc123de")
        mock_time.time.assert_called_once()
        mock_uuid.uuid4.assert_called_once()

    def test_generate_pipeline_run_object_basic(self):
        """Test basic pipeline run object generation."""
        result = generate_pipeline_run_object(
            run_name="run-123-abc",
            pipeline_name="test-pipeline",
            namespace="test-ns",
        )

        # Verify structure
        self.assertIn("typeMeta", result)
        self.assertEqual(result["typeMeta"]["kind"], "PipelineRun")
        self.assertEqual(result["typeMeta"]["apiVersion"], "michelangelo.api/v2")

        self.assertIn("metadata", result)
        self.assertEqual(result["metadata"]["name"], "run-123-abc")
        self.assertEqual(result["metadata"]["namespace"], "test-ns")

        self.assertIn("spec", result)
        self.assertEqual(result["spec"]["pipeline"]["name"], "test-pipeline")
        self.assertEqual(result["spec"]["pipeline"]["namespace"], "test-ns")
        self.assertEqual(result["spec"]["actor"]["name"], "mactl-user")

        # Verify no resume spec when resume_from not provided
        self.assertNotIn("resume", result["spec"])

    @patch("michelangelo.cli.mactl.plugins.pipeline.run.parse_resume_from")
    def test_generate_pipeline_run_object_with_resume_from(self, mock_parse):
        """Test pipeline run object generation with resume_from."""
        mock_resume_spec = {
            "pipelineRun": {"name": "previous-run", "namespace": "test-ns"},
            "resumeFrom": ["step-1"],
        }
        mock_parse.return_value = mock_resume_spec

        result = generate_pipeline_run_object(
            run_name="run-123-abc",
            pipeline_name="test-pipeline",
            namespace="test-ns",
            resume_from="previous-run:step-1",
        )

        # Verify parse_resume_from was called with correct args
        mock_parse.assert_called_once_with("previous-run:step-1", "test-ns")

        # Verify the returned resume spec was added to result
        self.assertIn("resume", result["spec"])
        self.assertEqual(result["spec"]["resume"], mock_resume_spec)

    def test_parse_resume_from_with_step_name(self):
        """Test parsing resume_from with step name."""
        result = parse_resume_from("pipeline-run-123:my-step", "test-ns")

        self.assertIsNotNone(result)
        self.assertEqual(result["pipelineRun"]["name"], "pipeline-run-123")
        self.assertEqual(result["pipelineRun"]["namespace"], "test-ns")
        self.assertEqual(result["resumeFrom"], ["my-step"])

    def test_parse_resume_from_without_step_name(self):
        """Test parsing resume_from without step name."""
        result = parse_resume_from("pipeline-run-123", "test-ns")

        self.assertIsNotNone(result)
        self.assertEqual(result["pipelineRun"]["name"], "pipeline-run-123")
        self.assertEqual(result["pipelineRun"]["namespace"], "test-ns")
        self.assertEqual(result["resumeFrom"], [])

    def test_parse_resume_from_empty_string(self):
        """Test parsing empty resume_from returns None."""
        result = parse_resume_from("", "test-ns")

        self.assertIsNone(result)

    def test_parse_resume_from_none(self):
        """Test parsing None resume_from returns None."""
        result = parse_resume_from(None, "test-ns")

        self.assertIsNone(result)

    def test_convert_crd_metadata_pipeline_run_invalid_input(self):
        """Test that invalid input raises ValueError."""
        mock_crd_class = MagicMock()

        with self.assertRaises(ValueError) as context:
            convert_crd_metadata_pipeline_run("not a dict", mock_crd_class, None)

        self.assertIn("Expected a dictionary", str(context.exception))

    def test_convert_crd_metadata_pipeline_run_missing_namespace(self):
        """Test that missing namespace raises ValueError."""
        yaml_dict = {"name": "test-pipeline"}
        mock_crd_class = MagicMock()

        with self.assertRaises(ValueError) as context:
            convert_crd_metadata_pipeline_run(yaml_dict, mock_crd_class, None)

        self.assertIn("--namespace is required", str(context.exception))

    def test_convert_crd_metadata_pipeline_run_missing_name(self):
        """Test that missing name raises ValueError."""
        yaml_dict = {"namespace": "test-ns"}
        mock_crd_class = MagicMock()

        with self.assertRaises(ValueError) as context:
            convert_crd_metadata_pipeline_run(yaml_dict, mock_crd_class, None)

        self.assertIn("--name is required", str(context.exception))

    @patch("michelangelo.cli.mactl.plugins.pipeline.run.generate_pipeline_run_name")
    @patch("michelangelo.cli.mactl.plugins.pipeline.run.generate_pipeline_run_object")
    def test_convert_crd_metadata_pipeline_run_basic(
        self, mock_generate_obj, mock_generate_name
    ):
        """Test basic conversion of CRD metadata for pipeline run."""
        yaml_dict = {
            "namespace": "test-ns",
            "name": "test-pipeline",
        }
        mock_crd_class = MagicMock()

        mock_generate_name.return_value = "run-123-abc"
        mock_pipeline_run = {
            "metadata": {"name": "run-123-abc"},
            "spec": {},
        }
        mock_generate_obj.return_value = mock_pipeline_run

        result = convert_crd_metadata_pipeline_run(yaml_dict, mock_crd_class, None)

        # Verify result wraps pipeline_run
        self.assertIn("pipeline_run", result)
        self.assertIs(result["pipeline_run"], mock_pipeline_run)

        # Verify generate_pipeline_run_name was called
        mock_generate_name.assert_called_once()

        # Verify generate_pipeline_run_object was called correctly
        mock_generate_obj.assert_called_once_with(
            run_name="run-123-abc",
            pipeline_name="test-pipeline",
            namespace="test-ns",
            resume_from=None,
        )

    @patch("michelangelo.cli.mactl.plugins.pipeline.run.generate_pipeline_run_name")
    @patch("michelangelo.cli.mactl.plugins.pipeline.run.generate_pipeline_run_object")
    def test_convert_crd_metadata_pipeline_run_with_resume_from(
        self, mock_generate_obj, mock_generate_name
    ):
        """Test conversion with resume_from parameter."""
        yaml_dict = {
            "namespace": "test-ns",
            "name": "test-pipeline",
            "resume_from": "previous-run:step-1",
        }
        mock_crd_class = MagicMock()

        mock_generate_name.return_value = "run-123-abc"
        mock_pipeline_run = {"metadata": {}, "spec": {}}
        mock_generate_obj.return_value = mock_pipeline_run

        convert_crd_metadata_pipeline_run(yaml_dict, mock_crd_class, None)

        # Verify resume_from was passed to generate_pipeline_run_object
        mock_generate_obj.assert_called_once_with(
            run_name="run-123-abc",
            pipeline_name="test-pipeline",
            namespace="test-ns",
            resume_from="previous-run:step-1",
        )

    @patch("michelangelo.cli.mactl.plugins.pipeline.run.get_methods_from_service")
    @patch("michelangelo.cli.mactl.plugins.pipeline.run.get_service_name")
    def test_generate_run_with_auto_detection(
        self, mock_get_service_name, mock_get_methods
    ):
        """Test generate_run uses get_service_name for auto-detection."""
        mock_crd = MagicMock()
        mock_crd.metadata = [("rpc-caller", "test")]
        mock_channel = MagicMock()
        mock_get_service_name.return_value = (
            "michelangelo.api.v2beta1.PipelineRunService"
        )
        mock_method = MagicMock()
        mock_method.input_type = ".michelangelo.api.v2beta1.CreatePipelineRunRequest"
        mock_method.output_type = ".michelangelo.api.v2beta1.PipelineRun"
        mock_methods = {"CreatePipelineRun": mock_method}
        mock_pool = MagicMock()
        mock_get_methods.return_value = (mock_methods, mock_pool)
        generate_run(mock_crd, mock_channel)
        mock_get_service_name.assert_called_once_with(
            mock_channel,
            mock_crd.metadata,
            "PipelineRunService",
            fallback="michelangelo.api.v2.PipelineRunService",
        )
        mock_get_methods.assert_called_once_with(
            mock_channel,
            "michelangelo.api.v2beta1.PipelineRunService",
            mock_crd.metadata,
        )
