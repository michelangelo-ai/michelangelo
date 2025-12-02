import importlib
import ast
from unittest import TestCase
from michelangelo.lib.model_manager._private.utils.module_finder import get_imports
from michelangelo.lib.model_manager._private.utils.module_finder.import_parser import (
    get_node_module,
)


class ImportParserTest(TestCase):
    def test_get_imports(self):
        module = importlib.import_module(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports"
        )
        imports = get_imports(module)

        self.assertEqual(
            imports,
            [
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.simple_module",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.folder.fn1",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.folder.fn2",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.package",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.folder.fn3",
            ],
        )

    def test_get_imports_with_relative_imports(self):
        module = importlib.import_module(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_relative_imports"
        )
        imports = get_imports(module)

        self.assertEqual(
            imports,
            [
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.simple_module",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.folder.fn1",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.folder.fn2",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.package",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.folder.fn3",
            ],
        )

    def test_get_imports_with_deeper_relative_imports(self):
        module = importlib.import_module(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.nested.module_with_deeper_relative_imports",
        )
        imports = get_imports(module)

        self.assertEqual(
            imports,
            [
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.simple_module",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.folder.fn1",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.folder.fn2",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.package",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.folder.fn3",
            ],
        )

    def test_get_imports_with_folder_as_package(self):
        module = importlib.import_module(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports_package_without_init",
        )
        imports = get_imports(module)

        self.assertEqual(
            imports,
            [
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.simple_module",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.package",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.folder",
            ],
        )

    def test_get_node_module_with_no_module(self):
        node = ast.ImportFrom(module=None)
        self.assertIsNone(
            get_node_module(
                node,
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports",
            ),
        )
