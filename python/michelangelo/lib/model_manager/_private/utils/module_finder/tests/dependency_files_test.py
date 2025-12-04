import os
from unittest import TestCase

from michelangelo.lib.model_manager._private.utils.module_finder import (
    find_dependency_files,
)


class DependencyFilesTest(TestCase):
    """Tests dependency file finding utilities."""

    def setUp(self):
        """Set up the test fixture."""
        self.module_prefix = (
            "michelangelo.lib.model_manager._private.utils."
            "module_finder.tests.fixtures."
        )

    def test_find_imported_module_files(self):
        """It discovers imported files for a given module."""
        files = find_dependency_files(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports"
        )

        cleaned_files = {
            m: os.path.join("", *f.split("/")[-2:]) for m, f in files.items()
        }
        prefix = self.module_prefix

        expected_files = {
            f"{prefix}module_with_imports": "fixtures/module_with_imports.py",
            f"{prefix}simple_module": "fixtures/simple_module.py",
            f"{prefix}folder.fn1": "folder/fn1.py",
            f"{prefix}folder.fn2": "folder/fn2.py",
            f"{prefix}folder.fn3": "folder/fn3.py",
            f"{prefix}folder.fn4": "folder/fn4.py",
            f"{prefix}package.__init__": "package/__init__.py",
            f"{prefix}package.fn1": "package/fn1.py",
            f"{prefix}package.fn2": "package/fn2.py",
        }
        self.assertEqual(cleaned_files, expected_files)

    def test_find_imported_module_files_with_prefixes(self):
        """It filters discovered files by prefix."""
        files = find_dependency_files(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports",
            prefixes=[
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.folder",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.package",
            ],
        )

        cleaned_files = {
            m: os.path.join("", *f.split("/")[-2:]) for m, f in files.items()
        }
        prefix = self.module_prefix

        expected_files = {
            f"{prefix}module_with_imports": "fixtures/module_with_imports.py",
            f"{prefix}folder.fn1": "folder/fn1.py",
            f"{prefix}folder.fn2": "folder/fn2.py",
            f"{prefix}folder.fn3": "folder/fn3.py",
            f"{prefix}folder.fn4": "folder/fn4.py",
            f"{prefix}package.__init__": "package/__init__.py",
            f"{prefix}package.fn1": "package/fn1.py",
            f"{prefix}package.fn2": "package/fn2.py",
        }

        self.assertEqual(cleaned_files, expected_files)

    def test_find_imported_module_files_with_max_depth(self):
        """It limits discovery depth when max_depth is set."""
        files = find_dependency_files(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports",
            max_depth=1,
        )

        cleaned_files = {
            m: os.path.join("", *f.split("/")[-2:]) for m, f in files.items()
        }
        prefix = self.module_prefix

        expected_files = {
            f"{prefix}module_with_imports": "fixtures/module_with_imports.py",
            f"{prefix}simple_module": "fixtures/simple_module.py",
            f"{prefix}folder.fn1": "folder/fn1.py",
            f"{prefix}folder.fn2": "folder/fn2.py",
            f"{prefix}folder.fn3": "folder/fn3.py",
            f"{prefix}package.__init__": "package/__init__.py",
            f"{prefix}package.fn1": "package/fn1.py",
            f"{prefix}package.fn2": "package/fn2.py",
        }

        self.assertEqual(cleaned_files, expected_files)

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

        cleaned_files = {
            m: os.path.join("", *f.split("/")[-2:]) for m, f in files.items()
        }
        prefix = self.module_prefix

        expected_files = {
            f"{prefix}module_with_faulty_imports": "fixtures/module_with_faulty_imports.py",  # noqa: E501
            f"{prefix}faulty_package.__init__": "faulty_package/__init__.py",
        }

        self.assertEqual(cleaned_files, expected_files)

    def test_find_imported_module_files_with_relative_imports(self):
        """It handles relative imports correctly."""
        files = find_dependency_files(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_relative_imports",
        )

        cleaned_files = {
            m: os.path.join("", *f.split("/")[-2:]) for m, f in files.items()
        }
        prefix = self.module_prefix

        expected_files = {
            f"{prefix}module_with_relative_imports": "fixtures/module_with_relative_imports.py",  # noqa: E501
            f"{prefix}simple_module": "fixtures/simple_module.py",
            f"{prefix}folder.fn1": "folder/fn1.py",
            f"{prefix}folder.fn2": "folder/fn2.py",
            f"{prefix}folder.fn3": "folder/fn3.py",
            f"{prefix}folder.fn4": "folder/fn4.py",
            f"{prefix}package.__init__": "package/__init__.py",
            f"{prefix}package.fn1": "package/fn1.py",
            f"{prefix}package.fn2": "package/fn2.py",
        }

        self.assertEqual(cleaned_files, expected_files)

    def test_find_imported_module_files_with_relative_imports_and_prefixes(self):
        """It handles relative imports with prefix filtering."""
        files = find_dependency_files(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_relative_imports",
            prefixes=[
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.folder",
                "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.package",
            ],
        )

        cleaned_files = {
            m: os.path.join("", *f.split("/")[-2:]) for m, f in files.items()
        }
        prefix = self.module_prefix

        expected_files = {
            f"{prefix}module_with_relative_imports": "fixtures/module_with_relative_imports.py",  # noqa: E501
            f"{prefix}folder.fn1": "folder/fn1.py",
            f"{prefix}folder.fn2": "folder/fn2.py",
            f"{prefix}folder.fn3": "folder/fn3.py",
            f"{prefix}folder.fn4": "folder/fn4.py",
            f"{prefix}package.__init__": "package/__init__.py",
            f"{prefix}package.fn1": "package/fn1.py",
            f"{prefix}package.fn2": "package/fn2.py",
        }
        self.assertEqual(cleaned_files, expected_files)

    def test_find_imported_module_files_with_multi_package_without_init(self):
        """It discovers files in implicit namespace packages."""
        files = find_dependency_files(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports_package_without_init",
        )

        cleaned_files = {
            m: os.path.join("", *f.split("/")[-2:]) for m, f in files.items()
        }
        prefix = self.module_prefix

        expected_files = {
            f"{prefix}module_with_imports_package_without_init": "fixtures/module_with_imports_package_without_init.py",  # noqa: E501
            f"{prefix}simple_module": "fixtures/simple_module.py",
            f"{prefix}folder.fn1": "folder/fn1.py",
            f"{prefix}folder.fn2": "folder/fn2.py",
            f"{prefix}folder.fn3": "folder/fn3.py",
            f"{prefix}folder.fn4": "folder/fn4.py",
            f"{prefix}folder.fn5": "folder/fn5.py",
            f"{prefix}package.__init__": "package/__init__.py",
            f"{prefix}package.fn1": "package/fn1.py",
            f"{prefix}package.fn2": "package/fn2.py",
        }

        self.assertEqual(cleaned_files, expected_files)
