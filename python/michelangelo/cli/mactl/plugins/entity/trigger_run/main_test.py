"""Unit tests for trigger_run plugin main module."""

from types import MethodType
from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

from michelangelo.cli.mactl.plugins.entity.trigger_run.main import apply_plugins


class ApplyPluginsKillTest(TestCase):
    """Tests for apply_plugins function."""

    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.main.generate_create")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.main"
        ".add_create_function_signature"
    )
    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.main.generate_kill")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.main.add_function_signature"
    )
    def test_apply_plugins_adds_function_signature(
        self, mock_add_function_signature, _, __, ___
    ):
        """Test that apply_plugins calls add_function_signature with CRD."""
        # Setup
        mock_crd = Mock()
        mock_channel = Mock()

        # Execute
        apply_plugins(mock_crd, mock_channel)

        # Verify
        mock_add_function_signature.assert_called_once_with(mock_crd)

    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.main.generate_create")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.main"
        ".add_create_function_signature"
    )
    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.main.generate_kill")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.main.add_function_signature"
    )
    def test_apply_plugins_adds_generate_kill_method(self, _, __, ___, ____):
        """Test that apply_plugins adds generate_kill method to CRD."""
        # Setup
        mock_crd = Mock()
        mock_channel = Mock()

        # Execute
        apply_plugins(mock_crd, mock_channel)

        # Verify that generate_kill method was added to crd
        self.assertTrue(hasattr(mock_crd, "generate_kill"))
        self.assertIsInstance(mock_crd.generate_kill, MethodType)

    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.main.generate_create")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.main"
        ".add_create_function_signature"
    )
    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.main.generate_kill")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.main.add_function_signature"
    )
    def test_apply_plugins_generate_kill_calls_correct_function(
        self, _, mock_generate_kill, __, ___
    ):
        """Test that the added generate_kill method calls generate_kill function."""
        # Setup
        mock_crd = Mock()
        mock_channel = Mock()
        mock_parser = Mock()

        # Execute
        apply_plugins(mock_crd, mock_channel)

        # Call the added generate_kill method
        mock_crd.generate_kill(mock_channel, mock_parser)

        # Verify that generate_kill was called with correct arguments
        mock_generate_kill.assert_called_once_with(mock_crd, mock_channel, mock_parser)

    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.main.generate_kill")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.main.add_function_signature"
    )
    def test_apply_plugins_with_real_crd_object(self, mock_add_function_signature, _):
        """Test apply_plugins with a more realistic CRD object."""
        # Setup - Create a mock CRD with some attributes
        mock_crd = MagicMock()
        mock_crd.name = "trigger_run"
        mock_crd.full_name = "michelangelo.TriggerRun"
        mock_channel = Mock()

        # Execute
        apply_plugins(mock_crd, mock_channel)

        # Verify
        mock_add_function_signature.assert_called_once_with(mock_crd)
        self.assertTrue(hasattr(mock_crd, "generate_kill"))

    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.main.generate_kill")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.main.add_function_signature"
    )
    def test_apply_plugins_preserves_existing_crd_attributes(self, _, __):
        """Test that apply_plugins doesn't modify existing CRD attributes."""
        # Setup
        mock_crd = MagicMock()
        mock_crd.existing_method = Mock(return_value="test")
        mock_crd.existing_attr = "test_value"
        mock_channel = Mock()

        # Execute
        apply_plugins(mock_crd, mock_channel)

        # Verify existing attributes are preserved
        self.assertEqual(mock_crd.existing_attr, "test_value")
        self.assertEqual(mock_crd.existing_method(), "test")


class ApplyPluginsCreateTest(TestCase):
    """Tests for apply_plugins create-related functionality."""

    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.main.generate_create")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.main"
        ".add_create_function_signature"
    )
    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.main.generate_kill")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.main.add_function_signature"
    )
    def test_apply_plugins_calls_add_create_function_signature(
        self, _, __, mock_add_create_sig, ___
    ):
        """Test that apply_plugins calls add_create_function_signature."""
        mock_crd = Mock()
        mock_channel = Mock()

        apply_plugins(mock_crd, mock_channel)

        mock_add_create_sig.assert_called_once_with(mock_crd)

    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.main.generate_create")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.main"
        ".add_create_function_signature"
    )
    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.main.generate_kill")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.main.add_function_signature"
    )
    def test_apply_plugins_adds_generate_create_method(self, _, __, ___, ____):
        """Test that apply_plugins adds generate_create method to CRD."""
        mock_crd = Mock()
        mock_channel = Mock()

        apply_plugins(mock_crd, mock_channel)

        self.assertTrue(hasattr(mock_crd, "generate_create"))
        self.assertIsInstance(mock_crd.generate_create, MethodType)

    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.main.generate_create")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.main"
        ".add_create_function_signature"
    )
    @patch("michelangelo.cli.mactl.plugins.entity.trigger_run.main.generate_kill")
    @patch(
        "michelangelo.cli.mactl.plugins.entity.trigger_run.main.add_function_signature"
    )
    def test_generate_create_method_delegates_correctly(
        self, _, __, ___, mock_generate_create
    ):
        """Test that the bound generate_create calls generate_create."""
        mock_crd = Mock()
        mock_channel = Mock()
        mock_parser = Mock()

        apply_plugins(mock_crd, mock_channel)

        mock_crd.generate_create(mock_channel, mock_parser)

        mock_generate_create.assert_called_once_with(
            mock_crd, mock_channel, mock_parser
        )
