import os
import tempfile
from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager._private.packager.python_triton import serialize_model_loader


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
                    "uber/ai/michelangelo/sdk/model_manager/_private/serde/loader/custom_model_loader.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/pickle_utils/__init__.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/pickle_utils/pickle_definition.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/pickle_utils/pickle_definition_walker.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/pickle_utils/pickled_file.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/reflection_utils/__init__.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/reflection_utils/module.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/reflection_utils/module_attr.py",
                    "uber/ai/michelangelo/sdk/model_manager/_private/utils/reflection_utils/root_import_path.py",
                    "uber/ai/michelangelo/sdk/model_manager/interface/custom_model.py",
                ],
            )
