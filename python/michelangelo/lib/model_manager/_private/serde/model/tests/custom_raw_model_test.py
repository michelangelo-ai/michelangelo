import os
import sys
import pickle
import tempfile
import numpy as np
from unittest import TestCase
from unittest.mock import patch
from uber.ai.michelangelo.sdk.model_manager.constants import StorageType
from uber.ai.michelangelo.sdk.model_manager.schema import ModelSchema, ModelSchemaItem, DataType
from uber.ai.michelangelo.sdk.model_manager.packager.python_triton import PythonTritonPackager
from uber.ai.michelangelo.sdk.model_manager._private.serde.model import load_custom_raw_model
from uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package import A, func
from uber.ai.michelangelo.sdk.model_manager.packager.python_triton.tests.fixtures.predict import Predict


class CustomRawModelTest(TestCase):
    def setUp(self):
        self.sys_path = sys.path.copy()
        self.main_dict = sys.modules["__main__"].__dict__.copy()
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

    def tearDown(self):
        sys.path = self.sys_path
        for key in list(sys.modules["__main__"].__dict__.keys()):
            if key not in self.main_dict:
                del sys.modules["__main__"].__dict__[key]
            else:
                sys.modules["__main__"].__dict__[key] = self.main_dict[key]

    @patch("uber.ai.michelangelo.sdk.model_manager._private.serde.model.custom_raw_model._logger.info")
    def test_load_custom_raw_model_from_external(self, mock_logger_info):
        model_path = "uber/ai/michelangelo/sdk/model_manager/_private/serde/model/tests/testdata/external_custom_raw_model_package"
        model = load_custom_raw_model(model_path)

        mock_logger_info.assert_called_with(
            "Module uber.ai.michelangelo.experimental.model_manager_playground.python_triton_model.predict not found in the system path. "
            "Trying to load from the model package."
        )

        # test predict
        inputs = {
            "feature": np.array(["test_feature"]),
        }

        result = model.predict(inputs)
        response = result.get("response")[0]

        self.assertEqual(
            response, "feature: test_feature and content: test_content and deps: package.fn1 and package.fn2 and folder.fn1 and deps: folder.fn2"
        )

    @patch("uber.ai.michelangelo.sdk.model_manager._private.serde.model.custom_raw_model._logger.info")
    def test_load_custom_raw_model_from_internal(self, mock_logger_info):
        model_class = "uber.ai.michelangelo.sdk.model_manager.packager.python_triton.tests.fixtures.predict.Predict"
        with tempfile.TemporaryDirectory() as temp_dir:
            src_model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "model_package")
            os.makedirs(src_model_path)
            os.makedirs(dest_model_path)

            with open(os.path.join(src_model_path, "test_file.txt"), "w") as f:
                f.write("test_content")

            packager = PythonTritonPackager()

            model_package = packager.create_raw_model_package(
                model_path=src_model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=self.sample_data,
                dest_model_path=dest_model_path,
                model_path_source_type=StorageType.LOCAL,
            )

            model = load_custom_raw_model(model_package)

            # the model is loaded without having to modify the system path
            mock_logger_info.assert_not_called()
            self.assertIsInstance(model, Predict)

            # test predict
            inputs = {
                "input": np.array(["test_feature"]),
            }

            result = model.predict(inputs)
            response = result.get("response")[0]

            self.assertEqual(response, "test_feature")

    def test_load_custom_raw_model_invalid_module(self):
        with tempfile.TemporaryDirectory() as model_package:
            os.makedirs(os.path.join(model_package, "defs"))
            with open(os.path.join(model_package, "defs", "model_class.txt"), "w") as f:
                f.write("uber.ai.michelangelo.sdk.model_manager.packager.python_triton.tests.fixtures.predict.InvalidPredict")

            with self.assertRaisesRegex(
                AttributeError,
                "Class InvalidPredict not found in module uber.ai.michelangelo.sdk.model_manager.packager.python_triton.tests.fixtures.predict.",
            ):
                load_custom_raw_model(model_package)

    def test_load_custom_raw_model_invalid_model_class_file(self):
        with tempfile.TemporaryDirectory() as model_package:
            with self.assertRaisesRegex(ValueError, "Missing defs/model_class.txt in the model package"):
                load_custom_raw_model(model_package)

            os.makedirs(os.path.join(model_package, "defs"))
            with open(os.path.join(model_package, "defs", "model_class.txt"), "w") as f:
                f.write("")

            with self.assertRaisesRegex(ValueError, "defs/model_class.txt is empty in the model package"):
                load_custom_raw_model(model_package)

            with open(os.path.join(model_package, "defs", "model_class.txt"), "w") as f:
                f.write("foo")

            with self.assertRaisesRegex(ValueError, "Invalid model class definition foo"):
                load_custom_raw_model(model_package)

    def test_load_custom_raw_model_with_pickle(self):
        model_class = "uber.ai.michelangelo.sdk.model_manager.packager.python_triton.tests.fixtures.predict.Predict"
        with tempfile.TemporaryDirectory() as temp_dir:
            src_model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "model_package")
            os.makedirs(src_model_path)
            os.makedirs(dest_model_path)

            with open(os.path.join(src_model_path, "test_file.txt"), "w") as f:
                f.write("test_content")

            with open(os.path.join(src_model_path, "A.pkl"), "wb") as f:
                pickle.dump(A(), f)

            with open(os.path.join(src_model_path, "func.pkl"), "wb") as f:
                pickle.dump(func, f)

            packager = PythonTritonPackager()

            model_package = packager.create_raw_model_package(
                model_path=src_model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=self.sample_data,
                dest_model_path=dest_model_path,
                model_path_source_type=StorageType.LOCAL,
            )

            model = load_custom_raw_model(model_package)
            self.assertIsInstance(model, Predict)

            # test predict
            inputs = {
                "input": np.array(["test_feature"]),
            }

            result = model.predict(inputs)
            response = result.get("response")[0]

            self.assertEqual(response, "test_feature")

    @patch("uber.ai.michelangelo.sdk.model_manager._private.serde.loader.custom_model_loader.walk_pickle_definitions_in_dir")
    def test_load_custom_raw_model_with_pickle_def_in_main(self, mock_walk_pickle_definitions_in_dir):
        mock_walk_pickle_definitions_in_dir.return_value = [(None, "fn1", None), (None, "fn2", None), (None, "module_attr", None)]
        model_class = "uber.ai.michelangelo.sdk.model_manager.packager.python_triton.tests.fixtures.predict.Predict"
        with tempfile.TemporaryDirectory() as temp_dir:
            src_model_path = os.path.join(temp_dir, "model")
            dest_model_path = os.path.join(temp_dir, "model_package")
            os.makedirs(src_model_path)
            os.makedirs(dest_model_path)

            with open(os.path.join(src_model_path, "test_file.txt"), "w") as f:
                f.write("test_content")

            with open(os.path.join(src_model_path, "A.pkl"), "wb") as f:
                pickle.dump(A(), f)

            with open(os.path.join(src_model_path, "func.pkl"), "wb") as f:
                pickle.dump(func, f)

            packager = PythonTritonPackager()

            model_package = packager.create_raw_model_package(
                model_path=src_model_path,
                model_class=model_class,
                model_schema=self.model_schema,
                sample_data=self.sample_data,
                dest_model_path=dest_model_path,
                model_path_source_type=StorageType.LOCAL,
            )

            model = load_custom_raw_model(model_package)
            self.assertIsInstance(model, Predict)

            # test predict
            inputs = {
                "input": np.array(["test_feature"]),
            }

            result = model.predict(inputs)
            response = result.get("response")[0]

            self.assertEqual(response, "test_feature")
