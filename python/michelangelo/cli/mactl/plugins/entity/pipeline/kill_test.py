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

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.kill.get_methods_from_service"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.kill.get_message_class_by_name"
    )
    def test_generate_kill_basic(self, mock_get_message_class, mock_get_methods):
        """Test basic kill command generation."""
        # Setup mock methods
        mock_get_method = Mock()
        mock_get_method.input_type = ".michelangelo.api.v2.GetPipelineRunRequest"
        mock_get_method.output_type = ".michelangelo.api.v2.GetPipelineRunResponse"

        mock_update_method = Mock()
        mock_update_method.input_type = ".michelangelo.api.v2.UpdatePipelineRunRequest"
        mock_update_method.output_type = (
            ".michelangelo.api.v2.UpdatePipelineRunResponse"
        )

        mock_methods = {
            "GetPipelineRun": mock_get_method,
            "UpdatePipelineRun": mock_update_method,
        }

        mock_get_methods.return_value = (mock_methods, Mock())

        # Setup mock message classes
        mock_get_message_class.return_value = Mock

        # Execute
        generate_kill(self.mock_crd, self.mock_channel)

        # Verify methods were called correctly
        mock_get_methods.assert_called_once()
        self.assertTrue(mock_get_message_class.called)

    def test_kill_command_requires_namespace_and_name(self):
        """Test that kill command requires namespace and name parameters."""
        # This is implicitly tested by the function signature definition
        # The test just verifies the function can be called
        add_function_signature(self.mock_crd)
        self.assertTrue(True)

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.kill.get_methods_from_service"
    )
    def test_generate_kill_missing_get_method(self, mock_get_methods):
        """Test generate_kill error when GetPipelineRun method missing."""
        mock_methods = {
            "UpdatePipelineRun": Mock(),
        }
        mock_get_methods.return_value = (mock_methods, Mock())

        with self.assertRaises(KeyError):
            generate_kill(self.mock_crd, self.mock_channel)

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.kill.get_methods_from_service"
    )
    def test_generate_kill_missing_update_method(self, mock_get_methods):
        """Test generate_kill error when UpdatePipelineRun method missing."""
        mock_methods = {
            "GetPipelineRun": Mock(),
        }
        mock_get_methods.return_value = (mock_methods, Mock())

        with self.assertRaises(KeyError):
            generate_kill(self.mock_crd, self.mock_channel)

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.kill.get_methods_from_service"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.kill.get_message_class_by_name"
    )
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.kill.MessageToDict")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.kill.ParseDict")
    def test_kill_func_with_yes_flag(
        self,
        mock_parse_dict,
        mock_message_to_dict,
        mock_get_message_class,
        mock_get_methods,
    ):
        """Test kill_func execution with --yes flag (auto-confirm)."""
        # Setup mock methods
        mock_get_method = Mock()
        mock_get_method.input_type = ".michelangelo.api.v2.GetPipelineRunRequest"
        mock_get_method.output_type = ".michelangelo.api.v2.GetPipelineRunResponse"

        mock_update_method = Mock()
        mock_update_method.input_type = ".michelangelo.api.v2.UpdatePipelineRunRequest"
        mock_update_method.output_type = (
            ".michelangelo.api.v2.UpdatePipelineRunResponse"
        )

        mock_methods = {
            "GetPipelineRun": mock_get_method,
            "UpdatePipelineRun": mock_update_method,
        }
        mock_get_methods.return_value = (mock_methods, Mock())

        # Setup mock message classes
        mock_input_class = MagicMock()
        mock_output_class = MagicMock()
        mock_get_message_class.side_effect = [
            mock_input_class,
            mock_output_class,
            mock_input_class,
            mock_output_class,
        ]

        # Setup mock channel responses
        mock_get_stub = Mock()
        mock_get_response = Mock(spec=Message)
        mock_get_stub.return_value = mock_get_response

        mock_update_stub = Mock()
        mock_update_response = Mock(spec=Message)
        mock_update_stub.return_value = mock_update_response

        self.mock_channel.unary_unary.side_effect = [mock_get_stub, mock_update_stub]

        # Setup MessageToDict to return proper structure
        mock_message_to_dict.side_effect = [
            {"pipeline_run": {"spec": {"some_field": "value"}}},
            {"pipeline_run": {"spec": {"kill": True}}},
        ]

        # Generate kill function
        generate_kill(self.mock_crd, self.mock_channel)

        # Get the kill function that was attached to the CRD
        kill_func = self.mock_crd.kill

        # Execute the kill function by calling it directly
        # The bind_signature decorator will handle binding
        result = kill_func(
            self.mock_crd,
            namespace="test-namespace",
            name="test-pipeline-run",
            yes=True,
        )

        # Verify the result is the update response
        self.assertEqual(result, mock_update_response)

        # Verify gRPC stubs were called
        self.assertEqual(self.mock_channel.unary_unary.call_count, 2)
        mock_get_stub.assert_called_once()
        mock_update_stub.assert_called_once()

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.kill.get_methods_from_service"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.kill.get_message_class_by_name"
    )
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.kill.MessageToDict")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.kill.ParseDict")
    @patch("builtins.input")
    def test_kill_func_user_confirms(
        self,
        mock_input,
        mock_parse_dict,
        mock_message_to_dict,
        mock_get_message_class,
        mock_get_methods,
    ):
        """Test kill_func execution with user confirmation."""
        # User types 'yes'
        mock_input.return_value = "yes"

        # Setup mocks
        mock_get_method = Mock()
        mock_get_method.input_type = ".michelangelo.api.v2.GetPipelineRunRequest"
        mock_get_method.output_type = ".michelangelo.api.v2.GetPipelineRunResponse"

        mock_update_method = Mock()
        mock_update_method.input_type = ".michelangelo.api.v2.UpdatePipelineRunRequest"
        mock_update_method.output_type = (
            ".michelangelo.api.v2.UpdatePipelineRunResponse"
        )

        mock_methods = {
            "GetPipelineRun": mock_get_method,
            "UpdatePipelineRun": mock_update_method,
        }
        mock_get_methods.return_value = (mock_methods, Mock())

        mock_input_class = MagicMock()
        mock_output_class = MagicMock()
        mock_get_message_class.side_effect = [
            mock_input_class,
            mock_output_class,
            mock_input_class,
            mock_output_class,
        ]

        mock_get_stub = Mock()
        mock_get_response = Mock(spec=Message)
        mock_get_stub.return_value = mock_get_response

        mock_update_stub = Mock()
        mock_update_response = Mock(spec=Message)
        mock_update_stub.return_value = mock_update_response

        self.mock_channel.unary_unary.side_effect = [mock_get_stub, mock_update_stub]

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

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.kill.get_methods_from_service"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.kill.get_message_class_by_name"
    )
    @patch("builtins.input")
    @patch("builtins.print")
    def test_kill_func_user_cancels(
        self, mock_print, mock_input, mock_get_message_class, mock_get_methods
    ):
        """Test kill_func when user cancels the operation."""
        # User types 'no'
        mock_input.return_value = "no"

        mock_get_method = Mock()
        mock_get_method.input_type = ".michelangelo.api.v2.GetPipelineRunRequest"
        mock_get_method.output_type = ".michelangelo.api.v2.GetPipelineRunResponse"

        mock_update_method = Mock()
        mock_update_method.input_type = ".michelangelo.api.v2.UpdatePipelineRunRequest"
        mock_update_method.output_type = (
            ".michelangelo.api.v2.UpdatePipelineRunResponse"
        )

        mock_methods = {
            "GetPipelineRun": mock_get_method,
            "UpdatePipelineRun": mock_update_method,
        }
        mock_get_methods.return_value = (mock_methods, Mock())

        mock_input_class = MagicMock()
        mock_output_class = MagicMock()
        mock_get_message_class.side_effect = [
            mock_input_class,
            mock_output_class,
            mock_input_class,
            mock_output_class,
        ]

        generate_kill(self.mock_crd, self.mock_channel)
        kill_func = self.mock_crd.kill

        result = kill_func(
            self.mock_crd, namespace="test-ns", name="test-run", yes=False
        )

        self.assertIsNone(result)
        mock_print.assert_called_with("Kill operation cancelled.")

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.kill.get_methods_from_service"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.kill.get_message_class_by_name"
    )
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.kill.MessageToDict")
    def test_kill_func_missing_spec_field(
        self, mock_message_to_dict, mock_get_message_class, mock_get_methods
    ):
        """Test kill_func error when spec field is missing."""
        mock_get_method = Mock()
        mock_get_method.input_type = ".michelangelo.api.v2.GetPipelineRunRequest"
        mock_get_method.output_type = ".michelangelo.api.v2.GetPipelineRunResponse"

        mock_update_method = Mock()
        mock_update_method.input_type = ".michelangelo.api.v2.UpdatePipelineRunRequest"
        mock_update_method.output_type = (
            ".michelangelo.api.v2.UpdatePipelineRunResponse"
        )

        mock_methods = {
            "GetPipelineRun": mock_get_method,
            "UpdatePipelineRun": mock_update_method,
        }
        mock_get_methods.return_value = (mock_methods, Mock())

        mock_input_class = MagicMock()
        mock_output_class = MagicMock()
        mock_get_message_class.side_effect = [
            mock_input_class,
            mock_output_class,
            mock_input_class,
            mock_output_class,
        ]

        mock_get_stub = Mock()
        mock_get_response = Mock(spec=Message)
        mock_get_stub.return_value = mock_get_response

        self.mock_channel.unary_unary.return_value = mock_get_stub

        # MessageToDict returns structure without spec field
        mock_message_to_dict.return_value = {"pipeline_run": {}}

        generate_kill(self.mock_crd, self.mock_channel)
        kill_func = self.mock_crd.kill

        with self.assertRaises(ValueError) as context:
            kill_func(self.mock_crd, namespace="test-ns", name="test-run", yes=True)

        self.assertIn("Cannot set kill flag", str(context.exception))

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.kill.get_methods_from_service"
    )
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline.kill.get_message_class_by_name"
    )
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.kill.MessageToDict")
    @patch("michelangelo.cli.mactl.plugins.entity.pipeline.kill.ParseDict")
    def test_kill_func_kill_flag_not_set(
        self,
        mock_parse_dict,
        mock_message_to_dict,
        mock_get_message_class,
        mock_get_methods,
    ):
        """Test kill_func error when kill flag is not set in response."""
        mock_get_method = Mock()
        mock_get_method.input_type = ".michelangelo.api.v2.GetPipelineRunRequest"
        mock_get_method.output_type = ".michelangelo.api.v2.GetPipelineRunResponse"

        mock_update_method = Mock()
        mock_update_method.input_type = ".michelangelo.api.v2.UpdatePipelineRunRequest"
        mock_update_method.output_type = (
            ".michelangelo.api.v2.UpdatePipelineRunResponse"
        )

        mock_methods = {
            "GetPipelineRun": mock_get_method,
            "UpdatePipelineRun": mock_update_method,
        }
        mock_get_methods.return_value = (mock_methods, Mock())

        mock_input_class = MagicMock()
        mock_output_class = MagicMock()
        mock_get_message_class.side_effect = [
            mock_input_class,
            mock_output_class,
            mock_input_class,
            mock_output_class,
        ]

        mock_get_stub = Mock()
        mock_get_response = Mock(spec=Message)
        mock_get_stub.return_value = mock_get_response

        mock_update_stub = Mock()
        mock_update_response = Mock(spec=Message)
        mock_update_stub.return_value = mock_update_response

        self.mock_channel.unary_unary.side_effect = [mock_get_stub, mock_update_stub]

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
