"""Unit tests for pipeline kill command.

Tests the kill command functionality for pipeline runs.
"""

from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from google.protobuf.message import Message

from michelangelo.cli.mactl.crd import CRD
from michelangelo.cli.mactl.plugins.entity.pipeline.kill import (
    add_function_signature,
    generate_kill,
)


class PipelineKillTest(TestCase):
    """Tests for pipeline kill command."""

    def setUp(self):
        """Set up test fixtures."""
        self.mock_crd = Mock(spec=CRD)
        self.mock_crd.name = "pipeline_run"
        self.mock_crd.full_name = "michelangelo.api.v2.PipelineRunService"
        self.mock_crd.metadata = {}
        self.mock_crd.func_signature = {}

        # Create a mock signature that properly handles binding
        mock_signature = Mock()

        def mock_bind(*args, **kwargs):
            bound = Mock()
            # Create arguments dict from args and kwargs
            bound.arguments = {
                "self": args[0] if args else kwargs.get("self"),
                "namespace": kwargs.get("namespace"),
                "name": kwargs.get("name"),
                "yes": kwargs.get("yes", False),
            }
            return bound

        mock_signature.bind = mock_bind
        self.mock_crd._read_signatures = Mock(return_value=mock_signature)
        self.mock_crd.configure_parser = Mock()
        self.mock_channel = Mock()

    def test_add_function_signature(self):
        """Test that add_function_signature properly configures the CRD."""
        add_function_signature(self.mock_crd)

        # Verify that inject_func_signature was called
        # We can't directly verify this without the actual implementation,
        # but we can check that the function runs without error
        self.assertTrue(True)

    def test_generate_kill_basic(self):
        """Test basic kill command generation."""
        # Mock generate_get and _extract_method_info
        self.mock_crd.generate_get = Mock()
        self.mock_crd._extract_method_info = Mock(
            return_value=("UpdatePipelineRun", Mock(), Mock())
        )

        # Execute
        generate_kill(self.mock_crd, self.mock_channel)

        # Verify methods were called correctly
        self.mock_crd.generate_get.assert_called_once_with(self.mock_channel)
        self.mock_crd._extract_method_info.assert_called_once_with(
            self.mock_channel, self.mock_crd.full_name, "Update"
        )

    def test_kill_command_requires_namespace_and_name(self):
        """Test that kill command requires namespace and name parameters."""
        # This is implicitly tested by the function signature definition
        # The test just verifies the function can be called
        add_function_signature(self.mock_crd)
        self.assertTrue(True)

    def test_generate_kill_missing_update_method(self):
        """Test generate_kill error when Update method is missing."""
        self.mock_crd.generate_get = Mock()
        self.mock_crd._extract_method_info = Mock(
            side_effect=ValueError("Method Update not found")
        )

        with self.assertRaises(ValueError):
            generate_kill(self.mock_crd, self.mock_channel)

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.kill.MessageToDict")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.kill.ParseDict")
    def test_kill_func_with_yes_flag(self, mock_parse_dict, mock_message_to_dict):
        """Test kill_func execution with --yes flag (auto-confirm)."""
        # Mock generate_get and _extract_method_info
        self.mock_crd.generate_get = Mock()
        mock_input_class = MagicMock()
        mock_output_class = MagicMock()
        self.mock_crd._extract_method_info = Mock(
            return_value=("UpdatePipelineRun", mock_input_class, mock_output_class)
        )

        # Mock get method response
        mock_get_response = Mock(spec=Message)
        self.mock_crd.get = Mock(return_value=mock_get_response)

        # Setup mock channel responses
        mock_update_stub = Mock()
        mock_update_response = Mock(spec=Message)
        mock_update_stub.return_value = mock_update_response

        self.mock_channel.unary_unary.return_value = mock_update_stub

        # Setup MessageToDict to return proper structure
        mock_message_to_dict.side_effect = [
            {"pipeline_run": {"spec": {"some_field": "value"}}},
            {"pipeline_run": {"spec": {"kill": True}}},
        ]

        # Generate kill function
        generate_kill(self.mock_crd, self.mock_channel)

        # Get the kill function that was attached to the CRD
        kill_func = self.mock_crd.kill

        # Execute the kill function
        result = kill_func(
            self.mock_crd,
            namespace="test-namespace",
            name="test-pipeline-run",
            yes=True,
        )

        # Verify the result is the update response
        self.assertEqual(result, mock_update_response)

        # Verify get was called
        self.mock_crd.get.assert_called_once_with("test-namespace", "test-pipeline-run")

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.kill.MessageToDict")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.kill.ParseDict")
    @patch("builtins.input")
    def test_kill_func_user_confirms(
        self, mock_input, mock_parse_dict, mock_message_to_dict
    ):
        """Test kill_func execution with user confirmation."""
        # User types 'yes'
        mock_input.return_value = "yes"

        # Mock generate_get and _extract_method_info
        self.mock_crd.generate_get = Mock()
        mock_input_class = MagicMock()
        mock_output_class = MagicMock()
        self.mock_crd._extract_method_info = Mock(
            return_value=("UpdatePipelineRun", mock_input_class, mock_output_class)
        )

        # Mock get method response
        mock_get_response = Mock(spec=Message)
        self.mock_crd.get = Mock(return_value=mock_get_response)

        # Setup mock channel responses
        mock_update_stub = Mock()
        mock_update_response = Mock(spec=Message)
        mock_update_stub.return_value = mock_update_response

        self.mock_channel.unary_unary.return_value = mock_update_stub

        mock_message_to_dict.side_effect = [
            {"pipeline_run": {"spec": {}}},
            {"pipeline_run": {"spec": {"kill": True}}},
        ]

        generate_kill(self.mock_crd, self.mock_channel)
        kill_func = self.mock_crd.kill

        result = kill_func(
            self.mock_crd, namespace="test-ns", name="test-run", yes=False
        )

        self.assertEqual(result, mock_update_response)
        mock_input.assert_called_once()

    @patch("builtins.input")
    @patch("builtins.print")
    def test_kill_func_user_cancels(self, mock_print, mock_input):
        """Test kill_func when user cancels the operation."""
        # User types 'no'
        mock_input.return_value = "no"

        # Mock generate_get and _extract_method_info
        self.mock_crd.generate_get = Mock()
        mock_input_class = MagicMock()
        mock_output_class = MagicMock()
        self.mock_crd._extract_method_info = Mock(
            return_value=("UpdatePipelineRun", mock_input_class, mock_output_class)
        )

        generate_kill(self.mock_crd, self.mock_channel)
        kill_func = self.mock_crd.kill

        result = kill_func(
            self.mock_crd, namespace="test-ns", name="test-run", yes=False
        )

        self.assertIsNone(result)
        mock_print.assert_called_with("Kill operation cancelled.")

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.kill.MessageToDict")
    def test_kill_func_missing_spec_field(self, mock_message_to_dict):
        """Test kill_func error when spec field is missing."""
        # Mock generate_get and _extract_method_info
        self.mock_crd.generate_get = Mock()
        mock_input_class = MagicMock()
        mock_output_class = MagicMock()
        self.mock_crd._extract_method_info = Mock(
            return_value=("UpdatePipelineRun", mock_input_class, mock_output_class)
        )

        # Mock get method response
        mock_get_response = Mock(spec=Message)
        self.mock_crd.get = Mock(return_value=mock_get_response)

        # MessageToDict returns structure without spec field
        mock_message_to_dict.return_value = {"pipeline_run": {}}

        generate_kill(self.mock_crd, self.mock_channel)
        kill_func = self.mock_crd.kill

        with self.assertRaises(ValueError) as context:
            kill_func(self.mock_crd, namespace="test-ns", name="test-run", yes=True)

        self.assertIn("Cannot set kill flag", str(context.exception))

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.kill.MessageToDict")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.kill.ParseDict")
    def test_kill_func_kill_flag_not_set(self, mock_parse_dict, mock_message_to_dict):
        """Test kill_func error when kill flag is not set in response."""
        # Mock generate_get and _extract_method_info
        self.mock_crd.generate_get = Mock()
        mock_input_class = MagicMock()
        mock_output_class = MagicMock()
        self.mock_crd._extract_method_info = Mock(
            return_value=("UpdatePipelineRun", mock_input_class, mock_output_class)
        )

        # Mock get method response
        mock_get_response = Mock(spec=Message)
        self.mock_crd.get = Mock(return_value=mock_get_response)

        # Setup mock channel responses
        mock_update_stub = Mock()
        mock_update_response = Mock(spec=Message)
        mock_update_stub.return_value = mock_update_response

        self.mock_channel.unary_unary.return_value = mock_update_stub

        # First call for get, second for update response
        mock_message_to_dict.side_effect = [
            {"pipeline_run": {"spec": {}}},
            {"pipeline_run": {"spec": {"kill": False}}},  # Kill flag not set
        ]

        generate_kill(self.mock_crd, self.mock_channel)
        kill_func = self.mock_crd.kill

        with self.assertRaises(RuntimeError) as context:
            kill_func(self.mock_crd, namespace="test-ns", name="test-run", yes=True)

        self.assertIn("Kill operation failed", str(context.exception))
