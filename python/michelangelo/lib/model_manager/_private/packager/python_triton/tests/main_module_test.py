import os
import tempfile
from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.sdk.model_manager._private.packager.python_triton import serialize_main_module


class MainModuleTest(TestCase):
    def test_serialize_main_module(self):
        with tempfile.TemporaryDirectory() as target_dir:
            serialize_main_module(target_dir, include_import_prefixes=["uber"])
            self.assertTrue(len(os.listdir(target_dir)) > 0)

    @patch("uber.ai.michelangelo.sdk.model_manager._private.packager.python_triton.main_module.inspect.getfile")
    def test_serilize_main_module_skip(self, mock_getfile):
        mock_getfile.side_effect = TypeError
        with tempfile.TemporaryDirectory() as target_dir:
            serialize_main_module(target_dir)
            self.assertTrue(len(os.listdir(target_dir)) == 0)
