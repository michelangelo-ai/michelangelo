"""Tests for import parsing."""

import ast
import importlib
from unittest import TestCase

from michelangelo.lib.model_manager._private.utils.module_finder import get_imports
from michelangelo.lib.model_manager._private.utils.module_finder.import_parser import (
    get_node_module,
)


class ImportParserTest(TestCase):
    """Tests import parsing utilities."""

    def test_get_imports(self):
        """It returns absolute imports for modules with standard imports."""
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
        """It handles modules that use relative imports."""
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
        """It resolves imports nested under deeper relative paths."""
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
        """It collects imports when folders are treated as packages."""
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
        """It returns None when an import node lacks a module."""
        node = ast.ImportFrom(module=None)
        self.assertIsNone(
            get_node_module(
                node,
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports",
            ),
        )
