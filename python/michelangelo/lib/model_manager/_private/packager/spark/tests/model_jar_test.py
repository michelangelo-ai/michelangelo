import os
import tempfile
from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.sdk.model_manager._private.utils.file_utils import cd
from uber.ai.michelangelo.sdk.model_manager._private.packager.spark import create_model_jar


class ModelJarTest(TestCase):
    def test_create_model_jar(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            model_jar_path = os.path.join(temp_dir, "model.jar")
            model_jar_content = {
                "a": "content_a",
                "b": "content_b",
            }

            create_model_jar(
                model_jar_content,
                model_jar_path,
            )
            with cd(temp_dir):
                os.system(f"jar xf {model_jar_path}")

            self.assertEqual(
                sorted(os.listdir(temp_dir)),
                ["a", "b", "model.jar"],
            )

            with open(os.path.join(temp_dir, "a")) as f:
                content_a = f.read()
                self.assertEqual(content_a, "content_a")

            with open(os.path.join(temp_dir, "b")) as f:
                content_b = f.read()
                self.assertEqual(content_b, "content_b")

    @patch("uber.ai.michelangelo.sdk.model_manager._private.packager.spark.model_jar.execute_cmd")
    def test_create_model_jar_error(self, mock_execute_cmd):
        mock_execute_cmd.return_value = (None, b"error", 1)

        with self.assertRaises(RuntimeError):
            create_model_jar(
                model_jar_content={},
                model_jar_path="invalid_path",
            )
