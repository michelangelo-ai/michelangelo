"""Unit tests for pipeline_run entity plugin."""

from unittest import TestCase
from unittest.mock import Mock, patch

from michelangelo.cli.mactl.crd import CRD
from michelangelo.cli.mactl.plugins.entity.pipeline_run.main import apply_plugins


class PipelineRunPluginTest(TestCase):
    """Tests for pipeline_run entity plugin."""

    def setUp(self):
        """Set up test fixtures."""
        self.mock_crd = Mock(spec=CRD)
        self.mock_crd.name = "pipeline_run"
        self.mock_crd.full_name = "michelangelo.api.v2.PipelineRunService"
        self.mock_crd.metadata = {}
        self.mock_crd.func_signature = {}
        self.mock_channel = Mock()

    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline_run.main.add_kill_function_signature"
    )
    def test_apply_plugins_adds_kill_signature(self, mock_add_kill_sig):
        """Test that apply_plugins adds kill function signature."""
        apply_plugins(self.mock_crd, self.mock_channel)

        # Verify that add_kill_function_signature was called
        mock_add_kill_sig.assert_called_once_with(self.mock_crd)

    @patch("michelangelo.cli.mactl.plugins.entity.pipeline_run.main.generate_kill")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.pipeline_run.main.add_kill_function_signature"
    )
    def test_apply_plugins_adds_generate_kill(
        self, mock_add_kill_sig, mock_generate_kill
    ):
        """Test that apply_plugins sets up generate_kill method."""
        apply_plugins(self.mock_crd, self.mock_channel)

        # Verify that generate_kill method was attached to CRD
        self.assertTrue(hasattr(self.mock_crd, "generate_kill"))

    def test_apply_plugins_logs_correctly(self):
        """Test that apply_plugins logs expected messages."""
        with self.assertLogs(
            "michelangelo.cli.mactl.plugins.entity.pipeline_run.main", level="INFO"
        ) as log:
            apply_plugins(self.mock_crd, self.mock_channel)

            # Check that the correct log messages were generated
            self.assertTrue(
                any("Applying pipeline_run plugin" in message for message in log.output)
            )
            self.assertTrue(
                any(
                    "Plugin entities applied successfully" in message
                    for message in log.output
                )
            )
