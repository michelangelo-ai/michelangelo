import os
import tempfile
from unittest import TestCase
from michelangelo.lib.model_manager._private.packager.python_triton import serialize_model_loader


class ModelLoaderTest(TestCase):
    def test_serialize_model_loader(self):
        with tempfile.TemporaryDirectory() as target_dir:
            serialize_model_loader(target_dir)

            files = sorted(
                [os.path.relpath(os.path.join(dirpath, filename), target_dir) for dirpath, _, filenames in os.walk(target_dir) for filename in filenames]
            )

            self.assertEqual(
                files,
                [
                    "michelangelo/lib/model_manager/_private/serde/loader/custom_model_loader.py",
                    "michelangelo/lib/model_manager/_private/utils/pickle_utils/__init__.py",
                    "michelangelo/lib/model_manager/_private/utils/pickle_utils/pickle_definition.py",
                    "michelangelo/lib/model_manager/_private/utils/pickle_utils/pickle_definition_walker.py",
                    "michelangelo/lib/model_manager/_private/utils/pickle_utils/pickled_file.py",
                    "michelangelo/lib/model_manager/_private/utils/reflection_utils/__init__.py",
                    "michelangelo/lib/model_manager/_private/utils/reflection_utils/module.py",
                    "michelangelo/lib/model_manager/_private/utils/reflection_utils/module_attr.py",
                    "michelangelo/lib/model_manager/_private/utils/reflection_utils/root_import_path.py",
                    "michelangelo/lib/model_manager/interface/custom_model.py",
                ],
            )
