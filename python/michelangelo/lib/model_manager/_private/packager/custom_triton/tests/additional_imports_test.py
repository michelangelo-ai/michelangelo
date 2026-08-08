"""Tests for serialize_additional_imports."""

import os
import tempfile
from unittest import TestCase

from michelangelo.lib.model_manager._private.packager.custom_triton.additional_imports import (  # noqa: E501
    serialize_additional_imports,
)


class SerializeAdditionalImportsTest(TestCase):
    """Tests serialization of dynamically-imported module prefixes."""

    def test_serialize_additional_imports_adds_files(self):
        """It copies the resolved prefix's source file into the target dir."""
        prefix = (
            "michelangelo.lib.model_manager._private.utils.module_finder.tests."
            "fixtures.simple_module"
        )

        with tempfile.TemporaryDirectory() as target_dir:
            serialize_additional_imports(
                [prefix],
                target_dir,
                include_import_prefixes=["michelangelo"],
            )

            expected_file = os.path.join(
                target_dir,
                "michelangelo",
                "lib",
                "model_manager",
                "_private",
                "utils",
                "module_finder",
                "tests",
                "fixtures",
                "simple_module.py",
            )
            self.assertTrue(os.path.exists(expected_file))

            with open(expected_file) as f:
                content = f.read()
            self.assertIn("module_attr", content)

    def test_serialize_additional_imports_none_is_noop(self):
        """It does nothing when additional_import_prefixes is None."""
        with tempfile.TemporaryDirectory() as target_dir:
            serialize_additional_imports(None, target_dir)
            self.assertEqual(os.listdir(target_dir), [])

    def test_serialize_additional_imports_empty_list_is_noop(self):
        """It does nothing when additional_import_prefixes is empty."""
        with tempfile.TemporaryDirectory() as target_dir:
            serialize_additional_imports([], target_dir)
            self.assertEqual(os.listdir(target_dir), [])

    def test_serialize_additional_imports_multiple_prefixes(self):
        """It resolves and copies every prefix in the given list."""
        prefixes = [
            "michelangelo.lib.model_manager._private.utils.module_finder.tests."
            "fixtures.simple_module",
            "michelangelo.lib.model_manager._private.utils.module_finder.tests."
            "fixtures.module_with_imports",
        ]

        with tempfile.TemporaryDirectory() as target_dir:
            serialize_additional_imports(
                prefixes,
                target_dir,
                include_import_prefixes=["michelangelo"],
            )

            base = os.path.join(
                target_dir,
                "michelangelo",
                "lib",
                "model_manager",
                "_private",
                "utils",
                "module_finder",
                "tests",
                "fixtures",
            )
            self.assertTrue(os.path.exists(os.path.join(base, "simple_module.py")))
            self.assertTrue(
                os.path.exists(os.path.join(base, "module_with_imports.py"))
            )

    def test_serialize_additional_imports_skips_faulty_transitive_import(self):
        """A corrupted/missing transitive import doesn't break the whole package.

        ``module_with_faulty_imports`` imports ``faulty_package``, whose
        ``fn1.py`` does ``import non_exist_import``. That single broken file
        must not prevent the rest of the package's real dependency files from
        being serialized -- otherwise a model with one bad dynamic import
        would fail to package (or worse, package silently without files it
        actually needs) instead of just dropping the one unresolvable file.
        """
        prefix = (
            "michelangelo.lib.model_manager._private.utils.module_finder.tests."
            "fixtures.module_with_faulty_imports"
        )

        with tempfile.TemporaryDirectory() as target_dir:
            serialize_additional_imports(
                [prefix],
                target_dir,
                include_import_prefixes=["michelangelo"],
            )

            fixtures_dir = os.path.join(
                target_dir,
                "michelangelo",
                "lib",
                "model_manager",
                "_private",
                "utils",
                "module_finder",
                "tests",
                "fixtures",
            )
            faulty_package_dir = os.path.join(fixtures_dir, "faulty_package")

            # The importing module itself, and the faulty package's
            # __init__.py, are real resolvable files and must be copied.
            self.assertTrue(
                os.path.exists(
                    os.path.join(fixtures_dir, "module_with_faulty_imports.py")
                )
            )
            self.assertTrue(
                os.path.exists(os.path.join(faulty_package_dir, "__init__.py"))
            )

            # fn1.py itself cannot be imported (it imports a nonexistent
            # module), so find_dependency_files never resolves a file path
            # for it -- it must not appear in the copied output.
            self.assertFalse(os.path.exists(os.path.join(faulty_package_dir, "fn1.py")))
