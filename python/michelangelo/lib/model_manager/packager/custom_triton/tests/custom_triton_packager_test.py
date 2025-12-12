"""Tests for CustomTritonPackager."""

import importlib
import os
import pickle
import tempfile
from pathlib import Path
from unittest import TestCase

import numpy as np

from michelangelo.lib.model_manager._private.schema.common import schema_to_yaml
from michelangelo.lib.model_manager.packager.custom_triton import CustomTritonPackager
from michelangelo.lib.model_manager.packager.custom_triton.tests.fixtures.model import (
    Model,
)
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem

model_class = "michelangelo.lib.model_manager.packager.custom_triton.tests.fixtures.predict.Predict"  # noqa: E501
model_class_with_relative_imports = (
    "michelangelo.lib.model_manager.packager.custom_triton.tests.fixtures."
    "predict_with_relative_import.Predict"
)


class CustomTritonPackagerTest(TestCase):
    """Tests instantiation of the custom Triton packager."""

    def setUp(self):
        """Set up the test environment."""
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
        self.model_loader_files = [
            "0/michelangelo/lib/model_manager/_private/serde/loader/custom_model_loader.py",
            "0/michelangelo/lib/model_manager/_private/utils/pickle_utils/__init__.py",
            "0/michelangelo/lib/model_manager/_private/utils/pickle_utils/pickle_definition.py",
            "0/michelangelo/lib/model_manager/_private/utils/pickle_utils/pickle_definition_walker.py",
            "0/michelangelo/lib/model_manager/_private/utils/pickle_utils/pickled_file.py",
            "0/michelangelo/lib/model_manager/_private/utils/reflection_utils/__init__.py",
            "0/michelangelo/lib/model_manager/_private/utils/reflection_utils/module.py",
            "0/michelangelo/lib/model_manager/_private/utils/reflection_utils/root_import_path.py",
        ]

    def test_custom_triton_packager(self):
        """It creates a packager instance with default settings."""
        packager = CustomTritonPackager()
        self.assertIsNotNone(packager)

    def assert_model_package(self, dest_model_path):
        """Assert that the model package has the expected structure."""
        with open(os.path.join(dest_model_path, "0", "model_class.txt")) as f:
            content = f.read()
            self.assertEqual(content, model_class)

        with open(os.path.join(dest_model_path, "0", "model.py")) as f:
            content = f.read()
            self.assertIsNotNone(content)

        with open(os.path.join(dest_model_path, "0", "user_model.py")) as f:
            content = f.read()
            self.assertIsNotNone(content)

        with open(os.path.join(dest_model_path, "0", "model", "file.txt")) as f:
            content = f.read()
            self.assertEqual(content, "file_content")

        with open(
            os.path.join(
                dest_model_path,
                "0",
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

        files = sorted(
            [
                str(
                    Path(os.path.join(dirpath, file)).relative_to(dest_model_path),
                )
                for dirpath, _, filenames in os.walk(dest_model_path)
                for file in filenames
            ],
        )

        package_files = [
            "0/model.py",
            "0/model/file.txt",
            "0/model_class.txt",
            "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn1.py",
            "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn2.py",
            "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn3.py",
            "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn4.py",
            "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/__init__.py",
            "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn1.py",
            "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn2.py",
            "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/simple_module.py",
            "0/michelangelo/lib/model_manager/interface/custom_model.py",
            "0/michelangelo/lib/model_manager/packager/custom_triton/tests/fixtures/predict.py",
            "0/user_model.py",
            "config.pbtxt",
        ]

        expected_files = sorted(package_files + self.model_loader_files)
        self.assertEqual(files, expected_files)

    def test_create_model_package(self):
        """It creates a model package."""
        packager = CustomTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "deployable_model")
            os.makedirs(model_path)
            with open(os.path.join(model_path, "file.txt"), "w") as f:
                f.write("file_content")
            dest_model_path = packager.create_model_package(
                model_path=model_path,
                dest_model_path=dest_model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                model_name="test_model_name",
                include_import_prefixes=["michelangelo"],
            )

            self.assert_model_package(dest_model_path)

            with (
                open(
                    "michelangelo/lib/model_manager/packager/custom_triton/tests/fixtures/config.pbtxt"
                ) as expected_f,
                open(os.path.join(dest_model_path, "config.pbtxt")) as f,
            ):
                expected_config = expected_f.read()
                config = f.read()
                self.assertEqual(config, expected_config)

            # running the predict function
            loaded_model_class = None
            with open(os.path.join(dest_model_path, "0", "model_class.txt")) as f:
                loaded_model_class = f.read().strip()

            module_def, _, class_name = loaded_model_class.rpartition(".")
            module = importlib.import_module(module_def)
            predict_class = getattr(module, class_name)
            predict_obj = predict_class()
            self.assertEqual(
                predict_obj.predict(self.sample_data[0]),
                {"response": self.sample_data[0].get("input")},
            )

    def test_create_model_package_without_dest_model_path(self):
        """It creates a model package without a destination model path."""
        packager = CustomTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            os.makedirs(model_path)
            with open(os.path.join(model_path, "file.txt"), "w") as f:
                f.write("file_content")
            dest_model_path = packager.create_model_package(
                model_path=model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                model_name="test_model_name",
                include_import_prefixes=["michelangelo"],
            )

            self.assert_model_package(dest_model_path)

            with (
                open(
                    "michelangelo/lib/model_manager/packager/custom_triton/tests/fixtures/config.pbtxt"
                ) as expected_f,
                open(os.path.join(dest_model_path, "config.pbtxt")) as f,
            ):
                expected_config = expected_f.read()
                config = f.read()
                self.assertEqual(config, expected_config)

            # running the predict function
            loaded_model_class = None
            with open(os.path.join(dest_model_path, "0", "model_class.txt")) as f:
                loaded_model_class = f.read().strip()

            module_def, _, class_name = loaded_model_class.rpartition(".")
            module = importlib.import_module(module_def)
            predict_class = getattr(module, class_name)
            predict_obj = predict_class()
            self.assertEqual(
                predict_obj.predict(self.sample_data[0]),
                {"response": self.sample_data[0].get("input")},
            )

    def test_create_model_package_with_custom_batch_processing(self):
        """It creates a model package with custom batch processing."""
        packager = CustomTritonPackager(custom_batch_processing=True)
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "deployable_model")
            os.makedirs(model_path)
            with open(os.path.join(model_path, "file.txt"), "w") as f:
                f.write("file_content")
            dest_model_path = packager.create_model_package(
                model_path=model_path,
                dest_model_path=dest_model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                model_name="test_model_name",
                include_import_prefixes=["michelangelo"],
            )

            self.assert_model_package(dest_model_path)

            with (
                open(
                    "michelangelo/lib/model_manager/packager/custom_triton/tests/fixtures/config.pbtxt"
                ) as expected_f,
                open(os.path.join(dest_model_path, "config.pbtxt")) as f,
            ):
                expected_config = expected_f.read()
                config = f.read()
                self.assertEqual(config, expected_config)

            # running the predict function
            loaded_model_class = None
            with open(os.path.join(dest_model_path, "0", "model_class.txt")) as f:
                loaded_model_class = f.read().strip()

            module_def, _, class_name = loaded_model_class.rpartition(".")
            module = importlib.import_module(module_def)
            predict_class = getattr(module, class_name)
            predict_obj = predict_class()
            self.assertEqual(
                predict_obj.predict(self.batch_sample_data[0]),
                {"response": self.batch_sample_data[0].get("input")},
            )

    def test_create_model_package_with_empty_model_schema(self):
        """It raises ValueError when model schema is empty."""
        packager = CustomTritonPackager()
        with self.assertRaises(ValueError):
            packager.create_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=None,
            )

    def test_create_model_package_with_invalid_model_schema(self):
        """It raises ValueError when model schema is invalid."""
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
            packager.create_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=model_schema,
            )

    def test_create_model_package_with_default_dest_model_path(self):
        """It creates a model package with default destination path."""
        packager = CustomTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "deployable_model")
            os.makedirs(model_path)
            with open(os.path.join(model_path, "file.txt"), "w") as f:
                f.write("file_content")
            dest_model_path = packager.create_model_package(
                model_path=model_path,
                dest_model_path=dest_model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                include_import_prefixes=["michelangelo"],
            )

            self.assert_model_package(dest_model_path)

    def test_missing_model_class(self):
        """It raises ValueError when model class is missing."""
        with self.assertRaises(ValueError):
            packager = CustomTritonPackager()
            packager.create_model_package(
                "test_model_path",
                model_class=None,
                model_schema=self.model_schema,
            )

    def test_with_invalid_model_class(self):
        """It raises ValueError when model class is invalid."""
        with self.assertRaises(ValueError):
            packager = CustomTritonPackager()
            packager.create_model_package(
                "test_model_path",
                model_class="invalid_class",
                model_schema=self.model_schema,
            )

    def test_create_model_package_with_altered_include_import_prefixes(self):
        """It creates a model package with altered import prefixes."""
        packager = CustomTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "deployable_model")
            os.makedirs(model_path)
            with open(os.path.join(model_path, "file.txt"), "w") as f:
                f.write("file_content")
            dest_model_path = packager.create_model_package(
                model_path=model_path,
                model_class=model_class,
                model_schema=self.model_schema,
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

            package_files = [
                "0/model.py",
                "0/model/file.txt",
                "0/model_class.txt",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/__init__.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn1.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn2.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/simple_module.py",
                "0/michelangelo/lib/model_manager/interface/custom_model.py",
                "0/michelangelo/lib/model_manager/packager/custom_triton/tests/fixtures/predict.py",
                "0/user_model.py",
                "config.pbtxt",
            ]

            expected_files = sorted(package_files + self.model_loader_files)
            self.assertEqual(files, expected_files)

    def test_create_model_package_with_relative_imports(self):
        """It creates a model package with relative imports."""
        packager = CustomTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "deployable_model")
            os.makedirs(model_path)
            with open(os.path.join(model_path, "file.txt"), "w") as f:
                f.write("file_content")
            dest_model_path = packager.create_model_package(
                model_path=model_path,
                model_schema=self.model_schema,
                model_class=model_class_with_relative_imports,
                model_name="test_model_name",
                dest_model_path=dest_model_path,
                include_import_prefixes=["michelangelo"],
            )

            with open(os.path.join(dest_model_path, "0", "model_class.txt")) as f:
                content = f.read()
                self.assertEqual(content, model_class_with_relative_imports)

            with open(os.path.join(dest_model_path, "0", "model.py")) as f:
                content = f.read()
                self.assertIsNotNone(content)

            with open(os.path.join(dest_model_path, "0", "user_model.py")) as f:
                content = f.read()
                self.assertIsNotNone(content)

            with open(os.path.join(dest_model_path, "0", "model", "file.txt")) as f:
                content = f.read()
                self.assertEqual(content, "file_content")

            with open(
                os.path.join(
                    dest_model_path,
                    "0",
                    "michelangelo",
                    "lib",
                    "model_manager",
                    "packager",
                    "custom_triton",
                    "tests",
                    "fixtures",
                    "predict_with_relative_import.py",
                ),
            ) as f:
                content = f.read()
                self.assertIn("class Predict(Model):", content)

            files = sorted(
                [
                    str(
                        Path(os.path.join(dirpath, file)).relative_to(dest_model_path),
                    )
                    for dirpath, _, filenames in os.walk(dest_model_path)
                    for file in filenames
                ],
            )

            package_files = [
                "0/model.py",
                "0/model/file.txt",
                "0/model_class.txt",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn1.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn2.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn3.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn4.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/__init__.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn1.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn2.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/simple_module.py",
                "0/michelangelo/lib/model_manager/interface/custom_model.py",
                "0/michelangelo/lib/model_manager/packager/custom_triton/tests/fixtures/predict_with_relative_import.py",
                "0/user_model.py",
                "config.pbtxt",
            ]

            expected_files = sorted(package_files + self.model_loader_files)
            self.assertEqual(files, expected_files)

            with (
                open(
                    "michelangelo/lib/model_manager/packager/custom_triton/tests/fixtures/config.pbtxt"
                ) as expected_f,
                open(os.path.join(dest_model_path, "config.pbtxt")) as f,
            ):
                expected_config = expected_f.read()
                config = f.read()
                self.assertEqual(config, expected_config)

            # running the predict function
            loaded_model_class = None
            with open(os.path.join(dest_model_path, "0", "model_class.txt")) as f:
                loaded_model_class = f.read().strip()

            module_def, _, class_name = loaded_model_class.rpartition(".")
            module = importlib.import_module(module_def)
            predict_class = getattr(module, class_name)
            predict_obj = predict_class()
            model_path = os.path.join(dest_model_path, "0", "model")
            self.assertEqual(predict_obj.predict(model_path), model_path)

    def test_create_model_package_with_pickle(self):
        """It creates a model package with pickle files."""
        packager = CustomTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "deployable_model")
            os.makedirs(model_path)
            with open(os.path.join(model_path, "file.txt"), "w+") as f:
                f.write("file_content")

            with open(os.path.join(model_path, "file.pkl"), "wb") as f:
                pickle.dump(Model(), f)
            dest_model_path = packager.create_model_package(
                model_path=model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                model_name="test_model_name",
                dest_model_path=dest_model_path,
                include_import_prefixes=["michelangelo"],
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

            package_files = [
                "0/model.py",
                "0/model/file.pkl",
                "0/model/file.txt",
                "0/model_class.txt",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn1.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn2.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn3.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn4.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/__init__.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn1.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn2.py",
                "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/simple_module.py",
                "0/michelangelo/lib/model_manager/interface/custom_model.py",
                "0/michelangelo/lib/model_manager/packager/custom_triton/tests/fixtures/model.py",
                "0/michelangelo/lib/model_manager/packager/custom_triton/tests/fixtures/predict.py",
                "0/user_model.py",
                "config.pbtxt",
            ]

            expected_files = sorted(package_files + self.model_loader_files)
            self.assertEqual(files, expected_files)

    def test_create_model_package_with_empty_model(self):
        """It creates a model package with an empty model directory."""
        packager = CustomTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "deployable_model")
            os.makedirs(model_path)
            dest_model_path = packager.create_model_package(
                model_path=model_path,
                dest_model_path=dest_model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                model_name="test_model_name",
                include_import_prefixes=["michelangelo"],
            )

            with (
                open(
                    "michelangelo/lib/model_manager/packager/custom_triton/tests/fixtures/config.pbtxt"
                ) as expected_f,
                open(os.path.join(dest_model_path, "config.pbtxt")) as f,
            ):
                expected_config = expected_f.read()
                config = f.read()
                self.assertEqual(config, expected_config)

            self.assertTrue(os.path.exists(os.path.join(dest_model_path, "0", "model")))
            self.assertEqual(
                len(os.listdir(os.path.join(dest_model_path, "0", "model"))), 0
            )

    def assert_raw_model_package(
        self, dest_model_path, with_requirements=False, batch_inference=False
    ):
        """Assert that the raw model package has the expected structure."""
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
                self.assertEqual(
                    content, "type: custom-python\nbatch_inference: true\n"
                )
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
        """It creates a raw model package."""
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
        """It creates a raw model package with custom batch processing."""
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
        """It creates a raw model package with empty model schema."""
        packager = CustomTritonPackager()
        with self.assertRaises(ValueError):
            packager.create_raw_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=None,
                sample_data=self.sample_data,
            )

    def test_create_raw_model_package_with_invalid_model_schema(self):
        """It creates a raw model package with invalid model schema."""
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
        """It creates a raw model package with missing model class."""
        with self.assertRaises(ValueError):
            packager = CustomTritonPackager()
            packager.create_raw_model_package(
                "test_model_path",
                model_class=None,
                model_schema=self.model_schema,
                sample_data=self.sample_data,
            )

    def test_create_raw_model_package_with_invalid_model_class(self):
        """It creates a raw model package with invalid model class."""
        with self.assertRaises(ValueError):
            packager = CustomTritonPackager()
            packager.create_raw_model_package(
                "test_model_path",
                model_class="invalid_class",
                model_schema=self.model_schema,
                sample_data=self.sample_data,
            )

    def test_create_raw_model_package_with_invalid_sample_data(self):
        """It creates a raw model package with invalid sample data."""
        with self.assertRaises(TypeError):
            packager = CustomTritonPackager()
            packager.create_raw_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=[{"a": "b"}],
            )

    def test_create_raw_model_package_with_mismatching_sample_data_and_model_schema(
        self,
    ):
        """It creates a raw model package with mismatching sample data and schema."""
        with self.assertRaises(ValueError):
            packager = CustomTritonPackager()
            packager.create_raw_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=[{"invalid": np.array([1])}],
            )

    def test_create_raw_model_package_with_requirements(self):
        """It creates a raw model package with requirements."""
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
        """It creates a raw model package with default dest model path."""
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
        """It creates a raw model package with altered include import prefixes."""
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

    def test_create_raw_model_package_with_pickle(self):
        """It creates a raw model package with pickle."""
        packager = CustomTritonPackager()

        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "raw_model")
            os.makedirs(model_path)
            with open(os.path.join(model_path, "file.txt"), "w+") as f:
                f.write("file_content")
            with open(os.path.join(model_path, "file.pkl"), "wb") as f:
                pickle.dump(Model(), f)
            dest_model_path = packager.create_raw_model_package(
                model_path=model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=self.sample_data,
                dest_model_path=dest_model_path,
                include_import_prefixes=["michelangelo"],
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
                    "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn1.py",
                    "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn2.py",
                    "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn3.py",
                    "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn4.py",
                    "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/__init__.py",
                    "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn1.py",
                    "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn2.py",
                    "defs/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/simple_module.py",
                    "defs/michelangelo/lib/model_manager/interface/custom_model.py",
                    "defs/michelangelo/lib/model_manager/packager/custom_triton/tests/fixtures/model.py",
                    "defs/michelangelo/lib/model_manager/packager/custom_triton/tests/fixtures/predict.py",
                    "defs/model_class.txt",
                    "metadata/sample_data.json",
                    "metadata/schema.yaml",
                    "metadata/type.yaml",
                    "model/file.pkl",
                    "model/file.txt",
                ],
            )

    def test_create_raw_model_package_with_empty_model(self):
        """It creates a raw model package with empty model."""
        packager = CustomTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "raw_model")
            os.makedirs(model_path)
            dest_model_path = packager.create_raw_model_package(
                model_path=model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=self.sample_data,
                dest_model_path=dest_model_path,
                include_import_prefixes=["michelangelo"],
            )

            self.assertTrue(os.path.exists(os.path.join(dest_model_path, "model")))
            self.assertEqual(len(os.listdir(os.path.join(dest_model_path, "model"))), 0)
