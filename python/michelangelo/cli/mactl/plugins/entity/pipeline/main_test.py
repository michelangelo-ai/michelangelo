"""Unit tests for pipeline main plugin.

Tests the apply_plugins function which configures the CRD with
plugin-specific converters.
"""

from unittest import TestCase
from unittest.mock import Mock, patch

from michelangelo.cli.mactl.plugins.entity.pipeline.create import (
    convert_crd_metadata_pipeline,
)
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

    def test_apply_command_sets_converter(self):
        """Apply command sets func_crd_metadata_converter to convert_crd_metadata_pipeline."""  # noqa: E501
        apply_plugin_command(self.mock_crd, "apply", self.mock_crds, self.mock_channel)

        self.assertEqual(
            self.mock_crd.func_crd_metadata_converter,
            convert_crd_metadata_pipeline,
        )

    @patch("michelangelo.cli.mactl.crd.apply_func_impl")
    def test_apply_command_sets_apply_func_impl(self, _):
        """Apply command patches module-level apply_func_impl with pipeline impl."""
        import michelangelo.cli.mactl.crd as crd_module
        from michelangelo.cli.mactl.plugins.entity.pipeline.apply import (
            pipeline_apply_func_impl,
        )

        apply_plugin_command(self.mock_crd, "apply", self.mock_crds, self.mock_channel)

        self.assertEqual(crd_module.apply_func_impl, pipeline_apply_func_impl)

    def test_apply_plugins_run_command(self):
        """Test apply_plugins for run command."""
        apply_plugin_command(self.mock_crd, "run", self.mock_crds, self.mock_channel)

        self.assertTrue(hasattr(self.mock_crd, "func_crd_metadata_converter"))
        self.assertTrue(hasattr(self.mock_crd, "generate_run"))
        self.assertIsNotNone(self.mock_crd.func_crd_metadata_converter)
        self.assertIsNotNone(self.mock_crd.generate_run)

    def test_apply_plugins_dev_run_command(self):
        """Test apply_plugins for dev_run command."""
        apply_plugin_command(
            self.mock_crd, "dev_run", self.mock_crds, self.mock_channel
        )

        self.assertTrue(hasattr(self.mock_crd, "func_crd_metadata_converter"))
        self.assertTrue(hasattr(self.mock_crd, "generate_dev_run"))
        self.assertIsNotNone(self.mock_crd.func_crd_metadata_converter)
        self.assertIsNotNone(self.mock_crd.generate_dev_run)

    def test_apply_plugins_registers_kill_command(self):
        """Test that apply_plugins registers the kill command."""
        apply_plugins(self.mock_crd, self.mock_channel)

        self.assertTrue(hasattr(self.mock_crd, "generate_kill"))
        self.assertIsNotNone(self.mock_crd.generate_kill)
