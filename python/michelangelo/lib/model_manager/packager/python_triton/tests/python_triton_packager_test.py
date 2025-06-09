import os
import tempfile
import importlib
import pickle
import numpy as np
from unittest import TestCase
from unittest.mock import patch
from typing import Optional
from pathlib import Path
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager.schema import (
    ModelSchema,
    ModelSchemaItem,
    DataType,
)
from michelangelo.lib.model_manager.packager.python_triton import PythonTritonPackager
from michelangelo.lib.model_manager._private.schema.common import schema_to_yaml
from michelangelo.lib.model_manager.packager.python_triton.tests.fixtures.model import Model

# enable metabuild to build bazel dependencies
import michelangelo.lib.model_manager.packager.python_triton.tests.fixtures.predict  # noqa:F401

model_class = "michelangelo.lib.model_manager.packager.python_triton.tests.fixtures.predict.Predict"
model_class_with_relative_imports = "michelangelo.lib.model_manager.packager.python_triton.tests.fixtures.predict_with_relative_import.Predict"


def download_model(
    model_path: str,  # noqa: ARG001
    dest_model_path: Optional[str] = None,
    model_path_source_type: Optional[str] = StorageType.HDFS,  # noqa: ARG001
) -> str:
    if not os.path.exists(dest_model_path):
        os.makedirs(dest_model_path)

    with open(os.path.join(dest_model_path, "file.txt"), "w+") as f:
        f.write("file_content")

    return dest_model_path


def download_model_with_pickle(
    model_path: str,  # noqa: ARG001
    dest_model_path: Optional[str] = None,
    model_path_source_type: Optional[str] = StorageType.HDFS,  # noqa: ARG001
) -> str:
    if not os.path.exists(dest_model_path):
        os.makedirs(dest_model_path)

    with open(os.path.join(dest_model_path, "file.txt"), "w+") as f:
        f.write("file_content")

    with open(os.path.join(dest_model_path, "file.pkl"), "wb") as f:
        pickle.dump(Model(), f)

    return dest_model_path


def download_empty_model(
    model_path: str,  # noqa: ARG001
    dest_model_path: Optional[str] = None,
    model_path_source_type: Optional[str] = StorageType.HDFS,  # noqa: ARG001
) -> str:
    return dest_model_path


class PythonTritonPackagerTest(TestCase):
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
        self.model_loader_files = [
            "0/michelangelo/lib/model_manager/_private/serde/loader/custom_model_loader.py",
            "0/michelangelo/lib/model_manager/_private/utils/pickle_utils/__init__.py",
            "0/michelangelo/lib/model_manager/_private/utils/pickle_utils/pickle_definition.py",
            "0/michelangelo/lib/model_manager/_private/utils/pickle_utils/pickle_definition_walker.py",
            "0/michelangelo/lib/model_manager/_private/utils/pickle_utils/pickled_file.py",
            "0/michelangelo/lib/model_manager/_private/utils/reflection_utils/__init__.py",
            "0/michelangelo/lib/model_manager/_private/utils/reflection_utils/module.py",
            "0/michelangelo/lib/model_manager/_private/utils/reflection_utils/module_attr.py",
            "0/michelangelo/lib/model_manager/_private/utils/reflection_utils/root_import_path.py",
        ]

    def assertModelPackage(self, dest_model_path, mock_download_model):
        mock_download_model.assert_called_once_with(
            "test_model_path",
            os.path.join(dest_model_path, "0", "model"),
            "hdfs",
        )

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
                "python_triton",
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
            "0/uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn1.py",
            "0/uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn2.py",
            "0/uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn3.py",
            "0/uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn4.py",
            "0/uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/package/__init__.py",
            "0/uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/package/fn1.py",
            "0/uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/package/fn2.py",
            "0/uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/simple_module.py",
            "0/uber/ai/michelangelo/sdk/model_manager/interface/custom_model.py",
            "0/uber/ai/michelangelo/sdk/model_manager/packager/python_triton/tests/fixtures/predict.py",
            "0/user_model.py",
            "config.pbtxt",
        ]

        expected_files = sorted(package_files + self.model_loader_files)
        self.assertEqual(files, expected_files)

    @patch("michelangelo.lib.model_manager._private.packager.python_triton.model_package.download_model", wraps=download_model)
    def test_create_model_package(self, mock_download_model):
        packager = PythonTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")
            dest_model_path = packager.create_model_package(
                "test_model_path",
                dest_model_path=dest_model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                model_name="test_model_name",
            )

            self.assertModelPackage(dest_model_path, mock_download_model)

            with open("uber/ai/michelangelo/sdk/model_manager/packager/python_triton/tests/fixtures/config.pbtxt") as expected_f:
                with open(os.path.join(dest_model_path, "config.pbtxt")) as f:
                    expected_config = expected_f.read()
                    config = f.read()
                    self.assertEqual(config, expected_config)

            # running the predict function
            loaded_model_class = None
            with open(os.path.join(dest_model_path, "0", "model_class.txt")) as f:
                loaded_model_class = f.read().strip()

            module_def, _, class_name = loaded_model_class.rpartition(".")
            module = importlib.import_module(module_def)
            Predict = getattr(module, class_name)
            predict_obj = Predict()
            self.assertEqual(predict_obj.predict(self.sample_data[0]), {"response": self.sample_data[0].get("input")})

    @patch("michelangelo.lib.model_manager._private.packager.python_triton.model_package.download_model", wraps=download_model)
    def test_create_model_package_with_custom_batch_processing(self, mock_download_model):
        packager = PythonTritonPackager(custom_batch_processing=True)
        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")
            dest_model_path = packager.create_model_package(
                "test_model_path",
                dest_model_path=dest_model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                model_name="test_model_name",
            )

            self.assertModelPackage(dest_model_path, mock_download_model)

            with open("uber/ai/michelangelo/sdk/model_manager/packager/python_triton/tests/fixtures/config.pbtxt") as expected_f:
                with open(os.path.join(dest_model_path, "config.pbtxt")) as f:
                    expected_config = expected_f.read()
                    config = f.read()
                    self.assertEqual(config, expected_config)

            # running the predict function
            loaded_model_class = None
            with open(os.path.join(dest_model_path, "0", "model_class.txt")) as f:
                loaded_model_class = f.read().strip()

            module_def, _, class_name = loaded_model_class.rpartition(".")
            module = importlib.import_module(module_def)
            Predict = getattr(module, class_name)
            predict_obj = Predict()
            self.assertEqual(predict_obj.predict(self.batch_sample_data[0]), {"response": self.batch_sample_data[0].get("input")})

    def test_create_model_package_with_empty_model_schema(self):
        packager = PythonTritonPackager()
        with self.assertRaises(ValueError):
            packager.create_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=None,
            )

    def test_create_model_package_with_invalid_model_schema(self):
        model_schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(
                    name="input",
                    data_type=DataType.UNKNOWN,
                ),
            ],
        )
        packager = PythonTritonPackager()
        with self.assertRaises(ValueError):
            packager.create_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=model_schema,
            )

    def test_create_model_package_with_default_dest_model_path(self):
        packager = PythonTritonPackager()
        with patch(
            "michelangelo.lib.model_manager._private.packager.python_triton.model_package.download_model",
            wraps=download_model,
        ) as mock_download_model:
            dest_model_path = packager.create_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=self.model_schema,
            )

            self.assertModelPackage(dest_model_path, mock_download_model)

    def test_missing_model_class(self):
        with self.assertRaises(ValueError):
            packager = PythonTritonPackager()
            packager.create_model_package(
                "test_model_path",
                model_class=None,
                model_schema=self.model_schema,
            )

    def test_with_invalid_model_class(self):
        with self.assertRaises(ValueError):
            packager = PythonTritonPackager()
            packager.create_model_package(
                "test_model_path",
                model_class="invalid_class",
                model_schema=self.model_schema,
            )

    def test_create_model_package_with_altered_include_import_prefixes(self):
        packager = PythonTritonPackager()
        with patch(
            "michelangelo.lib.model_manager._private.packager.python_triton.model_package.download_model",
            wraps=download_model,
        ):
            with tempfile.TemporaryDirectory() as temp_dir:
                dest_model_path = os.path.join(temp_dir, "model")
                dest_model_path = packager.create_model_package(
                    "test_model_path",
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
                    "0/uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/package/__init__.py",
                    "0/uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/package/fn1.py",
                    "0/uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/package/fn2.py",
                    "0/uber/ai/michelangelo/sdk/model_manager/_private/utils/module_finder/tests/fixtures/simple_module.py",
                    "0/uber/ai/michelangelo/sdk/model_manager/interface/custom_model.py",
                    "0/uber/ai/michelangelo/sdk/model_manager/packager/python_triton/tests/fixtures/predict.py",
                    "0/user_model.py",
                    "config.pbtxt",
                ]

                expected_files = sorted(package_files + self.model_loader_files)
                self.assertEqual(files, expected_files)

    def test_create_model_package_with_relative_imports(self):
        packager = PythonTritonPackager()

        with patch(
            "michelangelo.lib.model_manager._private.packager.python_triton.model_package.download_model",
            wraps=download_model,
        ) as mock_download_model:
            with tempfile.TemporaryDirectory() as temp_dir:
                dest_model_path = os.path.join(temp_dir, "model")
                dest_model_path = packager.create_model_package(
                    "test_model_path",
                    model_schema=self.model_schema,
                    model_class=model_class_with_relative_imports,
                    model_name="test_model_name",
                    dest_model_path=dest_model_path,
                )

                mock_download_model.assert_called_once_with(
                    "test_model_path",
                    os.path.join(dest_model_path, "0", "model"),
                    "hdfs",
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
                        "python_triton",
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
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn1.py",
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn2.py",
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn3.py",
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn4.py",
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/__init__.py",
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn1.py",
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn2.py",
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/simple_module.py",
                    "0/michelangelo/lib/model_manager/interface/custom_model.py",
                    "0/michelangelo/lib/model_manager/packager/python_triton/tests/fixtures/predict_with_relative_import.py",
                    "0/model.py",
                    "0/model/file.txt",
                    "0/model_class.txt",
                    "0/user_model.py",
                    "config.pbtxt",
                ]

                expected_files = sorted(package_files + self.model_loader_files)
                self.assertEqual(files, expected_files)

                with open("michelangelo/lib/model_manager/packager/python_triton/tests/fixtures/config.pbtxt") as expected_f:
                    with open(os.path.join(dest_model_path, "config.pbtxt")) as f:
                        expected_config = expected_f.read()
                        config = f.read()
                        self.assertEqual(config, expected_config)

                # running the predict function
                loaded_model_class = None
                with open(os.path.join(dest_model_path, "0", "model_class.txt")) as f:
                    loaded_model_class = f.read().strip()

                module_def, _, class_name = loaded_model_class.rpartition(".")
                module = importlib.import_module(module_def)
                Predict = getattr(module, class_name)
                predict_obj = Predict()
                model_path = os.path.join(dest_model_path, "0", "model")
                self.assertEqual(predict_obj.predict(model_path), model_path)

    def test_create_model_package_with_pickle(self):
        packager = PythonTritonPackager()

        with patch(
            "michelangelo.lib.model_manager._private.packager.python_triton.model_package.download_model",
            wraps=download_model_with_pickle,
        ) as mock_download_model:
            with tempfile.TemporaryDirectory() as temp_dir:
                dest_model_path = os.path.join(temp_dir, "model")
                dest_model_path = packager.create_model_package(
                    "test_model_path",
                    model_class=model_class,
                    model_schema=self.model_schema,
                    model_name="test_model_name",
                    dest_model_path=dest_model_path,
                )

                mock_download_model.assert_called_once_with(
                    "test_model_path",
                    os.path.join(dest_model_path, "0", "model"),
                    "hdfs",
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
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn1.py",
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn2.py",
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn3.py",
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/folder/fn4.py",
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/__init__.py",
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn1.py",
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/package/fn2.py",
                    "0/michelangelo/lib/model_manager/_private/utils/module_finder/tests/fixtures/simple_module.py",
                    "0/michelangelo/lib/model_manager/interface/custom_model.py",
                    "0/michelangelo/lib/model_manager/packager/python_triton/tests/fixtures/model.py",
                    "0/michelangelo/lib/model_manager/packager/python_triton/tests/fixtures/predict.py",
                    "0/model.py",
                    "0/model/file.pkl",
                    "0/model/file.txt",
                    "0/model_class.txt",
                    "0/user_model.py",
                    "config.pbtxt",
                ]

                expected_files = sorted(package_files + self.model_loader_files)
                self.assertEqual(files, expected_files)

    @patch("michelangelo.lib.model_manager._private.packager.python_triton.model_package.download_model", wraps=download_empty_model)
    def test_create_model_package_with_empty_model(self, mock_download_model):
        packager = PythonTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "model")
            dest_model_path = packager.create_model_package(
                "test_model_path",
                dest_model_path=dest_model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                model_name="test_model_name",
            )

            mock_download_model.assert_called_once()

            with (
                open("michelangelo/lib/model_manager/packager/python_triton/tests/fixtures/config.pbtxt") as expected_f,
                open(os.path.join(dest_model_path, "config.pbtxt")) as f,
            ):
                expected_config = expected_f.read()
                config = f.read()
                self.assertEqual(config, expected_config)

            self.assertTrue(os.path.exists(os.path.join(dest_model_path, "0", "model")))
            self.assertEqual(len(os.listdir(os.path.join(dest_model_path, "0", "model"))), 0)

    def assertRawModelPackage(self, dest_model_path, mock_download_model, with_requirements=False, batch_inference=False):
        mock_download_model.assert_called_once_with(
            "test_model_path",
            os.path.join(dest_model_path, "model"),
            "hdfs",
        )

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
                "python_triton",
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
            "defs/michelangelo/lib/model_manager/packager/python_triton/tests/fixtures/predict.py",
            "defs/model_class.txt",
            "metadata/sample_data.json",
            "metadata/schema.yaml",
            "metadata/type.yaml",
            "model/file.txt",
        ]

        if with_requirements:
            expected_files.insert(-4, "dependencies/requirements.txt")

        for file in files:
            print(file)

        self.assertEqual(
            files,
            expected_files,
        )

    @patch("michelangelo.lib.model_manager._private.packager.python_triton.raw_model_package.download_model", wraps=download_model)
    def test_create_raw_model_package(self, mock_download_model):
        packager = PythonTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "raw_model")
            dest_model_path = packager.create_raw_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=self.sample_data,
                dest_model_path=dest_model_path,
            )
            self.assertRawModelPackage(dest_model_path, mock_download_model)

    @patch("michelangelo.lib.model_manager._private.packager.python_triton.raw_model_package.download_model", wraps=download_model)
    def test_create_raw_model_package_with_custom_batch_processing(self, mock_download_model):
        packager = PythonTritonPackager(custom_batch_processing=True)
        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "raw_model")
            dest_model_path = packager.create_raw_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=self.batch_sample_data,
                dest_model_path=dest_model_path,
            )
            self.assertRawModelPackage(dest_model_path, mock_download_model, batch_inference=True)

    def test_create_raw_model_package_with_empty_model_schema(self):
        packager = PythonTritonPackager()
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
        packager = PythonTritonPackager()
        with self.assertRaises(ValueError):
            packager.create_raw_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=model_schema,
                sample_data=self.sample_data,
            )

    def test_create_raw_model_package_missing_model_class(self):
        with self.assertRaises(ValueError):
            packager = PythonTritonPackager()
            packager.create_raw_model_package(
                "test_model_path",
                model_class=None,
                model_schema=self.model_schema,
                sample_data=self.sample_data,
            )

    def test_create_raw_model_package_with_invalid_model_class(self):
        with self.assertRaises(ValueError):
            packager = PythonTritonPackager()
            packager.create_raw_model_package(
                "test_model_path",
                model_class="invalid_class",
                model_schema=self.model_schema,
                sample_data=self.sample_data,
            )

    def test_create_raw_model_package_with_invalid_sample_data(self):
        with self.assertRaises(TypeError):
            packager = PythonTritonPackager()
            packager.create_raw_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=[{"a": "b"}],
            )

    def test_create_raw_model_package_with_mismatching_sample_data_and_model_schema(self):
        with self.assertRaises(ValueError):
            packager = PythonTritonPackager()
            packager.create_raw_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=[{"invalid": np.array([1])}],
            )

    def test_create_raw_model_package_with_requirements(self):
        packager = PythonTritonPackager()

        with patch(
            "michelangelo.lib.model_manager._private.packager.python_triton.raw_model_package.download_model",
            wraps=download_model,
        ) as mock_download_model:
            with tempfile.TemporaryDirectory() as temp_dir:
                dest_model_path = os.path.join(temp_dir, "raw_model")
                dest_model_path = packager.create_raw_model_package(
                    "test_model_path",
                    model_class=model_class,
                    model_schema=self.model_schema,
                    sample_data=self.sample_data,
                    dest_model_path=dest_model_path,
                    requirements=["numpy", "pandas"],
                )
                self.assertRawModelPackage(dest_model_path, mock_download_model, with_requirements=True)

    def test_create_raw_model_package_with_default_dest_model_path(self):
        packager = PythonTritonPackager()

        with patch(
            "michelangelo.lib.model_manager._private.packager.python_triton.raw_model_package.download_model",
            wraps=download_model,
        ) as mock_download_model:
            dest_model_path = packager.create_raw_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=self.sample_data,
            )
            self.assertRawModelPackage(dest_model_path, mock_download_model)

    def test_create_raw_model_package_with_altered_include_import_prefixes(self):
        packager = PythonTritonPackager()

        with patch(
            "michelangelo.lib.model_manager._private.packager.python_triton.raw_model_package.download_model",
            wraps=download_model,
        ):
            with tempfile.TemporaryDirectory() as temp_dir:
                dest_model_path = os.path.join(temp_dir, "raw_model")
                dest_model_path = packager.create_raw_model_package(
                    "test_model_path",
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
                        "defs/michelangelo/lib/model_manager/packager/python_triton/tests/fixtures/predict.py",
                        "defs/model_class.txt",
                        "metadata/sample_data.json",
                        "metadata/schema.yaml",
                        "metadata/type.yaml",
                        "model/file.txt",
                    ],
                )

    def test_create_raw_model_package_with_pickle(self):
        packager = PythonTritonPackager()

        with patch(
            "michelangelo.lib.model_manager._private.packager.python_triton.raw_model_package.download_model",
            wraps=download_model_with_pickle,
        ) as mock_download_model:
            with tempfile.TemporaryDirectory() as temp_dir:
                dest_model_path = os.path.join(temp_dir, "raw_model")
                dest_model_path = packager.create_raw_model_package(
                    "test_model_path",
                    model_class=model_class,
                    model_schema=self.model_schema,
                    sample_data=self.sample_data,
                    dest_model_path=dest_model_path,
                )

                mock_download_model.assert_called_once_with(
                    "test_model_path",
                    os.path.join(dest_model_path, "model"),
                    "hdfs",
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
                        "defs/michelangelo/lib/model_manager/packager/python_triton/tests/fixtures/model.py",
                        "defs/michelangelo/lib/model_manager/packager/python_triton/tests/fixtures/predict.py",
                        "defs/model_class.txt",
                        "metadata/sample_data.json",
                        "metadata/schema.yaml",
                        "metadata/type.yaml",
                        "model/file.pkl",
                        "model/file.txt",
                    ],
                )

    @patch("michelangelo.lib.model_manager._private.packager.python_triton.raw_model_package.download_model", wraps=download_empty_model)
    def test_create_raw_model_package_with_empty_model(self, mock_download_model):
        packager = PythonTritonPackager()
        with tempfile.TemporaryDirectory() as temp_dir:
            dest_model_path = os.path.join(temp_dir, "raw_model")
            dest_model_path = packager.create_raw_model_package(
                "test_model_path",
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=self.sample_data,
                dest_model_path=dest_model_path,
            )

            mock_download_model.assert_called_once()

            self.assertTrue(os.path.exists(os.path.join(dest_model_path, "model")))
            self.assertEqual(len(os.listdir(os.path.join(dest_model_path, "model"))), 0)
