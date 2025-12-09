"""Tests for CustomTritonPackager."""

import os
import numpy as np
from pathlib import Path
from unittest import TestCase
from michelangelo.lib.model_manager.packager.custom_triton import CustomTritonPackager
from michelangelo.lib.model_manager.schema import ModelSchema, ModelSchemaItem, DataType

model_class = "michelangelo.lib.model_manager.packager.python_triton.tests.fixtures.predict.Predict"
model_class_with_relative_imports = "michelangelo.lib.model_manager.packager.custom_triton.tests.fixtures.predict_with_relative_import.Predict"


class CustomTritonPackagerTest(TestCase):
    """Tests instantiation of the custom Triton packager."""
    def setUp(self):
        self.model_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(
                    name="input",
                    data_type=DataType.INT,
                    shape=[1],
                ),
            ],
            output_schema=[
                ModelSchemaItem(
                    name="response",
                    data_type=DataType.INT,
                    shape=[1],
                ),
            ],
        )
        self.sample_data = [{"input": np.array([1])}]
        self.batch_sample_data = [{"input": np.array([[1], [2]])}]

    def test_custom_triton_packager(self):
        """It creates a packager instance with default settings."""
        packager = CustomTritonPackager()
        self.assertIsNotNone(packager)

    def assert_raw_model_package(self, dest_model_path, with_requirements=False, batch_inference=False):
        with open(os.path.join(dest_model_path, "defs", "model_class.txt")) as f:
            content = f.read()
            self.assertEqual(content, model_class)

        with open(
            os.path.join(
                dest_model_path,
                "defs",
                "michelangelo",
                "ai",
                "model_manager",
                "packager",
                "custom_triton",
                "tests",
                "fixtures",
                "predict.py",
            ),
        ) as f:
            content = f.read()
            self.assertIn("class Predict(Model):", content)

        with open(os.path.join(dest_model_path, "model", "file.txt")) as f:
            content = f.read()
            self.assertEqual(content, "file_content")

        with open(os.path.join(dest_model_path, "metadata", "type.yaml")) as f:
            content = f.read()
            if batch_inference:
                self.assertEqual(content, "type: custom-python\nbatch_inference: true\n")
            else:
                self.assertEqual(content, "type: custom-python\n")

        with open(os.path.join(dest_model_path, "metadata", "schema.yaml")) as f:
            content = f.read()
            self.assertEqual(content, schema_to_yaml(self.model_schema))

        with open(os.path.join(dest_model_path, "metadata", "sample_data.json")) as f:
            content = f.read()
            if batch_inference:
                self.assertEqual(content, '[{"input": [[1], [2]]}]')
            else:
                self.assertEqual(content, '[{"input": [1]}]')

        files = sorted(
            [
                str(
                    Path(os.path.join(dirpath, file)).relative_to(dest_model_path),
                )
                for dirpath, _, filenames in os.walk(dest_model_path)
                for file in filenames
            ],
        )

        expected_files = [
            "defs/model_class.txt",
            "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn1.py",
            "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn2.py",
            "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn3.py",
            "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn4.py",
            "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/__init__.py",
            "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn1.py",
            "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn2.py",
            "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/simple_module.py",
            "defs/michelangelo/lib/model_manager/interface/custom_model.py",
            "defs/michelangelo/lib/model_manager/packager/custom_triton/tests/fixtures/predict.py",
            "metadata/sample_data.json",
            "metadata/schema.yaml",
            "metadata/type.yaml",
            "model/file.txt",
        ]

        if with_requirements:
            expected_files.insert(-4, "dependencies/requirements.txt")

        self.assertEqual(
            files,
            expected_files,
        )