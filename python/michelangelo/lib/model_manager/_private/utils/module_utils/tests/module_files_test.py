import os
import tempfile
from unittest import TestCase
from michelangelo.lib.model_manager._private.utils.module_finder import find_dependency_files
from michelangelo.lib.model_manager._private.utils.module_utils import save_module_files

# enable metabuild to build bazel dependencies
import michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports  # noqa:F401


class ModuleFilesTest(TestCase):
    def test_save_module_files(self):
        files = find_dependency_files("michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports")
        with tempfile.TemporaryDirectory() as target_dir:
            save_module_files(files, target_dir)
            saved_files = sorted(
                [os.path.relpath(os.path.join(dirpath, filename), target_dir) for dirpath, _, filenames in os.walk(target_dir) for filename in filenames]
            )
            self.assertEqual(
                saved_files,
                [
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn1.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn2.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn3.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn4.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/module_with_imports.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/__init__.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn1.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn2.py",
                    "michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/simple_module.py",
                ],
            )
