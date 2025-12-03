import os
from unittest import TestCase

from michelangelo.lib.model_manager._private.utils.module_finder import (
    find_dependency_files,
)


class DependencyFilesTest(TestCase):
    """Tests dependency file finding utilities."""

    def test_find_imported_module_files(self):
        """It discovers imported files for a given module."""
        files = find_dependency_files(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports"
        )

        clean_paths = sorted(
            [os.path.join("", *f.split("/")[-2:]) for f in files],
        )

        self.assertEqual(
            clean_paths,
            [
                "fixtures/module_with_imports.py",
                "fixtures/simple_module.py",
                "folder/fn1.py",
                "folder/fn2.py",
                "folder/fn3.py",
                "folder/fn4.py",
                "package/__init__.py",
                "package/fn1.py",
                "package/fn2.py",
            ],
        )

    def test_find_imported_module_files_with_prefixes(self):
        """It filters discovered files by prefix."""
        files = find_dependency_files(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports",
            prefixes=[
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.folder",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.package",
            ],
        )

        clean_paths = sorted(
            [os.path.join("", *f.split("/")[-2:]) for f in files],
        )

        self.assertEqual(
            clean_paths,
            [
                "fixtures/module_with_imports.py",
                "folder/fn1.py",
                "folder/fn2.py",
                "folder/fn3.py",
                "folder/fn4.py",
                "package/__init__.py",
                "package/fn1.py",
                "package/fn2.py",
            ],
        )

    def test_find_imported_module_files_with_max_depth(self):
        """It limits discovery depth when max_depth is set."""
        files = find_dependency_files(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports",
            max_depth=1,
        )

        clean_paths = sorted(
            [os.path.join("", *f.split("/")[-2:]) for f in files],
        )

        self.assertEqual(
            clean_paths,
            [
                "fixtures/module_with_imports.py",
                "fixtures/simple_module.py",
                "folder/fn1.py",
                "folder/fn2.py",
                "folder/fn3.py",
                "package/__init__.py",
                "package/fn1.py",
                "package/fn2.py",
            ],
        )

    def test_find_imported_module_files_with_import_error(self):
        """It returns empty list for modules that cannot be imported."""
        files = find_dependency_files(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with",
        )
        self.assertEqual(len(files), 0)

    def test_find_imported_module_files_with_faulty_package(self):
        """It returns partially discovered files even if some imports fail."""
        files = find_dependency_files(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_faulty_imports",
            prefixes=["michelangelo"],
        )

        clean_paths = sorted(
            [os.path.join("", *f.split("/")[-2:]) for f in files],
        )

        self.assertEqual(
            clean_paths,
            [
                "faulty_package/__init__.py",
                "fixtures/module_with_faulty_imports.py",
            ],
        )

    def test_find_imported_module_files_with_relative_imports(self):
        """It handles relative imports correctly."""
        files = find_dependency_files(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_relative_imports",
        )

        clean_paths = sorted(
            [os.path.join("", *f.split("/")[-2:]) for f in files],
        )

        self.assertEqual(
            clean_paths,
            [
                "fixtures/module_with_relative_imports.py",
                "fixtures/simple_module.py",
                "folder/fn1.py",
                "folder/fn2.py",
                "folder/fn3.py",
                "folder/fn4.py",
                "package/__init__.py",
                "package/fn1.py",
                "package/fn2.py",
            ],
        )

    def test_find_imported_module_files_with_relative_imports_and_prefixes(self):
        """It handles relative imports with prefix filtering."""
        files = find_dependency_files(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_relative_imports",
            prefixes=[
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.folder",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.package",
            ],
        )

        clean_paths = sorted(
            [os.path.join("", *f.split("/")[-2:]) for f in files],
        )

        self.assertEqual(
            clean_paths,
            [
                "fixtures/module_with_relative_imports.py",
                "folder/fn1.py",
                "folder/fn2.py",
                "folder/fn3.py",
                "folder/fn4.py",
                "package/__init__.py",
                "package/fn1.py",
                "package/fn2.py",
            ],
        )

    def test_find_imported_module_files_with_multi_package_without_init(self):
        """It discovers files in implicit namespace packages."""
        files = find_dependency_files(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports_package_without_init",
        )

        clean_paths = sorted(
            [os.path.join("", *f.split("/")[-2:]) for f in files],
        )

        self.assertEqual(
            clean_paths,
            [
                "fixtures/module_with_imports_package_without_init.py",
                "fixtures/simple_module.py",
                "folder/fn1.py",
                "folder/fn2.py",
                "folder/fn3.py",
                "folder/fn4.py",
                "folder/fn5.py",
                "package/__init__.py",
                "package/fn1.py",
                "package/fn2.py",
            ],
        )
