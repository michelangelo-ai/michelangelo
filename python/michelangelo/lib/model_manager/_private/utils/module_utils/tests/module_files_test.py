"""Tests for saving module files to the target directory."""

import os
import tempfile
from unittest import TestCase

from michelangelo.lib.model_manager._private.utils.module_finder import (
    find_dependency_files,
)
from michelangelo.lib.model_manager._private.utils.module_utils import save_module_files


class ModuleFilesTest(TestCase):
    """Tests saving dependency files to disk."""

    def test_save_module_files(self):
        """It copies dependent modules into the target directory."""
        files = find_dependency_files(
            "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports"
        )
        with tempfile.TemporaryDirectory() as target_dir:
            save_module_files(files, target_dir)
            saved_files = sorted(
                [
                    os.path.relpath(os.path.join(dirpath, filename), target_dir)
                    for dirpath, _, filenames in os.walk(target_dir)
                    for filename in filenames
                ]
            )
            prefix = (
                "michelangelo/lib/model_manager/_private/utils/"
                "module_finder/tests/fixtures/"
            )
            self.assertEqual(
                saved_files,
                [
                    f"{prefix}folder/fn1.py",
                    f"{prefix}folder/fn2.py",
                    f"{prefix}folder/fn3.py",
                    f"{prefix}folder/fn4.py",
                    f"{prefix}module_with_imports.py",
                    f"{prefix}package/__init__.py",
                    f"{prefix}package/fn1.py",
                    f"{prefix}package/fn2.py",
                    f"{prefix}simple_module.py",
                ],
            )
