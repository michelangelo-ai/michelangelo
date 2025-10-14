"""
Unit tests for trigger_run plugin functions.

Tests trigger run functionality including pipeline validation, YAML parsing, and kubectl integration.
"""

import subprocess
from pathlib import Path
from unittest import TestCase
from unittest.mock import Mock, patch, mock_open

import yaml

from michelangelo.cli.mactl.mactl import (
    handle_args,
    get_single_arg,
)
from michelangelo.cli.mactl.plugins.trigger_run.run import (
    validate_pipeline_exists,
    parse_trigger_yaml,
    apply_trigger_run,
    generate_run,
)


class TriggerRunFunctionsTest(TestCase):
    """
    Tests for trigger run plugin functions
    """

    @patch('michelangelo.cli.mactl.plugins.trigger_run.run.subprocess.run')
    def test_validate_pipeline_exists_success(self, mock_subprocess_run):
        """
        Test validate_pipeline_exists when pipeline exists
        """
        # Mock successful kubectl get pipeline
        mock_result = Mock()
        mock_result.returncode = 0
        mock_subprocess_run.return_value = mock_result

        result = validate_pipeline_exists("test-pipeline", "test-namespace")

        self.assertTrue(result)
        mock_subprocess_run.assert_called_once_with(
            ["kubectl", "get", "pipeline", "test-pipeline", "-n", "test-namespace"],
            capture_output=True,
            text=True,
            check=True
        )

    @patch('michelangelo.cli.mactl.plugins.trigger_run.run.subprocess.run')
    def test_validate_pipeline_exists_failure(self, mock_subprocess_run):
        """
        Test validate_pipeline_exists when pipeline doesn't exist
        """
        # Mock kubectl get pipeline failure
        mock_subprocess_run.side_effect = subprocess.CalledProcessError(
            returncode=1,
            cmd=["kubectl", "get", "pipeline", "missing-pipeline", "-n", "test-namespace"],
            stderr="pipelines.michelangelo.api \"missing-pipeline\" not found"
        )

        result = validate_pipeline_exists("missing-pipeline", "test-namespace")

        self.assertFalse(result)
        mock_subprocess_run.assert_called_once_with(
            ["kubectl", "get", "pipeline", "missing-pipeline", "-n", "test-namespace"],
            capture_output=True,
            text=True,
            check=True
        )

    @patch('michelangelo.cli.mactl.plugins.trigger_run.run.subprocess.run')
    def test_validate_pipeline_exists_exception(self, mock_subprocess_run):
        """
        Test validate_pipeline_exists when subprocess raises unexpected exception
        """
        # Mock unexpected exception
        mock_subprocess_run.side_effect = Exception("Unexpected error")

        result = validate_pipeline_exists("test-pipeline", "test-namespace")

        self.assertFalse(result)

    def test_parse_trigger_yaml_success(self):
        """
        Test parse_trigger_yaml with valid YAML file
        """
        yaml_content = """
apiVersion: michelangelo.api/v2
kind: TriggerRun
metadata:
  name: test-trigger
  namespace: test-namespace
spec:
  pipeline:
    name: test-pipeline
    namespace: test-namespace
  trigger:
    cronSchedule:
      cron: "* * * * *"
"""
        with patch('pathlib.Path.exists', return_value=True):
            with patch('pathlib.Path.open', mock_open(read_data=yaml_content)):
                result = parse_trigger_yaml("/fake/path/trigger.yaml")

        expected_data = {
            'apiVersion': 'michelangelo.api/v2',
            'kind': 'TriggerRun',
            'metadata': {
                'name': 'test-trigger',
                'namespace': 'test-namespace'
            },
            'spec': {
                'pipeline': {
                    'name': 'test-pipeline',
                    'namespace': 'test-namespace'
                },
                'trigger': {
                    'cronSchedule': {
                        'cron': '* * * * *'
                    }
                }
            }
        }
        self.assertEqual(result, expected_data)

    def test_parse_trigger_yaml_file_not_found(self):
        """
        Test parse_trigger_yaml with non-existent file
        """
        with patch('pathlib.Path.exists', return_value=False):
            with self.assertRaises(FileNotFoundError) as context:
                parse_trigger_yaml("/fake/path/missing.yaml")

        self.assertIn("Trigger file not found", str(context.exception))

    def test_parse_trigger_yaml_invalid_yaml(self):
        """
        Test parse_trigger_yaml with invalid YAML content
        """
        invalid_yaml = "invalid: yaml: content: ["
        with patch('pathlib.Path.exists', return_value=True):
            with patch('pathlib.Path.open', mock_open(read_data=invalid_yaml)):
                with self.assertRaises(yaml.YAMLError):
                    parse_trigger_yaml("/fake/path/invalid.yaml")

    @patch('michelangelo.cli.mactl.plugins.trigger_run.run.subprocess.run')
    def test_apply_trigger_run_success(self, mock_subprocess_run):
        """
        Test apply_trigger_run with successful kubectl apply
        """
        # Mock successful kubectl apply
        mock_result = Mock()
        mock_result.returncode = 0
        mock_result.stdout = "triggerrun.michelangelo.api/test-trigger created"
        mock_subprocess_run.return_value = mock_result

        # Should not raise exception
        apply_trigger_run("/fake/path/trigger.yaml")

        mock_subprocess_run.assert_called_once_with(
            ["kubectl", "apply", "-f", "/fake/path/trigger.yaml"],
            capture_output=True,
            text=True,
            check=True
        )

    @patch('michelangelo.cli.mactl.plugins.trigger_run.run.subprocess.run')
    def test_apply_trigger_run_failure(self, mock_subprocess_run):
        """
        Test apply_trigger_run with kubectl apply failure
        """
        # Mock kubectl apply failure
        mock_subprocess_run.side_effect = subprocess.CalledProcessError(
            returncode=1,
            cmd=["kubectl", "apply", "-f", "/fake/path/trigger.yaml"],
            stderr="error validating data"
        )

        with self.assertRaises(subprocess.CalledProcessError):
            apply_trigger_run("/fake/path/trigger.yaml")

    @patch('michelangelo.cli.mactl.plugins.trigger_run.run.apply_trigger_run')
    @patch('michelangelo.cli.mactl.plugins.trigger_run.run.validate_pipeline_exists')
    @patch('michelangelo.cli.mactl.plugins.trigger_run.run.parse_trigger_yaml')
    def test_generate_run_success(self, mock_parse_yaml, mock_validate_pipeline, mock_apply_trigger):
        """
        Test generate_run function with successful execution
        """
        # Setup mocks
        mock_parse_yaml.return_value = {
            'spec': {
                'pipeline': {
                    'name': 'test-pipeline',
                    'namespace': 'test-namespace'
                }
            }
        }
        mock_validate_pipeline.return_value = True
        mock_apply_trigger.return_value = None

        # Create mock CRD and channel
        mock_crd = Mock()
        mock_channel = Mock()

        # Execute generate_run
        generate_run(mock_crd, mock_channel)

        # Verify that run method was attached to CRD
        self.assertTrue(hasattr(mock_crd, 'run'))

    def test_generate_run_creates_proper_signature(self):
        """
        Test that generate_run creates a function with proper signature
        """
        mock_crd = Mock()
        mock_channel = Mock()

        generate_run(mock_crd, mock_channel)

        # Verify run method exists and is callable
        self.assertTrue(hasattr(mock_crd, 'run'))
        self.assertTrue(callable(mock_crd.run))


class CommandAliasTest(TestCase):
    """
    Tests for command alias functionality (trigger -> trigger_run)
    """

    @patch('michelangelo.cli.mactl.mactl.parse_args')
    def test_handle_args_trigger_alias(self, mock_parse_args):
        """
        Test that 'trigger' command is converted to 'trigger_run' internally
        """
        # Mock parse_args to return trigger command
        mock_parse_args.return_value = (
            ['trigger', 'run'],
            {'file': ['/fake/path/trigger.yaml']}
        )

        user_command_crd, user_command_action, kwargs = handle_args()

        # Verify alias conversion
        self.assertEqual(user_command_crd, 'trigger_run')
        self.assertEqual(user_command_action, 'run')
        self.assertEqual(kwargs, {'file': ['/fake/path/trigger.yaml']})

    @patch('michelangelo.cli.mactl.mactl.parse_args')
    def test_handle_args_trigger_run_unchanged(self, mock_parse_args):
        """
        Test that 'trigger_run' command remains unchanged
        """
        # Mock parse_args to return trigger_run command
        mock_parse_args.return_value = (
            ['trigger_run', 'run'],
            {'file': ['/fake/path/trigger.yaml']}
        )

        user_command_crd, user_command_action, kwargs = handle_args()

        # Verify no conversion for trigger_run
        self.assertEqual(user_command_crd, 'trigger_run')
        self.assertEqual(user_command_action, 'run')
        self.assertEqual(kwargs, {'file': ['/fake/path/trigger.yaml']})

    @patch('michelangelo.cli.mactl.mactl.parse_args')
    def test_handle_args_file_validation_trigger(self, mock_parse_args):
        """
        Test file parameter validation for trigger run command
        """
        # Mock parse_args with missing file parameter
        mock_parse_args.return_value = (
            ['trigger', 'run'],
            {}  # No file parameter
        )

        # Should raise KeyError for missing file parameter
        with self.assertRaises(KeyError):
            handle_args()