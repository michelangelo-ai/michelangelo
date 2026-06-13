"""Unit tests for pipeline delete command.

Tests the pipeline-specific delete command which always sets foreground
propagation so deleting a Pipeline always terminates and removes its child
runs. The assertions use the real ``DeletePipelineRequest`` proto so they
exercise the actual message field.
"""

from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from michelangelo.cli.mactl.crd import CRD
from michelangelo.cli.mactl.plugins.entity.pipeline.delete import (
    add_function_signature,
    generate_delete,
)
from michelangelo.gen.api.v2 import pipeline_svc_pb2


class PipelineDeleteTest(TestCase):
    """Tests for pipeline delete command."""

    def setUp(self):
        """Set up test fixtures."""
        self.mock_crd = Mock(spec=CRD)
        self.mock_crd.name = "pipeline"
        self.mock_crd.full_name = "michelangelo.api.v2.PipelineService"
        self.mock_crd.metadata = {}
        self.mock_crd.func_signature = {}

        mock_signature = Mock()

        def mock_bind(*args, **kwargs):
            bound = Mock()
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

    def _use_real_delete_proto(self):
        """Wire _extract_method_info to return the real Delete proto classes."""
        self.mock_crd._extract_method_info = Mock(
            return_value=(
                "DeletePipeline",
                pipeline_svc_pb2.DeletePipelineRequest,
                pipeline_svc_pb2.DeletePipelineResponse,
            )
        )

    def _capture_request_stub(self):
        """Set up a channel stub that captures the request it is called with."""
        captured = {}

        def fake_stub(request_input, **kwargs):
            captured["request"] = request_input
            captured["kwargs"] = kwargs
            return pipeline_svc_pb2.DeletePipelineResponse()

        self.mock_channel.unary_unary.return_value = Mock(side_effect=fake_stub)
        return captured

    def test_add_function_signature(self):
        """Test that add_function_signature registers delete with flags."""
        add_function_signature(self.mock_crd)

        self.assertIn("delete", self.mock_crd.func_signature)
        flags = {
            flag
            for arg in self.mock_crd.func_signature["delete"]["args"]
            for flag in arg.get("args", [])
        }
        self.assertIn("--namespace", flags)
        self.assertIn("--name", flags)
        self.assertIn("--yes", flags)

    def test_generate_delete_basic(self):
        """Test basic delete command generation extracts the Delete method."""
        self.mock_crd._extract_method_info = Mock(
            return_value=("DeletePipeline", Mock(), Mock())
        )

        generate_delete(self.mock_crd, self.mock_channel)

        self.mock_crd._extract_method_info.assert_called_once_with(
            self.mock_channel, self.mock_crd.full_name, "Delete"
        )

    def test_generate_delete_missing_delete_method(self):
        """Test generate_delete error when Delete method is missing."""
        self.mock_crd._extract_method_info = Mock(
            side_effect=ValueError("Method Delete not found")
        )

        with self.assertRaises(ValueError):
            generate_delete(self.mock_crd, self.mock_channel)

    def test_delete_always_sets_foreground(self):
        """Delete always sets delete_options.propagationPolicy == 'Foreground'."""
        self._use_real_delete_proto()
        captured = self._capture_request_stub()

        generate_delete(self.mock_crd, self.mock_channel)
        delete_func = self.mock_crd.delete

        result = delete_func(
            self.mock_crd,
            namespace="test-ns",
            name="test-pipeline",
            yes=True,
        )

        request = captured["request"]
        self.assertTrue(request.HasField("delete_options"))
        self.assertEqual(request.delete_options.propagationPolicy, "Foreground")
        self.assertEqual(request.name, "test-pipeline")
        self.assertEqual(request.namespace, "test-ns")
        self.assertIsInstance(result, pipeline_svc_pb2.DeletePipelineResponse)

    @patch("builtins.input")
    @patch("builtins.print")
    def test_delete_func_user_cancels(self, mock_print, mock_input):
        """Declining the prompt makes no gRPC call and returns None."""
        mock_input.return_value = "n"
        self.mock_crd._extract_method_info = Mock(
            return_value=("DeletePipeline", MagicMock(), MagicMock())
        )

        generate_delete(self.mock_crd, self.mock_channel)
        delete_func = self.mock_crd.delete

        result = delete_func(
            self.mock_crd,
            namespace="test-ns",
            name="test-pipeline",
            yes=False,
        )

        self.assertIsNone(result)
        self.mock_channel.unary_unary.assert_not_called()
        mock_print.assert_called_with("Delete operation cancelled.")

    def test_delete_func_yes_flag_skips_prompt(self):
        """--yes skips the prompt and proceeds with the delete."""
        self._use_real_delete_proto()
        self._capture_request_stub()

        generate_delete(self.mock_crd, self.mock_channel)
        delete_func = self.mock_crd.delete

        with patch("builtins.input") as mock_input:
            result = delete_func(
                self.mock_crd,
                namespace="test-ns",
                name="test-pipeline",
                yes=True,
            )

        mock_input.assert_not_called()
        self.mock_channel.unary_unary.assert_called_once()
        self.assertIsInstance(result, pipeline_svc_pb2.DeletePipelineResponse)

    @patch("builtins.input")
    def test_delete_func_user_confirms(self, mock_input):
        """Confirming the prompt ('yes') proceeds with the delete."""
        mock_input.return_value = "yes"
        self._use_real_delete_proto()
        self._capture_request_stub()

        generate_delete(self.mock_crd, self.mock_channel)
        delete_func = self.mock_crd.delete

        result = delete_func(
            self.mock_crd,
            namespace="test-ns",
            name="test-pipeline",
            yes=False,
        )

        mock_input.assert_called_once()
        self.assertIsInstance(result, pipeline_svc_pb2.DeletePipelineResponse)

    @patch("builtins.input")
    @patch("builtins.print")
    def test_delete_prompt_warns_unconditionally(self, mock_print, mock_input):
        """The confirm prompt states deletion of child runs unconditionally."""
        mock_input.return_value = "n"
        self.mock_crd._extract_method_info = Mock(
            return_value=("DeletePipeline", MagicMock(), MagicMock())
        )

        generate_delete(self.mock_crd, self.mock_channel)
        delete_func = self.mock_crd.delete

        delete_func(
            self.mock_crd,
            namespace="test-ns",
            name="test-pipeline",
            yes=False,
        )

        warning = mock_print.call_args_list[0][0][0]
        # The warning must state the cascade unconditionally ("will ...").
        self.assertIn("will terminate and delete", warning)
        self.assertIn("child runs", warning)
        self.assertIn("PipelineRuns", warning)
        self.assertIn("TriggerRuns", warning)
        # No conditional hedge: deletion is always cascaded now.
        lowered = warning.lower()
        self.assertNotIn("if cascade delete is enabled", lowered)
        self.assertNotIn("if cascade", lowered)
        self.assertNotIn("enabled on the cluster", lowered)
        prompt = mock_input.call_args[0][0]
        self.assertIn("cannot be undone", prompt)
