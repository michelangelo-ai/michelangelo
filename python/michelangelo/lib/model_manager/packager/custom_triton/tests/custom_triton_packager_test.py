"""Tests for CustomTritonPackager."""

import os
import tempfile
import numpy as np
from pathlib import Path
from unittest import TestCase
from michelangelo.lib.model_manager.packager.custom_triton import CustomTritonPackager
from michelangelo.lib.model_manager.schema import ModelSchema, ModelSchemaItem, DataType
from michelangelo.lib.model_manager._private.schema.common import schema_to_yaml

model_class = "michelangelo.lib.model_manager.packager.custom_triton.tests.fixtures.predict.Predict"
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
                "lib",
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
            "defs/model_class.txt",
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

    def test_create_raw_model_package(self):
        packager = CustomTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "raw_model")
            os.makedirs(model_path)
            with open(os.path.join(model_path, "file.txt"), "w") as f:
                f.write("file_content")
            dest_model_path = packager.create_raw_model_package(
                model_path=model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=self.sample_data,
                dest_model_path=dest_model_path,
                include_import_prefixes=["michelangelo"],
            )
            self.assert_raw_model_package(dest_model_path)

    def test_create_raw_model_package_with_custom_batch_processing(self):
        packager = CustomTritonPackager(custom_batch_processing=True)
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "raw_model")
            os.makedirs(model_path)
            with open(os.path.join(model_path, "file.txt"), "w") as f:
                f.write("file_content")
            dest_model_path = packager.create_raw_model_package(
                model_path=model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=self.batch_sample_data,
                dest_model_path=dest_model_path,
                include_import_prefixes=["michelangelo"],
            )
            self.assert_raw_model_package(dest_model_path, batch_inference=True)

    def test_create_raw_model_package_with_empty_model_schema(self):
        packager = CustomTritonPackager()
        with self.assertRaises(ValueError):
            packager.create_raw_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=None,
                sample_data=self.sample_data,
            )

    def test_create_raw_model_package_with_invalid_model_schema(self):
        model_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(
                    name="input",
                    data_type=DataType.UNKNOWN,
                ),
            ],
        )
        packager = CustomTritonPackager()
        with self.assertRaises(ValueError):
            packager.create_raw_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=model_schema,
                sample_data=self.sample_data,
            )

    def test_create_raw_model_package_missing_model_class(self):
        with self.assertRaises(ValueError):
            packager = CustomTritonPackager()
            packager.create_raw_model_package(
                "test_model_path",
                model_class=None,
                model_schema=self.model_schema,
                sample_data=self.sample_data,
            )

    def test_create_raw_model_package_with_invalid_model_class(self):
        with self.assertRaises(ValueError):
            packager = CustomTritonPackager()
            packager.create_raw_model_package(
                "test_model_path",
                model_class="invalid_class",
                model_schema=self.model_schema,
                sample_data=self.sample_data,
            )

    def test_create_raw_model_package_with_invalid_sample_data(self):
        with self.assertRaises(TypeError):
            packager = CustomTritonPackager()
            packager.create_raw_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=[{"a": "b"}],
            )

    def test_create_raw_model_package_with_mismatching_sample_data_and_model_schema(self):
        with self.assertRaises(ValueError):
            packager = CustomTritonPackager()
            packager.create_raw_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=[{"invalid": np.array([1])}],
            )

    def test_create_raw_model_package_with_requirements(self):
        packager = CustomTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "raw_model")
            os.makedirs(model_path)
            with open(os.path.join(model_path, "file.txt"), "w") as f:
                f.write("file_content")
            dest_model_path = packager.create_raw_model_package(
                model_path=model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=self.sample_data,
                dest_model_path=dest_model_path,
                requirements=["numpy", "pandas"],
                include_import_prefixes=["michelangelo"],
            )
            self.assert_raw_model_package(dest_model_path, with_requirements=True)

    def test_create_raw_model_package_with_default_dest_model_path(self):
        packager = CustomTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            with open(os.path.join(model_path, "file.txt"), "w") as f:
                f.write("file_content")
            dest_model_path = packager.create_raw_model_package(
                model_path=model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=self.sample_data,
                include_import_prefixes=["michelangelo"],
            )
            self.assert_raw_model_package(dest_model_path)

    def test_create_raw_model_package_with_altered_include_import_prefixes(self):
        packager = CustomTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "raw_model")
            os.makedirs(model_path)
            with open(os.path.join(model_path, "file.txt"), "w") as f:
                f.write("file_content")
            dest_model_path = packager.create_raw_model_package(
                model_path=model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=self.sample_data,
                dest_model_path=dest_model_path,
                include_import_prefixes=[
                    "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.package",
                    "michelangelo.lib.model_manager._private.utils.module_finder.tests.fixtures.simple_module",
                ],
            )

            files = sorted(
                [
                    str(
                        Path(os.path.join(dirpath, file)).relative_to(dest_model_path),
                    )
                    for dirpath, _, filenames in os.walk(dest_model_path)
                    for file in filenames
                ],
            )

            self.assertEqual(
                files,
                [
                    "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/__init__.py",
                    "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn1.py",
                    "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn2.py",
                    "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/simple_module.py",
                    "defs/michelangelo/lib/model_manager/interface/custom_model.py",
                    "defs/michelangelo/lib/model_manager/packager/custom_triton/tests/fixtures/predict.py",
                    "defs/model_class.txt",
                    "metadata/sample_data.json",
                    "metadata/schema.yaml",
                    "metadata/type.yaml",
                    "model/file.txt",
                ],
            )