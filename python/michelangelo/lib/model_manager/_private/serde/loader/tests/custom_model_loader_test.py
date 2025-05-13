import os
import sys
import pickle
import tempfile
import numpy as np
from unittest import TestCase
from unittest.mock import patch
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager.schema import ModelSchema, ModelSchemaItem, DataType
from michelangelo.lib.model_manager.packager.python_triton import PythonTritonPackager
from michelangelo.lib.model_manager._private.serde.loader.custom_model_loader import load_custom_model
from michelangelo.lib.model_manager._private.serde.model import load_custom_raw_model
from michelangelo.lib.model_manager._private.utils.pickle_utils.tests.fixtures.package import A, func
from michelangelo.lib.model_manager.packager.python_triton.tests.fixtures.predict import Predict


class CustomModelLoaderTest(TestCase):
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

    def test_load_custom_raw_model_with_pickle(self):
        model_class = "michelangelo.lib.model_manager.packager.python_triton.tests.fixtures.predict.Predict"
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

    @patch("michelangelo.lib.model_manager._private.serde.loader.custom_model_loader.walk_pickle_definitions_in_dir")
    def test_load_custom_raw_model_with_pickle_def_in_main(self, mock_walk_pickle_definitions_in_dir):
        mock_walk_pickle_definitions_in_dir.return_value = [(None, "fn1", None), (None, "fn2", None), (None, "module_attr", None)]
        model_class = "michelangelo.lib.model_manager.packager.python_triton.tests.fixtures.predict.Predict"
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

    def test_load_custom_model_with_pickle_def_in_main_with_load_error(self):
        model_class = "michelangelo.lib.model_manager.packager.python_triton.tests.fixtures.predict.Predict"
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

            model_bin_path = os.path.join(model_package, "model")
            defs_path = os.path.join(model_package, "defs")

            sys.modules["__main__"].__dict__["fn1"] = func
            if "fn2" in sys.modules["__main__"].__dict__:
                del sys.modules["__main__"].__dict__["fn2"]
            if "module_attr" in sys.modules["__main__"].__dict__:
                del sys.modules["__main__"].__dict__["module_attr"]

            with (
                patch(
                    "michelangelo.lib.model_manager._private.serde.loader.custom_model_loader.walk_pickle_definitions_in_dir"
                ) as mock_walk_pickle_definitions_in_dir,
                patch.object(Predict, "load", side_effect=AttributeError("error")) as mock_load,
                self.assertRaises(RuntimeError),
            ):
                mock_walk_pickle_definitions_in_dir.return_value = [(None, "fn1", None), (None, "fn2", None), (None, "module_attr", None)]
                load_custom_model(model_bin_path, Predict, defs_path)
                mock_load.assert_called()
