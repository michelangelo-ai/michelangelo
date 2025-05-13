from unittest import TestCase
import os
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder import find_dependency_files

# enable metabuild to build bazel dependencies
import uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.simple_module  # noqa:F401


class DependencyFilesTest(TestCase):
    def test_find_imported_module_files(self):
        files = find_dependency_files("uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports")

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
        files = find_dependency_files(
            "uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports",
            prefixes=[
                "uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.folder",
                "uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.package",
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
        files = find_dependency_files(
            "uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports",
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
        files = find_dependency_files(
            "uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.module_with",
        )
        self.assertEqual(len(files), 0)

    def test_find_imported_module_files_with_faulty_package(self):
        files = find_dependency_files(
            "uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.module_with_faulty_imports",
            prefixes=["uber"],
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
        files = find_dependency_files(
            "uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.module_with_relative_imports",
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
        files = find_dependency_files(
            "uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.module_with_relative_imports",
            prefixes=[
                "uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.folder",
                "uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.package",
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
        files = find_dependency_files(
            "uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports_package_without_init",
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
