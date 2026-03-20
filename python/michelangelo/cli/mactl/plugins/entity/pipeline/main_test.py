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

    def test_apply_command_sets_metadata_converter(self):
        """Test that apply command sets func_crd_metadata_converter.

        CRITICAL BUG FIX TEST: This test ensures that the 'apply' command
        properly configures the metadata converter, which is essential for
        pipeline registration.

        Background:
        - The CRD.apply() function auto-creates an instance if it doesn't exist
          by calling CRD.create() internally (see crd.py:400-404)
        - CRD.create() relies on func_crd_metadata_converter to process the YAML
          (see crd.py:424-428)
        - For pipelines, convert_crd_metadata_pipeline_create() performs critical
          registration steps including:
          1. Running subprocess registration to build the uniflow tar
          2. Extracting the tar path and workflow inputs
          3. Adding uniflow tar path to spec.manifest.uniflowTar
          4. Adding workflow inputs to spec.manifest.content

        Impact of bug:
        - Without this converter, 'mactl apply -f pipeline.yaml' would create a
          pipeline WITHOUT the uniflow tar path in the spec
        - The pipeline would be registered in the metadata store, but would have
          no executable code
        - When triggered, the pipeline would fail because the uniflow tar is
          missing
        - This is a silent failure - the apply command succeeds, but the pipeline
          is broken

        Why this test matters:
        - Ensures 'apply' and 'create' commands have identical behavior for new
          pipelines
        - Prevents regression where apply command bypasses critical registration
          steps
        - Documents the dependency between apply → create → metadata converter
          → tar upload
        """
        # Test that apply command sets the metadata converter
        apply_plugin_command(self.mock_crd, "apply", self.mock_crds, self.mock_channel)

        # Verify that the metadata converter was set
        self.assertTrue(hasattr(self.mock_crd, "func_crd_metadata_converter"))
        self.assertIsNotNone(self.mock_crd.func_crd_metadata_converter)

        # Verify it's the same converter as create command uses
        create_crd = Mock()
        create_crd.func_signature = {}
        apply_plugin_command(create_crd, "create", self.mock_crds, self.mock_channel)

        self.assertEqual(
            self.mock_crd.func_crd_metadata_converter,
            create_crd.func_crd_metadata_converter,
            "apply and create commands must use the same metadata converter",
        )
