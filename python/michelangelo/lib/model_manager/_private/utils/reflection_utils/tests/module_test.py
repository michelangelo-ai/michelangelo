import os
import shutil
import sys
import tempfile
from unittest import TestCase
from unittest.mock import patch

from michelangelo.lib.model_manager._private.utils.reflection_utils import (
    find_attr_from_dir,
    find_attr_from_sys_modules,
    get_module,
)


class ModuleTest(TestCase):
    """Tests reflection utilities for module discovery."""

    def test_get_module(self):
        """It resolves modules by fully qualified attribute path."""
        module = get_module(
            "michelangelo.lib.model_manager._private.utils.reflection_utils.tests.fixtures.simple_module.module_attr"
        )

        self.assertIsNotNone(module)
        self.assertEqual(
            module.__name__,
            "michelangelo.lib.model_manager._private.utils.reflection_utils.tests.fixtures.simple_module",
        )

    def test_find_attr_from_sys_modules(self):
        """It discovers attributes already loaded in sys.modules."""
        import michelangelo.lib.model_manager._private.utils.reflection_utils.module as module_module  # noqa: E501
        import michelangelo.lib.model_manager._private.utils.reflection_utils.tests.fixtures.simple_module as fixture_module  # noqa: E501

        patched_modules = {
            "sys": sys,
            "michelangelo.lib.model_manager._private.utils.reflection_utils.tests.fixtures.simple_module": fixture_module,  # noqa: E501
            "michelangelo.lib.model_manager._private.utils.reflection_utils.module": module_module,  # noqa: E501
        }

        with patch.dict("sys.modules", patched_modules, clear=True):
            attributes = find_attr_from_sys_modules("module_attr")
        attr_names = sorted([attr.__name__ for attr in attributes])

        self.assertEqual(attr_names, ["module_attr"])

    def test_find_attr_from_dir(self):
        """It discovers attributes by walking a directory tree."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = (
                "michelangelo/lib/model_manager/_private/utils/reflection_utils/"
                "tests/fixtures"
            )
            defs_path = os.path.join(temp_dir, path)

            shutil.copytree(path, defs_path)
            attributes = find_attr_from_dir("module_attr", temp_dir)

            self.assertEqual([attr.__name__ for attr in attributes], ["module_attr"])

    @patch("importlib.import_module", side_effect=ImportError)
    def test_find_attr_from_dir_import_error(self, mock_import_module):
        """It returns an empty list if imports fail while scanning directories."""
        with tempfile.TemporaryDirectory() as temp_dir:
            path = (
                "michelangelo/lib/model_manager/_private/utils/reflection_utils/"
                "tests/fixtures"
            )
            defs_path = os.path.join(temp_dir, path)

            shutil.copytree(path, defs_path)
            attributes = find_attr_from_dir("module_attr", temp_dir)

            self.assertEqual(attributes, [])
            mock_import_module.assert_called()
