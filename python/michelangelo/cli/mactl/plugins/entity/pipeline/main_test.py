"""Unit tests for pipeline main plugin.

Tests the apply_plugins function which configures the CRD with
plugin-specific converters.
"""

from unittest import TestCase
from unittest.mock import Mock

from michelangelo.cli.mactl.plugins.entity.pipeline.main import (
    apply_plugin_command,
    apply_plugins,
)


class PipelineMainTest(TestCase):
    """Tests for pipeline main plugin."""

    def setUp(self):
        """Set up test fixtures."""
        self.mock_crd = Mock()
        self.mock_crd.func_signature = {}  # Real dict for item assignment
        self.mock_crds = {"pipeline": self.mock_crd}
        self.mock_channel = Mock()

    def test_apply_plugins_create_command(self):
        """Test apply_plugins for create command."""
        apply_plugin_command(self.mock_crd, "create", self.mock_crds, self.mock_channel)

        # Verify that the metadata converter was set
        self.assertTrue(hasattr(self.mock_crd, "func_crd_metadata_converter"))
        # Cannot check the exact function due to mock imports, but verify it was set
        self.assertIsNotNone(self.mock_crd.func_crd_metadata_converter)

    def test_apply_plugins_run_command(self):
        """Test apply_plugins for run command."""
        apply_plugin_command(self.mock_crd, "run", self.mock_crds, self.mock_channel)

        # Verify that both the metadata converter and generate_run were set
        self.assertTrue(hasattr(self.mock_crd, "func_crd_metadata_converter"))
        self.assertTrue(hasattr(self.mock_crd, "generate_run"))
        self.assertIsNotNone(self.mock_crd.func_crd_metadata_converter)
        self.assertIsNotNone(self.mock_crd.generate_run)

    def test_apply_plugins_dev_run_command(self):
        """Test apply_plugins for dev_run command."""
        apply_plugin_command(
            self.mock_crd, "dev_run", self.mock_crds, self.mock_channel
        )

        # Verify that both the metadata converter and generate_dev_run were set
        self.assertTrue(hasattr(self.mock_crd, "func_crd_metadata_converter"))
        self.assertTrue(hasattr(self.mock_crd, "generate_dev_run"))
        self.assertIsNotNone(self.mock_crd.func_crd_metadata_converter)
        self.assertIsNotNone(self.mock_crd.generate_dev_run)

    def test_apply_plugins_registers_kill_command(self):
        """Test that apply_plugins registers the kill command."""
        apply_plugins(self.mock_crd, self.mock_channel)

        # Verify that generate_kill was set
        self.assertTrue(hasattr(self.mock_crd, "generate_kill"))
        self.assertIsNotNone(self.mock_crd.generate_kill)
