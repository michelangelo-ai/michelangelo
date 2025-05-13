import os
import tempfile
from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager._private.constants import Placeholder
from uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils import replace_model_name_placeholder


class ModelNameTest(TestCase):
    def test_replace_model_name_placeholder(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            source = os.path.join(temp_dir, "source")
            os.makedirs(source)
            with open(os.path.join(source, "file.txt"), "w") as f:
                f.write(f"model_name: {Placeholder.MODEL_NAME}")

            target = os.path.join(temp_dir, "target")
            os.makedirs(target)
            with open(os.path.join(target, "config.pbtxt"), "w") as f:
                f.write(f"model_name: {Placeholder.MODEL_NAME}")

            replace_model_name_placeholder(temp_dir, "model")
            with open(os.path.join(target, "config.pbtxt")) as f:
                self.assertEqual(f.read(), "model_name: model")

            with open(os.path.join(source, "file.txt")) as f:
                self.assertEqual(f.read(), f"model_name: {Placeholder.MODEL_NAME}")

    def test_replace_model_name_placeholder_missing_file(self):
        with self.assertRaises(FileNotFoundError):
            replace_model_name_placeholder("non_exist_path", "model")
