import os
import tempfile
from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder import find_dependency_files
from uber.ai.michelangelo.sdk.model_manager._private.utils.module_utils import save_module_files

# enable metabuild to build bazel dependencies
import uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports  # noqa:F401


class ModuleFilesTest(TestCase):
    def test_save_module_files(self):
        files = find_dependency_files("uber.ai.michelangelo.sdk.model_manager._private.utils.module_finder.tests.fixtures.module_with_imports")
        with tempfile.TemporaryDirectory() as target_dir:
            save_module_files(files, target_dir)
            saved_files = sorted(
                [os.path.relpath(os.path.join(dirpath, filename), target_dir) for dirpath, _, filenames in os.walk(target_dir) for filename in filenames]
            )
            self.assertEqual(
                saved_files,
                [
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn1.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn2.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn3.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn4.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/module_with_imports.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/package/__init__.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/package/fn1.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/package/fn2.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/simple_module.py",
                ],
            )
