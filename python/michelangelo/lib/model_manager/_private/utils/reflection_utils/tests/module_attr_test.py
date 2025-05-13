import os
import sys
import tempfile
import shutil
from unittest import TestCase
from unittest.mock import patch
from michelangelo.lib.model_manager._private.utils.reflection_utils import (
    get_module_attr,
    find_attr_from_sys_modules,
    find_attr_from_dir,
)

# enable metabuild to build bazel dependencies
import michelangelo.lib.model_manager._private.utils.reflection_utils.tests.fixtures.simple_module
import michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.simple_module  # noqa:F401


class ModuleAttrTest(TestCase):
    def setUp(self):
        self.sys_path = sys.path.copy()

    def tearDown(self):
        sys.path = self.sys_path

    def test_get_module_attr(self):
        module_attr = get_module_attr("michelangelo.lib.model_manager._private.utils.reflection_utils.tests.fixtures.simple_module.module_attr")

        self.assertIsNotNone(module_attr)
        self.assertEqual(module_attr.__name__, "module_attr")

    def test_find_attr_from_sys_modules(self):
        attributes = find_attr_from_sys_modules("module_attr")
        attr_names = sorted([attr.__name__ for attr in attributes])

        self.assertEqual(attr_names, ["module_attr"] * 2 + ["michelangelo.lib.model_manager._private.utils.reflection_utils.module_attr"])

    def test_find_attr_from_dir(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            path = "uber/ai/michelangelo/sdk/model_manager/_private/utils/reflection_utils/tests/fixtures"
            defs_path = os.path.join(temp_dir, path)

            shutil.copytree(path, defs_path)
            attributes = find_attr_from_dir("module_attr", temp_dir)

            self.assertEqual([attr.__name__ for attr in attributes], ["module_attr"])

    @patch("importlib.import_module", side_effect=ImportError)
    def test_find_attr_from_dir_import_error(self, mock_import_module):
        with tempfile.TemporaryDirectory() as temp_dir:
            path = "uber/ai/michelangelo/sdk/model_manager/_private/utils/reflection_utils/tests/fixtures"
            defs_path = os.path.join(temp_dir, path)

            shutil.copytree(path, defs_path)
            attributes = find_attr_from_dir("module_attr", temp_dir)

            self.assertEqual(attributes, [])
            mock_import_module.assert_called()
