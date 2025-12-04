"""Tests for the main module serialization."""

import os
import tempfile
from unittest import TestCase
from unittest.mock import patch
from michelangelo.lib.model_manager._private.packager.custom_triton import serialize_main_module


class MainModuleTest(TestCase):
    """Tests for the main module serialization."""

    def test_serialize_main_module(self):
        """Tests that the main module is serialized."""
        with tempfile.TemporaryDirectory() as target_dir:
            serialize_main_module(target_dir, include_import_prefixes=["michelangelo"])
            self.assertTrue(len(os.listdir(target_dir)) > 0)

    @patch("michelangelo.lib.model_manager._private.packager.custom_triton.main_module.inspect.getfile")
    def test_serilize_main_module_skip(self, mock_getfile):
        """Tests that the main module is not serialized if it is not a file."""
        mock_getfile.side_effect = TypeError
        with tempfile.TemporaryDirectory() as target_dir:
            serialize_main_module(target_dir)
            self.assertTrue(len(os.listdir(target_dir)) == 0)
